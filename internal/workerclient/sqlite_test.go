package workerclient

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/cerebrai-app/urban-carnival/internal/config"
	"github.com/cerebrai-app/urban-carnival/internal/storage"
)

// newTestSQLite opens a fresh, migrated database in a temp directory and
// wraps it as a SQLite client, closing the underlying db when the test ends.
func newTestSQLite(t *testing.T) *SQLite {
	t.Helper()
	// Without this, storage.Path defaults to the real per-user application
	// data directory, and the Chdir below would not affect it.
	t.Setenv(config.EnvDevSettings, "1")
	t.Chdir(t.TempDir())

	ctx := context.Background()
	db, err := storage.Open(ctx)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})

	return NewSQLite(db)
}

func TestSQLiteListsSeededExampleAutomations(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	// The 0001 migration seeds two illustrative automations on a fresh
	// database; ListAutomations should surface them in insertion order.
	// (Migration idempotency across reopens is covered by
	// storage.TestOpenAppliesMigrationsAndIsIdempotent.)
	got, err := s.ListAutomations(ctx)
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d automations, want 2", len(got))
	}
	if got[0].ID != "water-plants" || !got[0].Enabled {
		t.Errorf("first automation = %+v, want id=water-plants enabled=true", got[0])
	}
	if got[1].ID != "inbox-summary" || got[1].Enabled {
		t.Errorf("second automation = %+v, want id=inbox-summary enabled=false", got[1])
	}
}

func TestSQLiteSetAutomationEnabledPersists(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	before, err := s.ListAutomations(ctx)
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	target := before[1]
	if target.Enabled {
		t.Fatalf("expected seeded automation %q to start disabled", target.ID)
	}

	if err := s.SetAutomationEnabled(ctx, target.ID, true); err != nil {
		t.Fatalf("SetAutomationEnabled: %v", err)
	}

	// A fresh SQLite wrapping the same db simulates a restart.
	after, err := NewSQLite(s.db).ListAutomations(ctx)
	if err != nil {
		t.Fatalf("ListAutomations after restart: %v", err)
	}
	if !after[1].Enabled {
		t.Errorf("automation %q still disabled after restart", target.ID)
	}
	if !after[1].UpdatedAt.After(target.UpdatedAt) {
		t.Errorf("UpdatedAt not advanced: was %v, now %v", target.UpdatedAt, after[1].UpdatedAt)
	}
}

func TestSQLiteSetAutomationEnabledUnknownID(t *testing.T) {
	s := newTestSQLite(t)

	err := s.SetAutomationEnabled(context.Background(), "nope", true)
	if err == nil {
		t.Fatal("expected an error for an unknown automation ID")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error should name the missing ID, got: %v", err)
	}
}

func TestSQLiteSendMessageEchoes(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	session, err := s.CreateSession(ctx, "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	got, err := s.SendMessage(ctx, session.ID, "hello there")
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if got.Role != "assistant" {
		t.Errorf("Role = %q, want %q", got.Role, "assistant")
	}
	if !strings.Contains(got.Content, "hello there") {
		t.Errorf("reply should echo the input, got: %q", got.Content)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt not set")
	}
}

func TestSQLiteCreateSessionDefaultsTitle(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	session, err := s.CreateSession(ctx, "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.Title != newSessionTitle {
		t.Errorf("Title = %q, want %q", session.Title, newSessionTitle)
	}
	if session.ID == "" {
		t.Error("ID not set")
	}
	if session.CreatedAt.IsZero() || session.UpdatedAt.IsZero() {
		t.Error("CreatedAt/UpdatedAt not set")
	}
}

func TestSQLiteListSessionsOrdersByMostRecentlyUpdated(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	first, err := s.CreateSession(ctx, "")
	if err != nil {
		t.Fatalf("CreateSession(first): %v", err)
	}
	second, err := s.CreateSession(ctx, "")
	if err != nil {
		t.Fatalf("CreateSession(second): %v", err)
	}

	// Sending a message on the first session bumps its updated_at, so it
	// should sort ahead of the second even though it was created earlier.
	if _, err := s.SendMessage(ctx, first.ID, "hello"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	got, err := s.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2", len(got))
	}
	if got[0].ID != first.ID {
		t.Errorf("got[0].ID = %q, want %q (most recently active)", got[0].ID, first.ID)
	}
	if got[1].ID != second.ID {
		t.Errorf("got[1].ID = %q, want %q", got[1].ID, second.ID)
	}
	if got[0].Title != titleFromContent("hello") {
		t.Errorf("first session title = %q, want retitled from its first message", got[0].Title)
	}
}

func TestSQLiteListMessagesReturnsSessionHistoryInOrder(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	session, err := s.CreateSession(ctx, "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	other, err := s.CreateSession(ctx, "")
	if err != nil {
		t.Fatalf("CreateSession(other): %v", err)
	}
	if _, err := s.SendMessage(ctx, other.ID, "unrelated"); err != nil {
		t.Fatalf("SendMessage(other): %v", err)
	}

	if _, err := s.SendMessage(ctx, session.ID, "first"); err != nil {
		t.Fatalf("SendMessage(first): %v", err)
	}
	if _, err := s.SendMessage(ctx, session.ID, "second"); err != nil {
		t.Fatalf("SendMessage(second): %v", err)
	}

	got, err := s.ListMessages(ctx, session.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d messages, want 4", len(got))
	}
	wantContents := []string{"first", "(mock worker) you said: \"first\"", "second", "(mock worker) you said: \"second\""}
	for i, want := range wantContents {
		if got[i].Content != want {
			t.Errorf("messages[%d].Content = %q, want %q", i, got[i].Content, want)
		}
		if got[i].SessionID != session.ID {
			t.Errorf("messages[%d].SessionID = %q, want %q", i, got[i].SessionID, session.ID)
		}
	}
}

// TestSQLiteConcurrentAccess exercises SQLite the way the desktop UI does:
// the automations view calls ListAutomations and SetAutomationEnabled from
// separate goroutines (internal/desktopui/automations.go). Run with -race.
func TestSQLiteConcurrentAccess(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := s.ListAutomations(ctx); err != nil {
				t.Errorf("ListAutomations: %v", err)
			}
		}()
		go func(enabled bool) {
			defer wg.Done()
			if err := s.SetAutomationEnabled(ctx, "inbox-summary", enabled); err != nil {
				t.Errorf("SetAutomationEnabled: %v", err)
			}
		}(i%2 == 0)
	}
	wg.Wait()
}
