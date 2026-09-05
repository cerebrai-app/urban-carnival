package devmode

import (
	"testing"

	"github.com/cerebrai-app/urban-carnival/internal/devmode/claudecode"
)

func TestDefaultModel(t *testing.T) {
	t.Setenv(EnvDevMode, "true")
	if got := DefaultModel(); got != ModelClaudeCode {
		t.Errorf("DefaultModel() = %q, want %q", got, ModelClaudeCode)
	}

	t.Setenv(EnvDevMode, "false")
	if got := DefaultModel(); got != "" {
		t.Errorf("DefaultModel() = %q, want empty", got)
	}
}

func TestAvailableModels(t *testing.T) {
	t.Setenv(EnvDevMode, "true")
	if got := AvailableModels(); len(got) != 1 || got[0] != ModelClaudeCode {
		t.Errorf("AvailableModels() = %v, want [%q]", got, ModelClaudeCode)
	}

	t.Setenv(EnvDevMode, "false")
	if got := AvailableModels(); len(got) != 0 {
		t.Errorf("AvailableModels() = %v, want empty", got)
	}
}

func TestProvider(t *testing.T) {
	if _, ok := Provider(ModelClaudeCode).(*claudecode.ChatModel); !ok {
		t.Errorf("Provider(%q) = %T, want *claudecode.ChatModel", ModelClaudeCode, Provider(ModelClaudeCode))
	}
	if got := Provider(""); got != nil {
		t.Errorf("Provider(\"\") = %T, want nil", got)
	}
	if got := Provider("nonexistent-model"); got != nil {
		t.Errorf("Provider(unknown) = %T, want nil", got)
	}
}
