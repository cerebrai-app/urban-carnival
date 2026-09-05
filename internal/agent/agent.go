// Package agent builds the conversation agent loop that sits between the
// worker's persisted chat history and a pluggable model.Provider
// (DESIGN.md §5). Orchestration is built on Eino so tool-calling and
// tracing can be layered on later without reshaping this package's
// callers.
package agent

import (
	"context"
	"fmt"

	"github.com/cerebrai-app/urban-carnival/internal/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// Loop runs one conversational turn: given the message history so far, it
// invokes the underlying model.Provider and returns its reply.
type Loop struct {
	runnable compose.Runnable[[]*schema.Message, *schema.Message]
}

// New compiles a Loop around provider. This is the seam DESIGN.md §5 calls
// for: swap provider for a real Anthropic/OpenAI/local model.Provider
// without touching anything downstream of Loop.
func New(ctx context.Context, provider model.Provider) (*Loop, error) {
	runnable, err := compose.NewChain[[]*schema.Message, *schema.Message]().
		AppendChatModel(provider).
		Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("compile agent loop: %w", err)
	}
	return &Loop{runnable: runnable}, nil
}

// Respond runs history through the loop's model provider and returns its
// reply message.
func (l *Loop) Respond(ctx context.Context, history []*schema.Message) (*schema.Message, error) {
	reply, err := l.runnable.Invoke(ctx, history)
	if err != nil {
		return nil, fmt.Errorf("agent loop: %w", err)
	}
	return reply, nil
}
