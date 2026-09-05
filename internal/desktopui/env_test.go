package desktopui

import (
	"os"
	"testing"
)

func TestLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  string
	}{
		{name: "unset", want: defaultLogLevel},
		{name: "empty", set: true, value: "", want: defaultLogLevel},
		{name: "debug", set: true, value: "debug", want: "debug"},
		{name: "error", set: true, value: "error", want: "error"},
		{name: "unknown falls back", set: true, value: "trace", want: defaultLogLevel},
		{name: "case sensitive", set: true, value: "DEBUG", want: defaultLogLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(envLogLevel, tt.value)
			} else {
				unset(t, envLogLevel)
			}
			if got := logLevel(); got != tt.want {
				t.Errorf("logLevel() = %q, want %q", got, tt.want)
			}
		})
	}
}

// unset clears key for the duration of the test. t.Setenv already restores
// the previous value at cleanup, so setting then clearing is enough.
func unset(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}
