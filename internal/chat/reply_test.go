package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/cerebrai-app/urban-carnival/internal/app"
)

// fakeConversationProvider records what Reply passed it and returns a canned
// message plus a handle.
type fakeConversationProvider struct {
	gotPrior   string
	gotHistory []*schema.Message
	reply      string
	handle     string
	err        error
}

func (f *fakeConversationProvider) Reply(_ context.Context, priorHandle string, history []*schema.Message) (*schema.Message, string, error) {
	f.gotPrior = priorHandle
	f.gotHistory = history
	if f.err != nil {
		return nil, "", f.err
	}
	return &schema.Message{Role: schema.Assistant, Content: f.reply}, f.handle, nil
}

func TestReplyThreadsHandleAndHistory(t *testing.T) {
	fake := &fakeConversationProvider{reply: "hi", handle: "sess-2"}
	history := []app.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "second"},
	}

	text, handle, err := Reply(context.Background(), fake, history, "sess-1")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if text != "hi" || handle != "sess-2" {
		t.Errorf("Reply = (%q, %q), want (%q, %q)", text, handle, "hi", "sess-2")
	}
	if fake.gotPrior != "sess-1" {
		t.Errorf("priorHandle = %q, want %q", fake.gotPrior, "sess-1")
	}
	if len(fake.gotHistory) != 3 || fake.gotHistory[2].Role != schema.User || fake.gotHistory[2].Content != "second" {
		t.Errorf("history = %+v, want 3 messages ending in user %q", fake.gotHistory, "second")
	}
}

func TestReplyUnconfigured(t *testing.T) {
	_, _, err := Reply(context.Background(), Unconfigured{}, nil, "")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Reply error = %v, want ErrNotConfigured", err)
	}
}
