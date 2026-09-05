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
