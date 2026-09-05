// Package claudecode implements the chat and automation-writer model
// provider seams (chat.ModelProvider / automationagent.ModelProvider) on top
// of the local Claude Code CLI (github.com/lancekrogers/claude-code-go), so
// both can be exercised end-to-end in developer builds without a hosted API
// key (see devmode.Provider, gated on devmode.Enabled).
package claudecode

import (
	"context"
	"errors"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/lancekrogers/claude-code-go/pkg/claude"
)

// promptRunner is the subset of *claude.ClaudeClient ChatModel depends on,
// narrowed so tests can substitute a fake instead of shelling out to a real
// claude binary.
type promptRunner interface {
	RunPromptCtx(ctx context.Context, prompt string, opts *claude.RunOptions) (*claude.ClaudeResult, error)
}

// ChatModel is a provider backed by the Claude Code CLI (`claude -p`). It
// satisfies both chat.ModelProvider and automationagent.ModelProvider — in
// dev builds the same CLI wrapper serves chat and the automation writer.
type ChatModel struct {
	client promptRunner
	// mcpConfigJSON, when non-empty, is an inline `--mcp-config` JSON document
	// pointing the CLI at cerebrai's in-process MCP server (DESIGN.md §5.6);
	// mcpTools is the qualified tool names to pre-approve. Set via WithMCP,
	// used only by the chat seam — the automation writer must not get these
	// tools since its own run is what they invoke.
	mcpConfigJSON string
	mcpTools      []string
}

// Option configures a ChatModel at construction.
type Option func(*ChatModel)

// WithMCP attaches cerebrai's in-process MCP server to every CLI invocation:
// configJSON is an inline `--mcp-config` document and toolNames are the
// mcp__<server>__<tool> names to allow without prompting (DESIGN.md §5.6).
func WithMCP(configJSON string, toolNames []string) Option {
	return func(m *ChatModel) {
		m.mcpConfigJSON = configJSON
		m.mcpTools = toolNames
	}
}

// New returns a ChatModel that invokes the Claude Code CLI at binPath
// (typically "claude", resolved via PATH).
func New(binPath string, opts ...Option) *ChatModel {
	m := &ChatModel{client: claude.NewClient(binPath)}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Generate flattens the conversation history into a system prompt (from any
// system messages) plus a single transcript prompt, and runs it through the
// Claude Code CLI — `claude -p` takes one prompt per invocation rather than
// a message list.
func (m *ChatModel) Generate(ctx context.Context, messages []*schema.Message, _ ...einomodel.Option) (*schema.Message, error) {
	system, prompt := buildPrompt(messages)

	opts := &claude.RunOptions{
		Format:       claude.JSONOutput,
		SystemPrompt: system,
	}
	if m.mcpConfigJSON != "" {
		// Attach only our server (StrictMCPConfig) and pre-approve its tools
		// so the non-interactive `-p` run doesn't block on a permission
		// prompt. bypassPermissions is acceptable here: dev-only, and
		// cerebrai runs automations unsandboxed anyway (DESIGN.md §7).
		opts.MCPConfigs = []string{m.mcpConfigJSON}
		opts.StrictMCPConfig = true
		opts.AllowedTools = m.mcpTools
		opts.PermissionMode = claude.PermissionModeBypassPermissions
	}

	result, err := m.client.RunPromptCtx(ctx, prompt, opts)
	if err != nil {
		return nil, fmt.Errorf("claudecode: %w", err)
	}
	if result.IsError {
		return nil, fmt.Errorf("claudecode: %s", result.Result)
	}

	return &schema.Message{Role: schema.Assistant, Content: result.Result}, nil
}

// Stream is not implemented: the CLI's streaming mode emits stream-json
// events rather than incremental *schema.Message chunks, and nothing in the
// agent loop needs incremental output yet.
func (m *ChatModel) Stream(context.Context, []*schema.Message, ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("claudecode: streaming not implemented")
}

// WithTools rejects any tools: the Claude Code CLI drives its own tool use
// inside the child process and has no mechanism to accept an externally
// supplied Eino tool schema.
func (m *ChatModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	if len(tools) > 0 {
		return nil, errors.New("claudecode: external tool binding is not supported")
	}
	return m, nil
}

// buildPrompt splits history into a system prompt (concatenated system
// messages) and a single transcript-style prompt for everything else.
func buildPrompt(messages []*schema.Message) (system, prompt string) {
	var sys, convo []string
	for _, msg := range messages {
		switch msg.Role {
		case schema.System:
			sys = append(sys, msg.Content)
		case schema.Assistant:
			convo = append(convo, "Assistant: "+msg.Content)
		case schema.Tool:
			convo = append(convo, "Tool ("+msg.ToolName+"): "+msg.Content)
		default:
			convo = append(convo, "User: "+msg.Content)
		}
	}
	return strings.Join(sys, "\n\n"), strings.Join(convo, "\n\n")
}
