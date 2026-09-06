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

	// calls counts every invocation; resumeErr, when set, fails any call that
	// carries a ResumeID, simulating a CLI that no longer has that session.
	calls     int
	resumeErr error
}

func (f *fakeRunner) RunPromptCtx(_ context.Context, prompt string, opts *claude.RunOptions) (*claude.ClaudeResult, error) {
	f.calls++
	f.gotPrompt = prompt
	f.gotOpts = opts
	if f.resumeErr != nil && opts.ResumeID != "" {
		return nil, f.resumeErr
	}
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

func TestChatModelReplyFirstTurn(t *testing.T) {
	fake := &fakeRunner{result: &claude.ClaudeResult{Result: "hi", SessionID: "sess-abc"}}
	m := &ChatModel{client: fake}

	reply, handle, err := m.Reply(context.Background(), "", []*schema.Message{
		{Role: schema.User, Content: "first"},
	})
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if reply.Content != "hi" || handle != "sess-abc" {
		t.Errorf("Reply = (%q, %q), want (%q, %q)", reply.Content, handle, "hi", "sess-abc")
	}
	if fake.gotOpts.ResumeID != "" {
		t.Errorf("ResumeID = %q, want empty on the first turn", fake.gotOpts.ResumeID)
	}
	if fake.gotPrompt != "User: first" {
		t.Errorf("prompt = %q, want the full transcript", fake.gotPrompt)
	}
}

func TestChatModelReplyResumes(t *testing.T) {
	fake := &fakeRunner{result: &claude.ClaudeResult{Result: "ok", SessionID: "sess-xyz"}}
	m := &ChatModel{client: fake}

	_, handle, err := m.Reply(context.Background(), "sess-abc", []*schema.Message{
		{Role: schema.User, Content: "first"},
		{Role: schema.Assistant, Content: "hi"},
		{Role: schema.User, Content: "second"},
	})
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if fake.gotOpts.ResumeID != "sess-abc" {
		t.Errorf("ResumeID = %q, want %q", fake.gotOpts.ResumeID, "sess-abc")
	}
	if fake.gotPrompt != "second" {
		t.Errorf("prompt = %q, want only the latest user message on resume", fake.gotPrompt)
	}
	if handle != "sess-xyz" {
		t.Errorf("handle = %q, want the session id from the result", handle)
	}
}

func TestChatModelReplyResumeOmitsSystemPrompt(t *testing.T) {
	fake := &fakeRunner{result: &claude.ClaudeResult{Result: "ok", SessionID: "s2"}}
	m := &ChatModel{client: fake}

	if _, _, err := m.Reply(context.Background(), "s1", []*schema.Message{
		{Role: schema.System, Content: "be terse"},
		{Role: schema.User, Content: "hi"},
	}); err != nil {
		t.Fatalf("Reply: %v", err)
	}
	// The resumed CLI session already carries its system prompt; re-sending
	// one would override it every turn.
	if fake.gotOpts.SystemPrompt != "" {
		t.Errorf("SystemPrompt = %q, want empty on resume", fake.gotOpts.SystemPrompt)
	}
	if fake.gotOpts.ResumeID != "s1" {
		t.Errorf("ResumeID = %q, want %q", fake.gotOpts.ResumeID, "s1")
	}
}

func TestChatModelReplyRetriesWhenResumeFails(t *testing.T) {
	fake := &fakeRunner{
		result:    &claude.ClaudeResult{Result: "recovered", SessionID: "sess-new"},
		resumeErr: errors.New("no conversation found for the given session id"),
	}
	m := &ChatModel{client: fake}

	reply, handle, err := m.Reply(context.Background(), "sess-stale", []*schema.Message{
		{Role: schema.User, Content: "first"},
		{Role: schema.Assistant, Content: "hi"},
		{Role: schema.User, Content: "second"},
	})
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("calls = %d, want 2 (failed resume, then full-transcript retry)", fake.calls)
	}
	if reply.Content != "recovered" || handle != "sess-new" {
		t.Errorf("Reply = (%q, %q), want (%q, %q)", reply.Content, handle, "recovered", "sess-new")
	}
	// The retry drops the stale handle and replays the whole conversation.
	if fake.gotOpts.ResumeID != "" {
		t.Errorf("retry ResumeID = %q, want empty", fake.gotOpts.ResumeID)
	}
	if fake.gotPrompt != "User: first\n\nAssistant: hi\n\nUser: second" {
		t.Errorf("retry prompt = %q, want the full transcript", fake.gotPrompt)
	}
}

func TestChatModelReplyFirstTurnErrorNotRetried(t *testing.T) {
	fake := &fakeRunner{err: errors.New("boom")}
	m := &ChatModel{client: fake}

	if _, _, err := m.Reply(context.Background(), "", []*schema.Message{
		{Role: schema.User, Content: "hi"},
	}); err == nil {
		t.Fatal("expected error")
	}
	if fake.calls != 1 {
		t.Errorf("calls = %d, want 1 (no resume handle, nothing to retry)", fake.calls)
	}
}

func TestChatModelGenerateWithoutMCP(t *testing.T) {
	fake := &fakeRunner{result: &claude.ClaudeResult{Result: "ok"}}
	m := &ChatModel{client: fake}

	if _, err := m.Generate(context.Background(), []*schema.Message{{Role: schema.User, Content: "hi"}}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(fake.gotOpts.MCPConfigs) != 0 || fake.gotOpts.PermissionMode != "" {
		t.Errorf("MCP options set without WithMCP: %+v", fake.gotOpts)
	}
}

func TestChatModelGenerateWithMCP(t *testing.T) {
	fake := &fakeRunner{result: &claude.ClaudeResult{Result: "ok"}}
	m := &ChatModel{client: fake}
	WithMCP(`{"mcpServers":{}}`, []string{"mcp__cerebrai__create_automation"})(m)

	if _, err := m.Generate(context.Background(), []*schema.Message{{Role: schema.User, Content: "hi"}}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := fake.gotOpts.MCPConfigs; len(got) != 1 || got[0] != `{"mcpServers":{}}` {
		t.Errorf("MCPConfigs = %v", got)
	}
	if !fake.gotOpts.StrictMCPConfig {
		t.Error("StrictMCPConfig not set")
	}
	if got := fake.gotOpts.AllowedTools; len(got) != 1 || got[0] != "mcp__cerebrai__create_automation" {
		t.Errorf("AllowedTools = %v", got)
	}
	if fake.gotOpts.PermissionMode != claude.PermissionModeBypassPermissions {
		t.Errorf("PermissionMode = %q", fake.gotOpts.PermissionMode)
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
