package claudecode

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/lancekrogers/claude-code-go/pkg/claude"
)

type fakeRunner struct {
	gotPrompt string
	gotOpts   *claude.RunOptions
	result    *claude.ClaudeResult
	err       error
}

func (f *fakeRunner) RunPromptCtx(_ context.Context, prompt string, opts *claude.RunOptions) (*claude.ClaudeResult, error) {
	f.gotPrompt = prompt
	f.gotOpts = opts
	return f.result, f.err
}

func TestChatModelGenerate(t *testing.T) {
	fake := &fakeRunner{result: &claude.ClaudeResult{Result: "hi there"}}
	m := &ChatModel{client: fake}

	reply, err := m.Generate(context.Background(), []*schema.Message{
		{Role: schema.System, Content: "be terse"},
		{Role: schema.User, Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if reply.Role != schema.Assistant || reply.Content != "hi there" {
		t.Errorf("reply = %+v, want role %q content %q", reply, schema.Assistant, "hi there")
	}
	if fake.gotOpts.SystemPrompt != "be terse" {
		t.Errorf("SystemPrompt = %q, want %q", fake.gotOpts.SystemPrompt, "be terse")
	}
	if fake.gotPrompt != "User: hello" {
		t.Errorf("prompt = %q, want %q", fake.gotPrompt, "User: hello")
	}
}

func TestChatModelGenerateRunError(t *testing.T) {
	fake := &fakeRunner{err: errors.New("boom")}
	m := &ChatModel{client: fake}

	if _, err := m.Generate(context.Background(), []*schema.Message{{Role: schema.User, Content: "hi"}}); err == nil {
		t.Fatal("expected error")
	}
}

func TestChatModelGenerateResultError(t *testing.T) {
	fake := &fakeRunner{result: &claude.ClaudeResult{IsError: true, Result: "denied"}}
	m := &ChatModel{client: fake}

	if _, err := m.Generate(context.Background(), []*schema.Message{{Role: schema.User, Content: "hi"}}); err == nil {
		t.Fatal("expected error")
	}
}

func TestChatModelWithTools(t *testing.T) {
	m := New("claude")

	if _, err := m.WithTools(nil); err != nil {
		t.Errorf("WithTools(nil): %v", err)
	}
	if _, err := m.WithTools([]*schema.ToolInfo{{Name: "x"}}); err == nil {
		t.Error("WithTools with tools: expected error")
	}
}

func TestChatModelStream(t *testing.T) {
	m := New("claude")
	if _, err := m.Stream(context.Background(), nil); err == nil {
		t.Error("Stream: expected error")
	}
}
