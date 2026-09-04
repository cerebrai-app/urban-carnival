// Package workerclient talks to the cerebrai background worker over its
// local API (see DESIGN.md §3). The desktop app is a thin UI over this
// client; it holds no automation, memory, or LLM logic of its own.
package workerclient

import (
	"context"
	"time"
)

// Message is one turn in a conversation with the assistant.
type Message struct {
	ID        string
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
	// SendMessage submits a user message to the conversation and returns
	// the assistant's reply once generated.
	SendMessage(ctx context.Context, content string) (Message, error)

	// ListAutomations returns all automations known to the worker.
	ListAutomations(ctx context.Context) ([]Automation, error)

	// SetAutomationEnabled enables or disables an automation's trigger.
	SetAutomationEnabled(ctx context.Context, id string, enabled bool) error
}
