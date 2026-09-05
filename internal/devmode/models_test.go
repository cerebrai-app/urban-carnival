package devmode

import (
	"testing"

	"github.com/cerebrai-app/urban-carnival/internal/devmode/claudecode"
)

func TestDefaultChatModel(t *testing.T) {
	t.Setenv(EnvDevMode, "true")
	if got := DefaultChatModel(); got != ModelClaudeCode {
		t.Errorf("DefaultChatModel() = %q, want %q", got, ModelClaudeCode)
	}

	t.Setenv(EnvDevMode, "false")
	if got := DefaultChatModel(); got != "" {
		t.Errorf("DefaultChatModel() = %q, want empty", got)
	}
}

func TestAvailableChatModels(t *testing.T) {
	t.Setenv(EnvDevMode, "true")
	if got := AvailableChatModels(); len(got) != 1 || got[0] != ModelClaudeCode {
		t.Errorf("AvailableChatModels() = %v, want [%q]", got, ModelClaudeCode)
	}

	t.Setenv(EnvDevMode, "false")
	if got := AvailableChatModels(); len(got) != 0 {
		t.Errorf("AvailableChatModels() = %v, want empty", got)
	}
}

func TestAgentModel(t *testing.T) {
	t.Setenv(EnvDevMode, "true")
	if got := AgentModel(); got != ModelClaudeCode {
		t.Errorf("AgentModel() = %q, want %q", got, ModelClaudeCode)
	}

	t.Setenv(EnvDevMode, "false")
	if got := AgentModel(); got != "" {
		t.Errorf("AgentModel() = %q, want empty", got)
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
