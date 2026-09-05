package config

import (
	"os"
	"testing"
)

func TestDevEnabled(t *testing.T) {
	tests := []struct {
		name  string
		set   bool
		value string
		want  bool
	}{
		{name: "unset", want: false},
		{name: "empty", set: true, value: "", want: false},
		{name: "1", set: true, value: "1", want: true},
		{name: "true", set: true, value: "true", want: true},
		{name: "0", set: true, value: "0", want: false},
		{name: "false", set: true, value: "false", want: false},
		{name: "non-boolean stays disabled", set: true, value: "yes please", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv(EnvDevMode, tt.value)
			} else {
				// t.Setenv restores the previous value at cleanup, so setting
				// then unsetting is enough to exercise LookupEnv's !ok branch.
				t.Setenv(EnvDevMode, "")
				if err := os.Unsetenv(EnvDevMode); err != nil {
					t.Fatalf("unset %s: %v", EnvDevMode, err)
				}
			}
			if got := DevEnabled(); got != tt.want {
				t.Errorf("DevEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
