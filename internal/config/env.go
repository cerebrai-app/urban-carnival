package config

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
