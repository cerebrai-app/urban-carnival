// Package chat is the plain, non-agentic conversation surface (DESIGN.md
// §5.2): one tool-bound Generate per user message, no tool-execution loop,
// no Eino react.Agent. It owns the chat model seam (ModelProvider) that
// vendor wiring plugs into, plus the per-session model catalog
// (DefaultModel / AvailableModels / ProviderFor), kept separate from the
// automation writer's seam (internal/automationagent) so the two can run on
// different models.
package chat

import (
	"context"
	"errors"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/cerebrai-app/urban-carnival/internal/devmode"
)

// ModelProvider is the model the chat session invokes once per turn
// (DESIGN.md §5.2). It's Eino's ToolCallingChatModel — chat needs WithTools
// only to bind create_automation/edit_automation as intent signals, not to
// drive a tool-execution loop. It is a named interface rather than an alias
// so it can diverge from automationagent.ModelProvider later.
type ModelProvider interface {
	einomodel.ToolCallingChatModel
}

// ErrNotConfigured is returned by every Unconfigured call.
var ErrNotConfigured = errors.New("chat: no model provider configured")

// Unconfigured is a placeholder ModelProvider used until a real model
// provider is wired in, so the chat surface can be built and tested before
// any vendor integration exists. It's the fallback for an
// unrecognized/empty session model ID.
type Unconfigured struct{}

var _ ModelProvider = Unconfigured{}

func (Unconfigured) Generate(context.Context, []*schema.Message, ...einomodel.Option) (*schema.Message, error) {
	return nil, ErrNotConfigured
}

func (Unconfigured) Stream(context.Context, []*schema.Message, ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, ErrNotConfigured
}

func (u Unconfigured) WithTools([]*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return u, nil
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
// ModelProvider. An empty or unrecognized ID falls back to Unconfigured —
// e.g. a session created before any model was available, or one whose model
// is no longer offered.
func ProviderFor(modelID string) ModelProvider {
	if p := devmode.ChatProvider(modelID); p != nil {
		return p
	}
	return Unconfigured{}
}

// DefaultProvider returns the ModelProvider for DefaultModel().
func DefaultProvider() ModelProvider {
	return ProviderFor(DefaultModel())
}
