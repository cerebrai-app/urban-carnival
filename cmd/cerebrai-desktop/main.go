// Command cerebrai-desktop is cerebrai's native macOS desktop app: the
// chat and automation management surface described in DESIGN.md §3. It
// talks to the background worker over a local API and holds no
// automation, memory, or LLM logic of its own.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/cerebrai-app/urban-carnival/internal/desktopui"
	"github.com/cerebrai-app/urban-carnival/internal/storage"
	"github.com/cerebrai-app/urban-carnival/internal/workerclient"
)

func main() {
	ctx := context.Background()

	// TODO: replace with a client that talks to the background worker's
	// local API once its transport is decided (DESIGN.md §3, §9). Until then
	// automations are persisted locally via SQLite (internal/storage), so
	// they survive a restart; see workerclient.SQLite.
	db, err := storage.Open(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open database:", err)
		os.Exit(1)
	}

	defer closeDB(db)

	desktopui.New(workerclient.NewSQLite(db)).Run(ctx)
}

func closeDB(db *sql.DB) {
	if err := db.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "close database:", err)
	}
}
