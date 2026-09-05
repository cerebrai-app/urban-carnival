package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cerebrai-app/urban-carnival/internal/config"
)

// fileName is the SQLite database's file name, in either location Path
// resolves.
const fileName = "cerebrai.db"

// Path resolves the on-disk location of cerebrai's SQLite database.
//
// config.EnvDBPath, when set, wins outright: it is returned verbatim. This
// is how the Makefile's install-macos target pins a Finder-launched dev
// build (working directory /) to the checkout's database.
//
// Otherwise, with config.EnvDevMode set, the database lives at the repo
// root as fileName (relative to the working directory the command was
// launched from) so a developer can find, inspect, or delete it freely; it
// is gitignored rather than committed. The Makefile's run/run-desktop
// targets set it for exactly this. Otherwise this is a real build (make
// build/build-desktop, or a packaged release) meant to be run outside the
// repo, so the database goes under the OS's per-user application data
// directory instead.
func Path() (string, error) {
	if p := os.Getenv(config.EnvDBPath); p != "" {
		return p, nil
	}

	if config.DevEnabled() {
		return fileName, nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve application data directory: %w", err)
	}
	return filepath.Join(dir, "cerebrai", fileName), nil
}
