package automationagent

import (
	"context"
	"errors"
	"strings"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/cerebrai-app/urban-carnival/internal/devmode"
	"github.com/cerebrai-app/urban-carnival/internal/devmode/claudecode"
)

// TestChatModelImplementsModelProvider keeps claudecode.ChatModel wired to
// this seam. devmode.Provider hands it back as an einomodel.ToolCallingChatModel,
// so this is where a future divergence between ModelProvider and that
// interface would first fail to compile. It lives here, in the in-package
// test, because automationagent already imports claudecode transitively via
// devmode — no external test package needed.
func TestChatModelImplementsModelProvider(_ *testing.T) {
	var _ ModelProvider = (*claudecode.ChatModel)(nil)
}

// echoProvider is a minimal ModelProvider stand-in for exercising Loop
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

func TestProviderDev(t *testing.T) {
	t.Setenv(devmode.EnvDevMode, "true")
	if _, ok := Provider().(*claudecode.ChatModel); !ok {
		t.Errorf("Provider() = %T, want *claudecode.ChatModel", Provider())
	}
}

func TestProviderProd(t *testing.T) {
	t.Setenv(devmode.EnvDevMode, "false")
	if _, ok := Provider().(Unconfigured); !ok {
		t.Errorf("Provider() = %T, want Unconfigured", Provider())
	}
}

func TestSpawnAgentRespectsDepthLimit(t *testing.T) {
	tl, err := newSpawnAgentTool(echoProvider{})
	if err != nil {
		t.Fatalf("newSpawnAgentTool: %v", err)
	}

	// A context already at the depth cap: the next spawn would exceed it.
	ctx := withSpawnDepth(context.Background(), maxSpawnDepth)
	if _, err := tl.InvokableRun(ctx, `{"task":"anything"}`); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Errorf("InvokableRun at the depth cap: err = %v, want a depth-limit error", err)
	}

	// One level below the cap still runs.
	ctx = withSpawnDepth(context.Background(), maxSpawnDepth-1)
	if _, err := tl.InvokableRun(ctx, `{"task":"anything"}`); err != nil {
		t.Errorf("InvokableRun below the depth cap: %v", err)
	}
}

func TestLoopRespondUnconfigured(t *testing.T) {
	ctx := context.Background()
	loop, err := New(ctx, Unconfigured{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = loop.Respond(ctx, []*schema.Message{{Role: schema.User, Content: "hello"}})
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Respond error = %v, want ErrNotConfigured", err)
	}
}
