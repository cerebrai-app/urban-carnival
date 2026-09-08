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

// fakeStreamingProvider implements StreamingProvider: it emits its reply and
// thoughts one rune at a time through onChunk.
type fakeStreamingProvider struct {
	fakeConversationProvider
	thoughts string
}

func (f *fakeStreamingProvider) ReplyStream(_ context.Context, priorHandle string, history []*schema.Message, onChunk func(app.ReplyChunk)) (*schema.Message, string, string, error) {
	f.gotPrior = priorHandle
	f.gotHistory = history
	if f.err != nil {
		return nil, "", "", f.err
	}
	for _, r := range f.thoughts {
		onChunk(app.ReplyChunk{Thought: string(r)})
	}
	for _, r := range f.reply {
		onChunk(app.ReplyChunk{Answer: string(r)})
	}
	return &schema.Message{Role: schema.Assistant, Content: f.reply}, f.thoughts, f.handle, nil
}

func TestReplyStreamStreamsAndAggregates(t *testing.T) {
	fake := &fakeStreamingProvider{
		fakeConversationProvider: fakeConversationProvider{reply: "hey", handle: "s2"},
		thoughts:                 "hm",
	}

	var streamed app.ReplyChunk
	reply, thoughts, handle, err := ReplyStream(context.Background(), fake, []app.Message{{Role: "user", Content: "hi"}}, "s1", func(c app.ReplyChunk) {
		streamed.Thought += c.Thought
		streamed.Answer += c.Answer
	})
	if err != nil {
		t.Fatalf("ReplyStream: %v", err)
	}
	if reply != "hey" || thoughts != "hm" || handle != "s2" {
		t.Errorf("ReplyStream = (%q, %q, %q), want (hey, hm, s2)", reply, thoughts, handle)
	}
	if streamed.Answer != "hey" || streamed.Thought != "hm" {
		t.Errorf("streamed = %+v, want answer hey / thought hm", streamed)
	}
	if fake.gotPrior != "s1" {
		t.Errorf("priorHandle = %q, want s1", fake.gotPrior)
	}
}

func TestReplyStreamFallsBackToReply(t *testing.T) {
	// A plain ConversationProvider (no ReplyStream): ReplyStream must still
	// work, with no chunks and empty thoughts.
	fake := &fakeConversationProvider{reply: "whole", handle: "h"}
	called := false
	reply, thoughts, handle, err := ReplyStream(context.Background(), fake, []app.Message{{Role: "user", Content: "hi"}}, "", func(app.ReplyChunk) { called = true })
	if err != nil {
		t.Fatalf("ReplyStream: %v", err)
	}
	if reply != "whole" || thoughts != "" || handle != "h" {
		t.Errorf("ReplyStream = (%q, %q, %q), want (whole, \"\", h)", reply, thoughts, handle)
	}
	if called {
		t.Error("onChunk called for a non-streaming provider")
	}
}

func TestReplyStreamUnconfigured(t *testing.T) {
	_, _, _, err := ReplyStream(context.Background(), Unconfigured{}, nil, "", nil)
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("ReplyStream error = %v, want ErrNotConfigured", err)
	}
}
