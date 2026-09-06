package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/lancekrogers/claude-code-go/pkg/claude"

	"github.com/cerebrai-app/urban-carnival/internal/app"
)

type fakeRunner struct {
	gotPrompt string
	gotOpts   *claude.RunOptions
	result    *claude.ClaudeResult
	err       error

	// streamMsgs is replayed on the channel StreamPrompt returns; streamErr
	// (if set) follows on the error channel. When streamMsgs is empty the
	// runner synthesizes a single result message from result.
	streamMsgs []claude.Message
	streamErr  error

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

func (f *fakeRunner) StreamPrompt(_ context.Context, prompt string, opts *claude.RunOptions) (<-chan claude.Message, <-chan error) {
	f.calls++
	f.gotPrompt = prompt
	f.gotOpts = opts

	msgCh := make(chan claude.Message)
	errCh := make(chan error, 1)
	go func() {
		defer close(msgCh)
		defer close(errCh)
		if f.resumeErr != nil && opts.ResumeID != "" {
			errCh <- f.resumeErr
			return
		}
		if f.streamErr != nil {
			errCh <- f.streamErr
			return
		}
		msgs := f.streamMsgs
		if len(msgs) == 0 && f.result != nil {
			msgs = []claude.Message{{
				Type:      "result",
				Result:    f.result.Result,
				SessionID: f.result.SessionID,
				IsError:   f.result.IsError,
			}}
		}
		for _, m := range msgs {
			msgCh <- m
		}
	}()
	return msgCh, errCh
}

// textDelta / thinkingDelta build the stream_event messages the CLI emits with
// --include-partial-messages, so tests can drive ReplyStream token by token.
func textDelta(s string) claude.Message {
	return claude.Message{Type: "stream_event", Event: []byte(
		`{"type":"content_block_delta","delta":{"type":"text_delta","text":` + jsonString(s) + `}}`)}
}

func thinkingDelta(s string) claude.Message {
	return claude.Message{Type: "stream_event", Event: []byte(
		`{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":` + jsonString(s) + `}}`)}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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

func TestChatModelReplyStreamChunks(t *testing.T) {
	fake := &fakeRunner{streamMsgs: []claude.Message{
		{Type: "system"},
		thinkingDelta("weigh "),
		thinkingDelta("options"),
		textDelta("Hello"),
		textDelta(", world"),
		{Type: "result", Result: "Hello, world", SessionID: "sess-1"},
	}}
	m := &ChatModel{client: fake}

	var thoughts, answer string
	reply, gotThoughts, handle, err := m.ReplyStream(context.Background(), "", []*schema.Message{
		{Role: schema.User, Content: "hi"},
	}, func(c app.ReplyChunk) {
		thoughts += c.Thought
		answer += c.Answer
	})
	if err != nil {
		t.Fatalf("ReplyStream: %v", err)
	}
	if answer != "Hello, world" || thoughts != "weigh options" {
		t.Errorf("streamed (answer=%q thoughts=%q), want (%q, %q)", answer, thoughts, "Hello, world", "weigh options")
	}
	if reply.Content != "Hello, world" || gotThoughts != "weigh options" || handle != "sess-1" {
		t.Errorf("ReplyStream = (%q, %q, %q), want (%q, %q, %q)", reply.Content, gotThoughts, handle, "Hello, world", "weigh options", "sess-1")
	}
	if !fake.gotOpts.IncludePartialMessages || fake.gotOpts.Format != claude.StreamJSONOutput {
		t.Errorf("opts = %+v, want stream-json + partial messages", fake.gotOpts)
	}
}

func TestChatModelReplyStreamNilCallback(t *testing.T) {
	fake := &fakeRunner{streamMsgs: []claude.Message{
		textDelta("ok"),
		{Type: "result", Result: "ok", SessionID: "s"},
	}}
	m := &ChatModel{client: fake}

	if _, _, _, err := m.ReplyStream(context.Background(), "", []*schema.Message{{Role: schema.User, Content: "hi"}}, nil); err != nil {
		t.Fatalf("ReplyStream(nil callback): %v", err)
	}
}

func TestChatModelReplyStreamResumeThenRetry(t *testing.T) {
	fake := &fakeRunner{
		result:    &claude.ClaudeResult{Result: "recovered", SessionID: "sess-new"},
		resumeErr: errors.New("no conversation found for the given session id"),
	}
	m := &ChatModel{client: fake}

	reply, _, handle, err := m.ReplyStream(context.Background(), "sess-stale", []*schema.Message{
		{Role: schema.User, Content: "first"},
		{Role: schema.Assistant, Content: "hi"},
		{Role: schema.User, Content: "second"},
	}, nil)
	if err != nil {
		t.Fatalf("ReplyStream: %v", err)
	}
	if fake.calls != 2 {
		t.Errorf("calls = %d, want 2 (failed resume, then full-transcript retry)", fake.calls)
	}
	if reply.Content != "recovered" || handle != "sess-new" {
		t.Errorf("ReplyStream = (%q, %q), want (%q, %q)", reply.Content, handle, "recovered", "sess-new")
	}
	if fake.gotOpts.ResumeID != "" || fake.gotPrompt != "User: first\n\nAssistant: hi\n\nUser: second" {
		t.Errorf("retry sent ResumeID=%q prompt=%q, want empty resume + full transcript", fake.gotOpts.ResumeID, fake.gotPrompt)
	}
}

func TestChatModelReplyStreamResultError(t *testing.T) {
	fake := &fakeRunner{streamMsgs: []claude.Message{
		{Type: "result", Result: "denied", IsError: true},
	}}
	m := &ChatModel{client: fake}

	if _, _, _, err := m.ReplyStream(context.Background(), "", []*schema.Message{{Role: schema.User, Content: "hi"}}, nil); err == nil {
		t.Fatal("expected error from an is_error result")
	}
}

func TestChatModelReplyStreamThinkingBlockFallback(t *testing.T) {
	// No thinking_delta events; the consolidated assistant message carries the
	// reasoning instead. It should still surface, via a single chunk.
	fake := &fakeRunner{streamMsgs: []claude.Message{
		{Type: "assistant", Message: []byte(`{"content":[{"type":"thinking","thinking":"quiet reasoning"},{"type":"text","text":"answer"}]}`)},
		{Type: "result", Result: "answer", SessionID: "s"},
	}}
	m := &ChatModel{client: fake}

	var thoughts string
	_, gotThoughts, _, err := m.ReplyStream(context.Background(), "", []*schema.Message{{Role: schema.User, Content: "hi"}}, func(c app.ReplyChunk) {
		thoughts += c.Thought
	})
	if err != nil {
		t.Fatalf("ReplyStream: %v", err)
	}
	if gotThoughts != "quiet reasoning" || thoughts != "quiet reasoning" {
		t.Errorf("thoughts = (%q returned, %q streamed), want %q", gotThoughts, thoughts, "quiet reasoning")
	}
}
