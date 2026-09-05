package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/cerebrai-app/urban-carnival/internal/model"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// echoProvider is a minimal model.Provider stand-in for exercising Loop
// without a real vendor integration.
type echoProvider struct{}

func (echoProvider) Generate(_ context.Context, input []*schema.Message, _ ...einomodel.Option) (*schema.Message, error) {
	last := input[len(input)-1]
	return &schema.Message{Role: schema.Assistant, Content: "echo: " + last.Content}, nil
}

func (echoProvider) Stream(context.Context, []*schema.Message, ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("echoProvider: streaming not implemented")
}

func (e echoProvider) WithTools([]*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return e, nil
}

func TestLoopRespond(t *testing.T) {
	ctx := context.Background()
	loop, err := New(ctx, echoProvider{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	reply, err := loop.Respond(ctx, []*schema.Message{{Role: schema.User, Content: "hello"}})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if want := "echo: hello"; reply.Content != want {
		t.Errorf("reply.Content = %q, want %q", reply.Content, want)
	}
}

// spawningProvider drives a spawn_agent tool call on its first turn, then a
// plain reply once it sees the tool's result — used to exercise Loop's
// tool-calling wiring end to end, including the sub-Loop spawn_agent builds
// (see tools.go). Behavior is keyed off the last message rather than a call
// counter so it works the same for the outer loop and the sub-loop it spawns.
type spawningProvider struct{}

func (spawningProvider) Generate(_ context.Context, input []*schema.Message, _ ...einomodel.Option) (*schema.Message, error) {
	last := input[len(input)-1]
	switch {
	case last.Role == schema.Tool:
		return &schema.Message{Role: schema.Assistant, Content: "final: " + last.Content}, nil
	case last.Content == "ping":
		return &schema.Message{Role: schema.Assistant, Content: "sub-reply: ping"}, nil
	default:
		return &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:       "call-1",
				Type:     "function",
				Function: schema.FunctionCall{Name: "spawn_agent", Arguments: `{"task":"ping"}`},
			}},
		}, nil
	}
}

func (spawningProvider) Stream(context.Context, []*schema.Message, ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("spawningProvider: streaming not implemented")
}

func (p spawningProvider) WithTools([]*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return p, nil
}

func TestLoopRespondSpawnsAgent(t *testing.T) {
	ctx := context.Background()
	loop, err := New(ctx, spawningProvider{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	reply, err := loop.Respond(ctx, []*schema.Message{{Role: schema.User, Content: "hello"}})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if want := "final: sub-reply: ping"; reply.Content != want {
		t.Errorf("reply.Content = %q, want %q", reply.Content, want)
	}
}

func TestLoopRespondUnconfigured(t *testing.T) {
	ctx := context.Background()
	loop, err := New(ctx, model.Unconfigured{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = loop.Respond(ctx, []*schema.Message{{Role: schema.User, Content: "hello"}})
	if !errors.Is(err, model.ErrNotConfigured) {
		t.Errorf("Respond error = %v, want ErrNotConfigured", err)
	}
}
