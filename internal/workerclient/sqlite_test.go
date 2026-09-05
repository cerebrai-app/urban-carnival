package workerclient

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

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

	s, err := NewSQLite(ctx, db)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	return s
}

func TestSQLiteSeedsExampleAutomationsOnce(t *testing.T) {
	s := newTestSQLite(t)
	ctx := context.Background()

	got, err := s.ListAutomations(ctx)
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	want := len(exampleAutomations(time.Now()))
	if len(got) != want {
		t.Fatalf("got %d automations, want %d", len(got), want)
	}

	// Re-wrapping the same db (as a restart would) must not seed again.
	s2, err := NewSQLite(ctx, s.db)
	if err != nil {
		t.Fatalf("second NewSQLite: %v", err)
	}
	again, err := s2.ListAutomations(ctx)
	if err != nil {
		t.Fatalf("ListAutomations after reopen: %v", err)
	}
	if len(again) != len(got) {
		t.Errorf("automation count changed across reopen: got %d, want %d", len(again), len(got))
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
	restarted, err := NewSQLite(ctx, s.db)
	if err != nil {
		t.Fatalf("NewSQLite after update: %v", err)
	}
	after, err := restarted.ListAutomations(ctx)
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

	got, err := s.SendMessage(context.Background(), "hello there")
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
