package config

import (
	"fmt"
	"os"
	"strconv"
)

// EnvDevMode, when set to a value strconv.ParseBool accepts as true (1,
// t, true), means cerebrai is running in a developer's checkout rather than
// a real build. Unset, empty, or false means production behavior.
//
// It gates more than one thing: internal/desktopui shows the desktop app's
// Developer preferences section when it's set, and internal/storage keeps
// the SQLite database in the repo instead of the OS's per-user application
// data directory. Both call DevEnabled below rather than parsing it
// themselves.
const EnvDevMode = "CEREBRAI_DEV_MODE"

// EnvLogLevel sets the desktop app's minimum slog level (debug, info, warn,
// error). Unset or empty means internal/desktopui's default log level.
const EnvLogLevel = "CEREBRAI_LOG_LEVEL"

// EnvDBPath, when set to a non-empty value, is the exact path
// internal/storage.Path returns for cerebrai's SQLite database, overriding
// both the dev (repo-relative) and production (per-user app data) locations.
// The Makefile's install-macos target sets it to the checkout's cerebrai.db
// so a Finder-launched dev build, whose working directory is /, still finds
// that database instead of failing on an unwritable ./cerebrai.db.
const EnvDBPath = "CEREBRAI_DB_PATH"

// DevEnabled reports whether EnvDevMode is set to a value
// strconv.ParseBool accepts as true (1, t, true). Unset, empty, or false all
// mean false; an unparsable value also means false, but is warned about on
// stderr since a typo'd config value should not fail silently.
func DevEnabled() bool {
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
