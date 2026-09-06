package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/cerebrai-app/urban-carnival/internal/devmode"
	"github.com/cerebrai-app/urban-carnival/internal/devmode/claudecode"
)

// TestChatModelImplementsConversationProvider keeps claudecode.ChatModel
// wired to this seam. devmode.ChatProvider hands it back as a concrete
// *claudecode.ChatModel, so this is where a future divergence between
// ConversationProvider and that type would first fail to compile. It lives
// here, in the in-package test, because chat already imports claudecode
// transitively via devmode — no external test package needed.
func TestChatModelImplementsConversationProvider(_ *testing.T) {
	var _ ConversationProvider = (*claudecode.ChatModel)(nil)
}

func TestUnconfigured(t *testing.T) {
	ctx := context.Background()
	var p ConversationProvider = Unconfigured{}

	if _, _, err := p.Reply(ctx, "", nil); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Reply error = %v, want ErrNotConfigured", err)
	}
}

func TestDefaultModelDev(t *testing.T) {
	t.Setenv(devmode.EnvDevMode, "true")
	if got := DefaultModel(); got != devmode.ModelClaudeCode {
		t.Errorf("DefaultModel() = %q, want %q", got, devmode.ModelClaudeCode)
	}
	if got := AvailableModels(); len(got) != 1 || got[0] != devmode.ModelClaudeCode {
		t.Errorf("AvailableModels() = %v, want [%q]", got, devmode.ModelClaudeCode)
	}
}

func TestDefaultModelProd(t *testing.T) {
	t.Setenv(devmode.EnvDevMode, "false")
	if got := DefaultModel(); got != "" {
		t.Errorf("DefaultModel() = %q, want empty", got)
	}
	if got := AvailableModels(); len(got) != 0 {
		t.Errorf("AvailableModels() = %v, want empty", got)
	}
}

func TestProviderFor(t *testing.T) {
	if _, ok := ProviderFor(devmode.ModelClaudeCode).(*claudecode.ChatModel); !ok {
		t.Errorf("ProviderFor(%q) = %T, want *claudecode.ChatModel", devmode.ModelClaudeCode, ProviderFor(devmode.ModelClaudeCode))
	}
	if _, ok := ProviderFor("").(Unconfigured); !ok {
		t.Errorf("ProviderFor(\"\") = %T, want Unconfigured", ProviderFor(""))
	}
	if _, ok := ProviderFor("nonexistent-model").(Unconfigured); !ok {
		t.Errorf("ProviderFor(unknown) = %T, want Unconfigured", ProviderFor("nonexistent-model"))
	}
}

func TestDefaultProvider(t *testing.T) {
	t.Setenv(devmode.EnvDevMode, "true")
	if _, ok := DefaultProvider().(*claudecode.ChatModel); !ok {
		t.Errorf("DefaultProvider() = %T, want *claudecode.ChatModel", DefaultProvider())
	}

	t.Setenv(devmode.EnvDevMode, "false")
	if _, ok := DefaultProvider().(Unconfigured); !ok {
		t.Errorf("DefaultProvider() = %T, want Unconfigured", DefaultProvider())
	}
}
