// Package devmode gathers the behavior that is active only in a developer's
// checkout rather than a real build: the CEREBRAI_DEV_MODE gate itself
// (Enabled) and the local Claude Code model wiring that gate turns on
// (internal/devmode/claudecode, surfaced here as DefaultModel /
// AvailableModels / Provider). The cerebrai_dev build tag's raw chat-content
// logging lives in internal/telemetry.
package devmode

import (
	"fmt"
	"os"
	"strconv"
)

// EnvDevMode, when set to a value strconv.ParseBool accepts as true (1, t,
// true), means cerebrai is running in a developer's checkout rather than a
// real build. Unset, empty, or false means production behavior.
//
// It gates more than one thing: internal/desktopui shows the desktop app's
// Developer preferences section when it's set, internal/storage keeps the
// SQLite database in the repo instead of the OS's per-user application data
// directory, and the local Claude Code model (see models.go) is offered only
// when it's set. Callers use Enabled below rather than parsing it themselves.
const EnvDevMode = "CEREBRAI_DEV_MODE"

// Enabled reports whether EnvDevMode is set to a value strconv.ParseBool
// accepts as true (1, t, true). Unset, empty, or false all mean false; an
// unparsable value also means false, but is warned about on stderr since a
// typo'd config value should not fail silently.
func Enabled() bool {
	value, ok := os.LookupEnv(EnvDevMode)
	if !ok || value == "" {
		return false
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %q is not a boolean, treating it as disabled\n", EnvDevMode, value)
		return false
	}
	return enabled
}
