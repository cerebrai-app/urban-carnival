// Package agent builds the conversation agent loop that sits between the
// worker's persisted chat history and a pluggable model.Provider
// (DESIGN.md §5). Orchestration is built on Eino's ReAct agent, so the loop
// can call tools (see tools.go) as part of producing a reply, with tracing
// layered on later without reshaping this package's callers.
package agent

import (
	"context"
	"fmt"

	"github.com/cerebrai-app/urban-carnival/internal/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// Loop runs one conversational turn: given the message history so far, it
// invokes the underlying model.Provider — calling any tools it decides to
// use along the way — and returns its final reply.
type Loop struct {
	agent *react.Agent
}

// New compiles a Loop around provider, wired with the tools every Loop
// exposes to its model (see defaultTools). This is the seam DESIGN.md §5
// calls for: swap provider for a real Anthropic/OpenAI/local model.Provider
// without touching anything downstream of Loop.
//
// Not every provider supports tools — e.g. claudecode.ChatModel.WithTools
// rejects a non-empty tool list outright, since the Claude Code CLI drives
// its own tool use internally. New surfaces that as an error here rather
// than silently building a tool-less loop.
func New(ctx context.Context, provider model.Provider) (*Loop, error) {
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

// Respond runs history through the loop — including any tool calls the
// model makes along the way — and returns its final reply message.
func (l *Loop) Respond(ctx context.Context, history []*schema.Message) (*schema.Message, error) {
	reply, err := l.agent.Generate(ctx, history)
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	return reply, nil
}
