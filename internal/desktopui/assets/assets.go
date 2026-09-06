// Package assets embeds the desktop UI's static assets. icon-white.png is the
// all-white variant of cerebrai's brain-network mark, loaded at runtime for
// the window/Dock and system-tray icon so it stays legible on dark menu bars.
// The full-color source lives at build/macos/icon.png, which
// build/macos/package-app.sh renders into CerebrAI.app's icon.icns.
package assets

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed icon-white.png
var iconPNG []byte

// Icon is cerebrai's app icon, a 512×512 all-white PNG.
var Icon fyne.Resource = fyne.NewStaticResource("icon-white.png", iconPNG)
