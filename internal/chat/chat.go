// Package chat is the plain, non-agentic conversation surface (DESIGN.md
// §5.2): one reply per user message, no tool-execution loop, no Eino
// react.Agent. It owns the chat session seam (ConversationProvider) that
// vendor wiring plugs into, plus the per-session model catalog (DefaultModel
// / AvailableModels / ProviderFor), kept separate from the automation
// writer's seam (internal/automationagent) so the two can run on different
// models — and so chat can carry provider-side conversation continuity the
// writer doesn't need.
package chat

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/schema"

	"github.com/cerebrai-app/urban-carnival/internal/devmode"
)

// ConversationProvider is the chat session seam (DESIGN.md §5.2): one
// assistant turn per call, with provider-side conversation continuity. It is
// deliberately distinct from automationagent.ModelProvider / Eino's
// ToolCallingChatModel — chat needs a resumable conversation, the automation
// writer needs multi-round tool orchestration.
type ConversationProvider interface {
	// Reply generates one assistant turn from the conversation history
	// (oldest first, including the just-sent user message). priorHandle is
	// the provider-side conversation handle returned by the previous turn
	// (empty on the first turn); the returned handle is persisted and
	// replayed on the next turn. It may differ from priorHandle — a provider
	// may fork on resume. A provider with no such concept ignores priorHandle
	// and returns "".
	Reply(ctx context.Context, priorHandle string, history []*schema.Message) (reply *schema.Message, handle string, err error)
}

// ErrNotConfigured is returned by every Unconfigured call.
var ErrNotConfigured = errors.New("chat: no model provider configured")

// Unconfigured is a placeholder ConversationProvider used until a real
// provider is wired in, so the chat surface can be built and tested before
// any vendor integration exists. It's the fallback for an unrecognized/empty
// session model ID.
type Unconfigured struct{}

var _ ConversationProvider = Unconfigured{}

func (Unconfigured) Reply(context.Context, string, []*schema.Message) (*schema.Message, string, error) {
	return nil, "", ErrNotConfigured
}

// DefaultModel returns the chat model ID a newly created session should be
// assigned. Only developer builds have a model to offer so far; production
// sessions start with an empty model until a real hosted provider is wired
// in.
func DefaultModel() string {
	return devmode.DefaultChatModel()
}

// AvailableModels lists the chat model IDs a client should offer the user
// for per-session selection, in display order. Empty in production builds.
func AvailableModels() []string {
	return devmode.AvailableChatModels()
}

// ProviderFor resolves a session's model ID (app.Session.Model) to a
// ConversationProvider. An empty or unrecognized ID falls back to
// Unconfigured — e.g. a session created before any model was available, or
// one whose model is no longer offered.
func ProviderFor(modelID string) ConversationProvider {
	if p := devmode.ChatProvider(modelID); p != nil {
		return p
	}
	return Unconfigured{}
}

// DefaultProvider returns the ConversationProvider for DefaultModel().
func DefaultProvider() ConversationProvider {
	return ProviderFor(DefaultModel())
}
