package devmode

import (
	einomodel "github.com/cloudwego/eino/components/model"

	"github.com/cerebrai-app/urban-carnival/internal/devmode/claudecode"
)

// ModelClaudeCode is the model ID for the local Claude Code CLI provider
// (internal/devmode/claudecode), available only in developer builds
// (Enabled).
const ModelClaudeCode = "claude-code"

// claudeCodeBin is the Claude Code CLI binary Provider invokes for
// ModelClaudeCode, resolved via PATH.
const claudeCodeBin = "claude"

// DefaultChatModel returns the chat model ID a newly created session should
// be assigned: ModelClaudeCode in developer builds, so a session can be
// exercised end-to-end without a hosted API key, or empty otherwise until a
// real hosted provider is wired in.
func DefaultChatModel() string {
	if Enabled() {
		return ModelClaudeCode
	}
	return ""
}

// AvailableChatModels lists the dev-only chat model IDs a client should
// offer the user for per-session selection, in display order. Nil in
// production builds until a real hosted provider is wired in.
func AvailableChatModels() []string {
	if Enabled() {
		return []string{ModelClaudeCode}
	}
	return nil
}

// AgentModel returns the model ID the automation writer agent runs on
// (DESIGN.md §5.3). Unlike the chat model this is worker-global, not
// per-session: production picks it once during user setup, and developer
// builds hard-code ModelClaudeCode. Empty in production builds until that
// setup exists.
func AgentModel() string {
	if Enabled() {
		return ModelClaudeCode
	}
	return ""
}

// Provider resolves a dev-only model ID to its concrete chat model, or nil
// if the ID is not one of the dev models (the caller then falls back to its
// own default, e.g. chat.Unconfigured / automationagent.Unconfigured). The
// same Claude Code CLI wrapper serves both the chat and automation-writer
// seams in dev builds, so there's one resolver here; chat vs. agent is a
// distinction the callers (chat.ModelProvider / automationagent.ModelProvider)
// make, not this catalog.
func Provider(modelID string) einomodel.ToolCallingChatModel {
	if modelID == ModelClaudeCode {
		return claudecode.New(claudeCodeBin)
	}
	return nil
}
