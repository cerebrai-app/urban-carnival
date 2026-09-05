package devmode

import (
	"github.com/cerebrai-app/urban-carnival/internal/devmode/claudecode"
	"github.com/cerebrai-app/urban-carnival/internal/model"
)

// ModelClaudeCode is the model ID for the local Claude Code CLI provider
// (internal/devmode/claudecode), available only in developer builds
// (Enabled).
const ModelClaudeCode = "claude-code"

// claudeCodeBin is the Claude Code CLI binary Provider invokes for
// ModelClaudeCode, resolved via PATH.
const claudeCodeBin = "claude"

// DefaultModel returns the model ID a newly created session should be
// assigned: ModelClaudeCode in developer builds, so a session can be
// exercised end-to-end without a hosted API key, or empty otherwise until a
// real hosted provider is wired in.
func DefaultModel() string {
	if Enabled() {
		return ModelClaudeCode
	}
	return ""
}

// AvailableModels lists the dev-only model IDs a client should offer the
// user for per-session selection, in display order. Nil in production builds
// until a real hosted provider is wired in.
func AvailableModels() []string {
	if Enabled() {
		return []string{ModelClaudeCode}
	}
	return nil
}

// Provider resolves a dev-only model ID to a model.Provider, or nil if the
// ID is not one of the dev models (the caller then falls back to its own
// default, e.g. model.Unconfigured).
func Provider(modelID string) model.Provider {
	switch modelID {
	case ModelClaudeCode:
		return claudecode.New(claudeCodeBin)
	default:
		return nil
	}
}
