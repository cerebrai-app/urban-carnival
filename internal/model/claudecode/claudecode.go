// Package claudecode implements model.Provider on top of the local Claude
// Code CLI (github.com/lancekrogers/claude-code-go), so the agent loop can
// be exercised end-to-end in developer builds without a hosted API key
// (see workerclient.DefaultProvider, gated on config.EnvDevSettings).
package claudecode

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cerebrai-app/urban-carnival/internal/model"
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

// ChatModel is a model.Provider backed by the Claude Code CLI (`claude -p`).
type ChatModel struct {
	client promptRunner
}

var _ model.Provider = (*ChatModel)(nil)

// New returns a ChatModel that invokes the Claude Code CLI at binPath
// (typically "claude", resolved via PATH).
func New(binPath string) *ChatModel {
	return &ChatModel{client: claude.NewClient(binPath)}
}

// Generate flattens the conversation history into a system prompt (from any
// system messages) plus a single transcript prompt, and runs it through the
// Claude Code CLI — `claude -p` takes one prompt per invocation rather than
// a message list.
func (m *ChatModel) Generate(ctx context.Context, messages []*schema.Message, _ ...einomodel.Option) (*schema.Message, error) {
	system, prompt := buildPrompt(messages)

	result, err := m.client.RunPromptCtx(ctx, prompt, &claude.RunOptions{
		Format:       claude.JSONOutput,
		SystemPrompt: system,
	})
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
