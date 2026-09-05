package workerclient

import (
	"context"

	"github.com/cerebrai-app/urban-carnival/internal/agent"
	"github.com/cerebrai-app/urban-carnival/internal/config"
	"github.com/cerebrai-app/urban-carnival/internal/model"
	"github.com/cerebrai-app/urban-carnival/internal/model/claudecode"
	"github.com/cloudwego/eino/schema"
)

// claudeCodeBin is the Claude Code CLI binary ProviderFor invokes for
// ModelClaudeCode, resolved via PATH.
const claudeCodeBin = "claude"

// ModelClaudeCode is the model ID for the local Claude Code CLI provider
// (internal/model/claudecode), available only in developer builds
// (config.EnvDevSettings).
const ModelClaudeCode = "claude-code"

// NewAgentLoop builds the conversation agent loop that SendMessage should
// use to generate real assistant replies once a model.Provider is wired in
// (DESIGN.md §5). Until then, Client implementations generate their own
// placeholder replies (see SQLite.SendMessage's mock echo).
func NewAgentLoop(ctx context.Context, provider model.Provider) (*agent.Loop, error) {
	return agent.New(ctx, provider)
}

// DefaultModel returns the model ID CreateSession assigns to a newly
// created session: ModelClaudeCode in developer builds, so a session can be
// exercised end-to-end without a hosted API key, or empty otherwise until a
// real hosted provider is wired in.
func DefaultModel() string {
	if config.DevEnabled() {
		return ModelClaudeCode
	}
	return ""
}

// AvailableModels lists the model IDs a client should offer the user for
// per-session selection, in display order. Empty in production builds
// until a real hosted provider is wired in.
func AvailableModels() []string {
	if config.DevEnabled() {
		return []string{ModelClaudeCode}
	}
	return nil
}

// ProviderFor resolves a session's model ID (Session.Model) to a
// model.Provider. An empty or unrecognized ID falls back to
// model.Unconfigured — e.g. a session created before any model was
// available, or one whose model is no longer offered.
func ProviderFor(modelID string) model.Provider {
	switch modelID {
	case ModelClaudeCode:
		return claudecode.New(claudeCodeBin)
	default:
		return model.Unconfigured{}
	}
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
