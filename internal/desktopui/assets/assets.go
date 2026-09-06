// Package assets embeds the desktop UI's static assets. icon.png is the
// full-color cerebrai brain-network mark (a copy of build/macos/icon.png,
// which build/macos/package-app.sh renders into CerebrAI.app's icon.icns).
//
// Icon is used as-is for the app/window/Dock/taskbar icon. TrayIcon wraps it
// in a themed resource so Fyne hands macOS a template NSImage for the menu-bar
// tray: the system tints it to match the menu bar in both light and dark mode,
// instead of a fixed color that vanishes against one of them.
package assets

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

//go:embed icon.png
var iconPNG []byte

// Icon is cerebrai's full-color app icon, a 512×512 PNG.
var Icon fyne.Resource = fyne.NewStaticResource("icon.png", iconPNG)

// TrayIcon is Icon flagged as a *theme.ThemedResource, which is Fyne's cue to
// hand macOS a template (theme-adaptive) tray icon via systray.SetTemplateIcon;
// the OS then tints the mark's alpha silhouette to the menu bar in either
// appearance. Theme recoloring itself is a no-op here (that path only rewrites
// SVG fills, and this is a PNG), so Fyne logs one "falling back to static
// content" line the first time it reads the resource — harmless.
var TrayIcon fyne.Resource = theme.NewThemedResource(Icon)
