package desktopui

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Environment variables configuring the desktop app. These are developer
// controls rather than user preferences: they are read at startup, are not
// persisted alongside the user's settings, and are documented in README.md.
const (
	// envLogLevel sets the minimum slog level the app emits. Unset or empty
	// means defaultLogLevel.
	envLogLevel = "CEREBRAI_LOG_LEVEL"

	// envDevSettings reveals the Developer section of the Preferences window
	// when set to a value strconv.ParseBool accepts as true (1, t, true).
	// Unset, empty, or false means hidden.
	envDevSettings = "CEREBRAI_DEV_SETTINGS"
)

// defaultLogLevel matches the CLI's --log-level default.
const defaultLogLevel = "info"

// logLevelOptions are the values accepted by telemetry.Setup, matching the
// CLI's --log-level flag description.
var logLevelOptions = []string{"debug", "info", "warn", "error"}

// logLevel returns the slog level from envLogLevel, falling back to
// defaultLogLevel when unset or unrecognized.
func logLevel() string {
	level, ok := os.LookupEnv(envLogLevel)
	if !ok || level == "" {
		return defaultLogLevel
	}
	if !slices.Contains(logLevelOptions, level) {
		// Straight to stderr: this runs before telemetry.Setup has installed
		// a logger, so slog.Default() is not yet the app's configured one.
		fmt.Fprintf(os.Stderr, "%s: unknown log level %q, using %q (valid: %s)\n",
			envLogLevel, level, defaultLogLevel, strings.Join(logLevelOptions, ", "))
		return defaultLogLevel
	}
	return level
}

// devSettingsEnabled reports whether the Preferences window should show its
// Developer section.
func devSettingsEnabled() bool {
	value, ok := os.LookupEnv(envDevSettings)
	if !ok || value == "" {
		return false
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %q is not a boolean, developer settings stay hidden\n",
			envDevSettings, value)
		return false
	}
	return enabled
}
