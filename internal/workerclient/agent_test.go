package workerclient

import (
	"context"
	"testing"
	"time"

	"github.com/cerebrai-app/urban-carnival/internal/config"
	"github.com/cerebrai-app/urban-carnival/internal/model"
	"github.com/cerebrai-app/urban-carnival/internal/model/claudecode"
	"github.com/cloudwego/eino/schema"
)

func TestToSchemaMessages(t *testing.T) {
	history := []Message{
		{ID: "1", SessionID: "s", Role: "user", Content: "hi", CreatedAt: time.Now()},
		{ID: "2", SessionID: "s", Role: "assistant", Content: "hello", CreatedAt: time.Now()},
	}

	got := toSchemaMessages(history)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Role != schema.User || got[0].Content != "hi" {
		t.Errorf("got[0] = %+v, want role %q content %q", got[0], schema.User, "hi")
	}
	if got[1].Role != schema.Assistant || got[1].Content != "hello" {
		t.Errorf("got[1] = %+v, want role %q content %q", got[1], schema.Assistant, "hello")
	}
}

func TestNewAgentLoop(t *testing.T) {
	if _, err := NewAgentLoop(context.Background(), model.Unconfigured{}); err != nil {
		t.Fatalf("NewAgentLoop: %v", err)
	}
}

func TestDefaultProviderDev(t *testing.T) {
	t.Setenv(config.EnvDevSettings, "true")

	if _, ok := DefaultProvider().(*claudecode.ChatModel); !ok {
		t.Errorf("DefaultProvider() = %T, want *claudecode.ChatModel", DefaultProvider())
	}
}

func TestDefaultProviderProd(t *testing.T) {
	t.Setenv(config.EnvDevSettings, "false")

	if _, ok := DefaultProvider().(model.Unconfigured); !ok {
		t.Errorf("DefaultProvider() = %T, want model.Unconfigured", DefaultProvider())
	}
}

func TestDefaultModel(t *testing.T) {
	t.Setenv(config.EnvDevSettings, "true")
	if got := DefaultModel(); got != ModelClaudeCode {
		t.Errorf("DefaultModel() = %q, want %q", got, ModelClaudeCode)
	}

	t.Setenv(config.EnvDevSettings, "false")
	if got := DefaultModel(); got != "" {
		t.Errorf("DefaultModel() = %q, want empty", got)
	}
}

func TestAvailableModels(t *testing.T) {
	t.Setenv(config.EnvDevSettings, "true")
	if got := AvailableModels(); len(got) != 1 || got[0] != ModelClaudeCode {
		t.Errorf("AvailableModels() = %v, want [%q]", got, ModelClaudeCode)
	}

	t.Setenv(config.EnvDevSettings, "false")
	if got := AvailableModels(); len(got) != 0 {
		t.Errorf("AvailableModels() = %v, want empty", got)
	}
}

func TestProviderFor(t *testing.T) {
	if _, ok := ProviderFor(ModelClaudeCode).(*claudecode.ChatModel); !ok {
		t.Errorf("ProviderFor(%q) = %T, want *claudecode.ChatModel", ModelClaudeCode, ProviderFor(ModelClaudeCode))
	}
	if _, ok := ProviderFor("").(model.Unconfigured); !ok {
		t.Errorf("ProviderFor(\"\") = %T, want model.Unconfigured", ProviderFor(""))
	}
	if _, ok := ProviderFor("nonexistent-model").(model.Unconfigured); !ok {
		t.Errorf("ProviderFor(unknown) = %T, want model.Unconfigured", ProviderFor("nonexistent-model"))
	}
}
