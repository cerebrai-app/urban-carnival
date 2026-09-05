package workerclient

import (
	"context"

	"github.com/cerebrai-app/urban-carnival/internal/agent"
	"github.com/cerebrai-app/urban-carnival/internal/devmode"
	"github.com/cerebrai-app/urban-carnival/internal/model"
	"github.com/cloudwego/eino/schema"
)

// NewAgentLoop builds the conversation agent loop that SendMessage should
// use to generate real assistant replies once a model.Provider is wired in
// (DESIGN.md §5). Until then, Client implementations generate their own
// placeholder replies (see SQLite.SendMessage's mock echo).
func NewAgentLoop(ctx context.Context, provider model.Provider) (*agent.Loop, error) {
	return agent.New(ctx, provider)
}

// DefaultModel returns the model ID CreateSession assigns to a newly
// created session. Only developer builds have a model to offer so far
// (devmode.DefaultModel); production sessions are created with an empty
// model until a real hosted provider is wired in.
func DefaultModel() string {
	return devmode.DefaultModel()
}

// AvailableModels lists the model IDs a client should offer the user for
// per-session selection, in display order. Only dev models exist so far
// (devmode.AvailableModels); empty in production builds.
func AvailableModels() []string {
	return devmode.AvailableModels()
}

// ProviderFor resolves a session's model ID (Session.Model) to a
// model.Provider. An empty or unrecognized ID falls back to
// model.Unconfigured — e.g. a session created before any model was
// available, or one whose model is no longer offered.
func ProviderFor(modelID string) model.Provider {
	if p := devmode.Provider(modelID); p != nil {
		return p
	}
	return model.Unconfigured{}
}

// DefaultProvider returns the model.Provider for DefaultModel().
func DefaultProvider() model.Provider {
	return ProviderFor(DefaultModel())
}

// toSchemaMessages converts persisted conversation history into the
// message format the agent loop's underlying chat model expects.
func toSchemaMessages(history []Message) []*schema.Message {
	out := make([]*schema.Message, len(history))
	for i, m := range history {
		role := schema.User
		if m.Role == "assistant" {
			role = schema.Assistant
		}
		out[i] = &schema.Message{Role: role, Content: m.Content}
	}
	return out
}
