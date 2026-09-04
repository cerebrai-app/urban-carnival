package workerclient

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// TestMockConcurrentAccess exercises Mock the way the desktop UI does: the
// automations view calls ListAutomations and SetAutomationEnabled from
// separate goroutines (internal/desktopui/automations.go), so Mock must be
// safe for concurrent use. Run with -race.
func TestMockConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	m := NewMock()

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := m.ListAutomations(ctx); err != nil {
				t.Errorf("ListAutomations: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := m.SetAutomationEnabled(ctx, "inbox-summary", i%2 == 0); err != nil {
				t.Errorf("SetAutomationEnabled: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestMockSetAutomationEnabled(t *testing.T) {
	ctx := context.Background()
	m := NewMock()

	before, err := m.ListAutomations(ctx)
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	target := before[1]
	if target.Enabled {
		t.Fatalf("expected seeded automation %q to start disabled", target.ID)
	}

	if err := m.SetAutomationEnabled(ctx, target.ID, true); err != nil {
		t.Fatalf("SetAutomationEnabled: %v", err)
	}

	after, err := m.ListAutomations(ctx)
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	if !after[1].Enabled {
		t.Errorf("automation %q still disabled after enabling", target.ID)
	}
	if !after[1].UpdatedAt.After(target.UpdatedAt) {
		t.Errorf("UpdatedAt not advanced: was %v, now %v", target.UpdatedAt, after[1].UpdatedAt)
	}
}

func TestMockSetAutomationEnabledUnknownID(t *testing.T) {
	err := NewMock().SetAutomationEnabled(context.Background(), "nope", true)
	if err == nil {
		t.Fatal("expected an error for an unknown automation ID")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error should name the missing ID, got: %v", err)
	}
}

// TestMockListAutomationsReturnsCopy guards the callers' assumption that the
// returned slice is theirs to hold: the automations view keeps it as the
// list's backing data and mutates it optimistically after a toggle.
func TestMockListAutomationsReturnsCopy(t *testing.T) {
	ctx := context.Background()
	m := NewMock()

	got, err := m.ListAutomations(ctx)
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	got[0].Name = "mutated by caller"

	again, err := m.ListAutomations(ctx)
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	if again[0].Name == "mutated by caller" {
		t.Error("ListAutomations exposed its internal slice to callers")
	}
}

func TestMockSendMessageEchoes(t *testing.T) {
	got, err := NewMock().SendMessage(context.Background(), "hello there")
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
