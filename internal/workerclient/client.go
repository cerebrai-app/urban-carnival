// Package workerclient talks to the cerebrai background worker over its
// local API (see DESIGN.md §3). The desktop app is a thin UI over this
// client; it holds no automation, memory, or LLM logic of its own.
package workerclient

import (
	"context"
	"time"
)

// Session is one persisted conversation thread. A user may hold several at
// once (DESIGN.md §2's "Conversation"); each keeps its own message history.
type Session struct {
	ID    string
	Title string
	// Model is the model ID this session's replies should be generated
	// with (see DefaultModel, AvailableModels, ProviderFor). Empty means
	// none has been assigned yet.
	Model     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message is one turn in a conversation with the assistant.
type Message struct {
	ID        string
	SessionID string
	Role      string // "user" or "assistant"
	Content   string
	CreatedAt time.Time
}

// Automation is a discrete piece of LLM-generated code plus metadata, as
// described in DESIGN.md §2.
type Automation struct {
	ID          string
	Name        string
	Description string
	Trigger     string // e.g. "schedule: 0 8 * * *", "webhook", "manual"
	Enabled     bool
	UpdatedAt   time.Time
}

// Client is the desktop app's view of the background worker's local API.
// The concrete implementation (HTTP, Unix socket, etc.) is decided when the
// worker's IPC transport is implemented.
//
// Implementations must be safe for concurrent use: the UI issues every call
// from its own goroutine so the main thread never blocks on the worker.
type Client interface {
	// CreateSession starts a new, empty conversation thread with the given
	// title and returns it.
	CreateSession(ctx context.Context, title string) (Session, error)

	// ListSessions returns every conversation thread, most recently active
	// first.
	ListSessions(ctx context.Context) ([]Session, error)

	// SetSessionModel changes the model ID a session's future replies
	// should be generated with. See AvailableModels for the IDs a client
	// should offer the user, and ProviderFor to resolve one to a
	// model.Provider.
	SetSessionModel(ctx context.Context, sessionID, model string) error

	// ListMessages returns every message in the given session, oldest
	// first.
	ListMessages(ctx context.Context, sessionID string) ([]Message, error)

	// SendMessage submits a user message to the given session and returns
	// the assistant's reply once generated. Both the user's message and
	// the reply are persisted.
	SendMessage(ctx context.Context, sessionID, content string) (Message, error)

	// ListAutomations returns all automations known to the worker.
	ListAutomations(ctx context.Context) ([]Automation, error)

	// SetAutomationEnabled enables or disables an automation's trigger.
	SetAutomationEnabled(ctx context.Context, id string, enabled bool) error
}
