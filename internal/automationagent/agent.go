// Package automationagent builds the automation writer agent loop (DESIGN.md
// §5.3): the genuinely agentic loop that authors or edits an automation,
// invoked by the chat handoff (§5.2) rather than wrapped around every chat
// turn. It sits between the seed task it's handed and a pluggable
// ModelProvider (Provider resolves the worker-global one). Orchestration is
// built on Eino's ReAct agent, so the loop can call tools (see tools.go) as
// part of producing its result, with tracing layered on later without
// reshaping this package's callers.
package automationagent

import (
	"context"
	"errors"
	"fmt"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/cerebrai-app/urban-carnival/internal/devmode"
)

// ModelProvider is the model the automation writer's loop invokes (DESIGN.md
// §5.3), potentially many times per task as it calls tools. It's Eino's
// ToolCallingChatModel, a named interface rather than an alias so it can
// diverge from chat.ModelProvider — the automation writer can run on a
// different (e.g. stronger, code-focused) model than chat.
type ModelProvider interface {
	einomodel.ToolCallingChatModel
}

// ErrNotConfigured is returned by every Unconfigured call.
var ErrNotConfigured = errors.New("automationagent: no model provider configured")

// Unconfigured is a placeholder ModelProvider used until the worker-global
// automation writer model is wired in, so the loop can be built and tested
// before any vendor integration exists.
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

// Provider resolves the worker-global automation writer model (DESIGN.md
// §5.3) to a ModelProvider. Unlike chat's model this is not per-session:
// production picks it once during user setup, developer builds hard-code it
// (devmode.AgentModel). Falls back to Unconfigured until that setup exists /
// in production builds.
func Provider() ModelProvider {
	if p := devmode.Provider(devmode.AgentModel()); p != nil {
		return p
	}
	return Unconfigured{}
}

// Loop runs one automation-writing task to completion: given the seed
// message(s), it invokes the underlying ModelProvider — calling any tools it
// decides to use along the way — and returns its final reply.
type Loop struct {
	agent *react.Agent
}

// New compiles a Loop around provider, wired with the tools every Loop
// exposes to its model (see defaultTools). This is the seam DESIGN.md §5.6
// calls for: swap provider for a real Anthropic/OpenAI/local ModelProvider
// without touching anything downstream of Loop.
//
// Not every provider supports tools — e.g. claudecode.ChatModel.WithTools
// rejects a non-empty tool list outright, since the Claude Code CLI drives
// its own tool use internally. New surfaces that as an error here rather
// than silently building a tool-less loop.
func New(ctx context.Context, provider ModelProvider) (*Loop, error) {
	tools, err := defaultTools(provider)
	if err != nil {
		return nil, fmt.Errorf("build agent loop tools: %w", err)
	}

	a, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: provider,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: tools},
	})
	if err != nil {
		return nil, fmt.Errorf("compile agent loop: %w", err)
	}
	return &Loop{agent: a}, nil
}

// Respond runs the seed messages through the loop — including any tool calls
// the model makes along the way — and returns its final reply message.
func (l *Loop) Respond(ctx context.Context, history []*schema.Message) (*schema.Message, error) {
	reply, err := l.agent.Generate(ctx, history)
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	return reply, nil
}
