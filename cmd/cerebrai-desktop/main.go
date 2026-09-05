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

	"github.com/cerebrai-app/urban-carnival/internal/automationagent"
	"github.com/cerebrai-app/urban-carnival/internal/desktopui"
	"github.com/cerebrai-app/urban-carnival/internal/devmode"
	"github.com/cerebrai-app/urban-carnival/internal/devmode/devmcp"
	"github.com/cerebrai-app/urban-carnival/internal/storage"
)

func main() {
	ctx := context.Background()

	// TODO: replace with a client that talks to the background worker's
	// local API once its transport is decided (DESIGN.md §3, §9). Until then
	// automations are persisted locally via SQLite (internal/storage), so
	// they survive a restart; see storage.SQLite.
	db, err := storage.Open(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open database:", err)
		os.Exit(1)
	}
	defer closeDB(db)

	client := storage.NewSQLite(db)

	// In developer builds the chat model is the local Claude Code CLI, which
	// reaches cerebrai's tools (create_automation, edit_automation) only over
	// MCP. Start the in-process MCP server and point the chat provider at it
	// (DESIGN.md §5.6). A failure here is not fatal: the app still runs, dev
	// chat just can't author automations.
	if devmode.Enabled() {
		srv, err := devmcp.Start(ctx, devmcp.Deps{Store: client, Writer: automationagent.Provider()})
		if err != nil {
			fmt.Fprintln(os.Stderr, "start dev MCP server:", err)
		} else {
			defer func() { _ = srv.Close(context.Background()) }()
			devmode.SetMCPBridge(srv)
		}
	}

	desktopui.New(client).Run(ctx)
}

func closeDB(db *sql.DB) {
	if err := db.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "close database:", err)
	}
}
