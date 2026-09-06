// Package claudecode implements the chat and automation-writer model
// provider seams (chat.ConversationProvider / automationagent.ModelProvider)
// on top of the local Claude Code CLI (github.com/lancekrogers/claude-code-go),
// so both can be exercised end-to-end in developer builds without a hosted API
// key (see devmode.Provider, gated on devmode.Enabled).
package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/lancekrogers/claude-code-go/pkg/claude"

	"github.com/cerebrai-app/urban-carnival/internal/app"
)

// promptRunner is the subset of *claude.ClaudeClient ChatModel depends on,
// narrowed so tests can substitute a fake instead of shelling out to a real
// claude binary.
type promptRunner interface {
	RunPromptCtx(ctx context.Context, prompt string, opts *claude.RunOptions) (*claude.ClaudeResult, error)
	// StreamPrompt runs a prompt in stream-json mode, delivering messages as
	// they arrive on the first channel and a terminal error (if any) on the
	// second. Both channels are closed when the run ends.
	StreamPrompt(ctx context.Context, prompt string, opts *claude.RunOptions) (<-chan claude.Message, <-chan error)
}

// ChatModel is a provider backed by the Claude Code CLI (`claude -p`). It
// satisfies both chat.ConversationProvider (via Reply) and
// automationagent.ModelProvider (via Generate) — in dev builds the same CLI
// wrapper serves chat and the automation writer.
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
// a message list. It's the automation-writer seam
// (automationagent.ModelProvider); the chat seam uses Reply instead, which
// carries the CLI's own session across turns.
func (m *ChatModel) Generate(ctx context.Context, messages []*schema.Message, _ ...einomodel.Option) (*schema.Message, error) {
	msg, _, err := m.run(ctx, messages, "")
	return msg, err
}

// Reply implements chat.ConversationProvider (DESIGN.md §5.2): one assistant
// turn that continues the CLI's own session. On the first turn priorHandle is
// empty and the whole transcript is sent; once the CLI has returned a
// session_id, later turns pass it as --resume and send only the newest user
// message, letting the CLI keep the prior context itself. The returned handle
// is the session_id to persist and replay next turn. A resume that fails
// (stale session) is retried once with the full transcript.
func (m *ChatModel) Reply(ctx context.Context, priorHandle string, history []*schema.Message) (*schema.Message, string, error) {
	return m.run(ctx, history, priorHandle)
}

// run invokes the CLI, resuming resumeID when it's set. A resume that fails
// — most often because the CLI no longer has that session on disk — is
// retried once from scratch with the full transcript, so a stale handle
// self-heals instead of wedging every future turn.
func (m *ChatModel) run(ctx context.Context, messages []*schema.Message, resumeID string) (*schema.Message, string, error) {
	if resumeID != "" && len(messages) > 0 {
		msg, handle, err := m.invoke(ctx, messages, resumeID)
		if err == nil || ctx.Err() != nil {
			return msg, handle, err
		}
		// The resume failed; fall through and replay the whole conversation
		// so one bad handle can't fail this session forever.
	}
	return m.invoke(ctx, messages, "")
}

// invoke runs the CLI once. resumeID, when set, resumes that CLI session and
// narrows the prompt to just the latest message; otherwise the full history
// is flattened into a system prompt + transcript.
func (m *ChatModel) invoke(ctx context.Context, messages []*schema.Message, resumeID string) (*schema.Message, string, error) {
	prompt, opts := m.buildRun(messages, resumeID)
	opts.Format = claude.JSONOutput

	result, err := m.client.RunPromptCtx(ctx, prompt, opts)
	if err != nil {
		return nil, "", fmt.Errorf("claudecode: %w", err)
	}
	if result.IsError {
		return nil, "", fmt.Errorf("claudecode: %s", result.Result)
	}

	return &schema.Message{Role: schema.Assistant, Content: result.Result}, result.SessionID, nil
}

// buildRun turns the turn's messages plus an optional resume handle into the
// CLI prompt and a base RunOptions (output Format left for the caller to set).
// It's shared by the one-shot invoke and the streaming streamInvoke.
func (m *ChatModel) buildRun(messages []*schema.Message, resumeID string) (string, *claude.RunOptions) {
	system, prompt := buildPrompt(messages)

	opts := &claude.RunOptions{}
	if resumeID != "" && len(messages) > 0 {
		// The CLI already holds everything before this turn — the system
		// prompt included — so send only the latest message; re-sending the
		// transcript would duplicate it, and re-sending the system prompt
		// would override the resumed session's own every turn.
		opts.ResumeID = resumeID
		prompt = messages[len(messages)-1].Content
	} else {
		opts.SystemPrompt = system
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
	return prompt, opts
}

// ReplyStream implements chat.StreamingProvider (DESIGN.md §5.2): one assistant
// turn that continues the CLI's own session, with the model's reasoning and
// answer streamed to onChunk as they're produced. Session-continuity and the
// stale-resume retry match Reply; the returned message holds the full answer,
// thoughts the full reasoning (empty when the run surfaced none), and handle
// the session_id to persist.
func (m *ChatModel) ReplyStream(ctx context.Context, priorHandle string, history []*schema.Message, onChunk func(app.ReplyChunk)) (*schema.Message, string, string, error) {
	if priorHandle != "" && len(history) > 0 {
		msg, thoughts, handle, err := m.streamInvoke(ctx, history, priorHandle, onChunk)
		if err == nil || ctx.Err() != nil {
			return msg, thoughts, handle, err
		}
		// The resume failed; replay the whole conversation so one bad handle
		// can't wedge the session forever (see run).
	}
	return m.streamInvoke(ctx, history, "", onChunk)
}

// streamInvoke runs the CLI once in stream-json mode, forwarding incremental
// reasoning and answer text to onChunk and accumulating both for the return.
func (m *ChatModel) streamInvoke(ctx context.Context, messages []*schema.Message, resumeID string, onChunk func(app.ReplyChunk)) (*schema.Message, string, string, error) {
	prompt, opts := m.buildRun(messages, resumeID)
	opts.Format = claude.StreamJSONOutput
	// Ask the CLI to pass through the underlying model stream so reasoning and
	// answer text arrive token-by-token rather than only in the final message.
	opts.IncludePartialMessages = true

	msgCh, errCh := m.client.StreamPrompt(ctx, prompt, opts)

	var answer, thought, blockThought strings.Builder
	var handle, resultText string
	var isError bool
	for msg := range msgCh {
		if t, ok := msg.PartialText(); ok {
			answer.WriteString(t)
			emitChunk(onChunk, app.ReplyChunk{Answer: t})
		}
		if t, ok := partialThinking(msg); ok {
			thought.WriteString(t)
			emitChunk(onChunk, app.ReplyChunk{Thought: t})
		}
		if t := assistantThinking(msg); t != "" {
			// The consolidated assistant message: a fallback for when the
			// stream carried no thinking_delta events. Kept separate so it
			// doesn't double-count the streamed deltas.
			blockThought.Reset()
			blockThought.WriteString(t)
		}
		if msg.Type == "result" {
			handle = msg.SessionID
			resultText = msg.Result
			isError = msg.IsError
		}
	}
	if err := <-errCh; err != nil {
		return nil, "", "", fmt.Errorf("claudecode: %w", err)
	}
	if isError {
		return nil, "", "", fmt.Errorf("claudecode: %s", resultText)
	}

	finalAnswer := resultText
	if finalAnswer == "" {
		finalAnswer = answer.String()
	}
	thoughts := thought.String()
	if thoughts == "" && blockThought.Len() > 0 {
		thoughts = blockThought.String()
		emitChunk(onChunk, app.ReplyChunk{Thought: thoughts})
	}
	return &schema.Message{Role: schema.Assistant, Content: finalAnswer}, thoughts, handle, nil
}

func emitChunk(onChunk func(app.ReplyChunk), c app.ReplyChunk) {
	if onChunk != nil {
		onChunk(c)
	}
}

// partialThinking extracts an incremental reasoning fragment from a
// "stream_event" message's raw Anthropic event payload (a content_block_delta
// carrying a thinking_delta). All other messages and event types yield ("",
// false). It's the streaming counterpart to claude.Message.PartialText, which
// the SDK only implements for text_delta.
func partialThinking(m claude.Message) (string, bool) {
	if m.Type != "stream_event" || len(m.Event) == 0 {
		return "", false
	}
	var ev struct {
		Type  string `json:"type"`
		Delta struct {
			Type     string `json:"type"`
			Thinking string `json:"thinking"`
		} `json:"delta"`
	}
	if err := json.Unmarshal(m.Event, &ev); err != nil {
		return "", false
	}
	if ev.Type != "content_block_delta" || ev.Delta.Type != "thinking_delta" || ev.Delta.Thinking == "" {
		return "", false
	}
	return ev.Delta.Thinking, true
}

// assistantThinking concatenates the thinking content blocks of a consolidated
// "assistant" message, or "" when there are none. Used only as a fallback when
// the stream carried no thinking_delta events.
func assistantThinking(m claude.Message) string {
	if m.Type != "assistant" || len(m.Message) == 0 {
		return ""
	}
	var envelope struct {
		Content []struct {
			Type     string `json:"type"`
			Thinking string `json:"thinking"`
		} `json:"content"`
	}
	if err := json.Unmarshal(m.Message, &envelope); err != nil {
		return ""
	}
	var b strings.Builder
	for _, block := range envelope.Content {
		if block.Type == "thinking" && block.Thinking != "" {
			b.WriteString(block.Thinking)
		}
	}
	return b.String()
}

// Stream is not implemented: the einomodel streaming contract wants
// incremental *schema.Message chunks, which the automation-writer loop
// doesn't need. The chat seam streams via ReplyStream instead.
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
