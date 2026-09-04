// Command cerebrai-desktop is cerebrai's native macOS desktop app: the
// chat and automation management surface described in DESIGN.md §3. It
// talks to the background worker over a local API and holds no
// automation, memory, or LLM logic of its own.
package main

import (
	"context"

	"github.com/cerebrai-app/urban-carnival/internal/desktopui"
	"github.com/cerebrai-app/urban-carnival/internal/workerclient"
)

func main() {
	// TODO: replace with a client that talks to the background worker's
	// local API once its transport is decided (DESIGN.md §3, §9).
	client := workerclient.NewMock()

	desktopui.New(client).Run(context.Background())
}
