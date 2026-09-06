// Package app defines cerebrai's application layer: the domain types and the
// Client port that the desktop UI (internal/desktopui) is written against.
// The UI holds no automation, memory, or LLM logic of its own — it only
// calls this interface (DESIGN.md §3). The engine behind the port runs
// in-process in the same cmd/cerebrai-desktop binary; the concrete
// implementation lives elsewhere (internal/storage's SQLite Client today) so
// swapping it never touches the UI.
package app

import (
	"context"
	"strings"
	"time"
)

// Summarize reduces s to a one-line label: its first line, with surrounding
// whitespace trimmed and the result truncated to at most maxRunes runes with a
// trailing ellipsis. It returns "" when that first line is empty or all
// whitespace. Shared by session titling (internal/storage) and automation
// naming (internal/devmode/devmcp), which want the same rule.
func Summarize(s string, maxRunes int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return s
}

// Session is one persisted conversation thread. A user may hold several at
// once (DESIGN.md §2's "Conversation"); each keeps its own message history.
type Session struct {
	ID    string
	Title string
	// Model is the chat model ID this session's replies should be generated
	// with (see chat.DefaultModel, chat.AvailableModels, chat.ProviderFor).
	// Empty means none has been assigned yet. The automation writer agent's
	// model is process-global, not stored here (see automationagent.Provider).
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
	// Source is the automation writer's authored code for this automation
	// (DESIGN.md §5.5). It's a draft until §4's review flow activates it, so
	// a freshly written automation is persisted with Enabled false.
	Source    string
	UpdatedAt time.Time
}

// AutomationDraft is the not-yet-persisted output of the automation writer
// (DESIGN.md §5.5): authored Source plus the metadata the writer settled on.
// The store assigns the ID, forces Enabled false, and stamps UpdatedAt. It
// lives here, next to Automation, so both internal/storage and the dev MCP
// server (internal/devmode/devmcp) can name it without importing each other.
type AutomationDraft struct {
	Name        string
	Description string
	Trigger     string
	Source      string
}

// Client is the UI's view of the engine that owns automations, chat, and
// memory. That engine runs in-process (DESIGN.md §3); the concrete
// implementation is chosen at wire-up (SQLite today). The port is kept as a
// seam so the engine could be split out later without touching the UI.
//
// Implementations must be safe for concurrent use: the UI issues every call
// from its own goroutine so the main thread never blocks on a slow call.
type Client interface {
	// CreateSession starts a new, empty conversation thread with the given
	// title and returns it.
	CreateSession(ctx context.Context, title string) (Session, error)

	// ListSessions returns every conversation thread, most recently active
	// first.
	ListSessions(ctx context.Context) ([]Session, error)

	// SetSessionModel changes the chat model ID a session's future replies
	// should be generated with. See chat.AvailableModels for the IDs a
	// client should offer the user, and chat.ProviderFor to resolve one to a
	// chat.ModelProvider.
	SetSessionModel(ctx context.Context, sessionID, model string) error

	// ListMessages returns every message in the given session, oldest
	// first.
	ListMessages(ctx context.Context, sessionID string) ([]Message, error)

	// SendMessage submits a user message to the given session and returns
	// the assistant's reply once generated. Both the user's message and
	// the reply are persisted.
	SendMessage(ctx context.Context, sessionID, content string) (Message, error)

	// ListAutomations returns all automations known to the engine.
	ListAutomations(ctx context.Context) ([]Automation, error)

	// SetAutomationEnabled enables or disables an automation's trigger.
	SetAutomationEnabled(ctx context.Context, id string, enabled bool) error
}
