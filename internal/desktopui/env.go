package desktopui

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/cerebrai-app/urban-carnival/internal/config"
)

// envLogLevel sets the minimum slog level the app emits. Unset or empty
// means defaultLogLevel. It is a developer control rather than a user
// preference: read once at startup, not persisted alongside the user's
// settings, and documented in README.md.
const envLogLevel = config.EnvLogLevel

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
