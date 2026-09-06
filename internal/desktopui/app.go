// Package desktopui implements cerebrai's native macOS desktop UI: a
// chat surface for conversing with the assistant and an automation
// management surface, per DESIGN.md §3.
package desktopui

import (
	"context"
	"fmt"
	"os"
	"sync"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/cerebrai-app/urban-carnival/internal/app"
	"github.com/cerebrai-app/urban-carnival/internal/config"
	"github.com/cerebrai-app/urban-carnival/internal/desktopui/assets"
	"github.com/cerebrai-app/urban-carnival/internal/telemetry"
)

// App is the desktop application: a native window over the app.Client port
// (DESIGN.md §3). It holds no automation, memory, or LLM logic itself; the
// engine behind the port runs in-process in the same binary.
type App struct {
	fyneApp fyne.App
	window  fyne.Window
	client  app.Client
	ctx     context.Context

	preferencesWindow fyne.Window

	telemetryMu       sync.Mutex
	telemetryShutdown telemetry.Shutdown
}

// New builds the desktop App, wiring it to client.
func New(client app.Client) *App {
	fyneApp := fyneapp.NewWithID("app.cerebrai.desktop")
	fyneApp.SetIcon(assets.Icon)
	window := fyneApp.NewWindow("cerebrai")
	window.Resize(fyne.NewSize(960, 640))

	return &App{fyneApp: fyneApp, window: window, client: client}
}

// Run builds the window content and blocks until the window is closed.
func (a *App) Run(ctx context.Context) {
	a.ctx = ctx
	a.applyTelemetry()
	a.fyneApp.Lifecycle().SetOnStopped(func() { a.shutdownTelemetry() })

	chat := newChatView(ctx, a.client)
	automations := newAutomationsView(ctx, a.client)

	tabs := container.NewAppTabs(
		container.NewTabItem("Chat", chat),
		container.NewTabItem("Automations", automations),
	)
	tabs.SetTabLocation(container.TabLocationLeading)

	a.window.SetContent(tabs)
	a.window.SetMainMenu(a.buildMainMenu())
	a.setupSystemTray()

	// Closing the window hides it rather than quitting; the app keeps
	// running in the background and is only fully quit from the tray menu.
	a.window.SetCloseIntercept(a.window.Hide)

	a.window.ShowAndRun()
}

// setupSystemTray adds a taskbar/menu-bar icon with a menu to reopen the
// window or quit the app outright. Closing the window only hides it, so the
// tray menu is the sole way to quit.
func (a *App) setupSystemTray() {
	desk, ok := a.fyneApp.(desktop.App)
	if !ok {
		return
	}

	showItem := fyne.NewMenuItem("Show cerebrai", func() {
		a.window.Show()
		a.window.RequestFocus()
	})
	trayMenu := fyne.NewMenu("cerebrai", showItem, fyne.NewMenuItemSeparator(), fyne.NewMenuItem("Quit", a.fyneApp.Quit))

	desk.SetSystemTrayMenu(trayMenu)
	desk.SetSystemTrayIcon(assets.TrayIcon)
}

func (a *App) buildMainMenu() *fyne.MainMenu {
	preferencesItem := fyne.NewMenuItem("Preferences…", a.showPreferencesWindow)
	preferencesItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierSuper}
	quitItem := fyne.NewMenuItem("Quit cerebrai", a.fyneApp.Quit)
	quitItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyQ, Modifier: fyne.KeyModifierSuper}
	quitItem.IsQuit = true

	// The first menu becomes the app menu on macOS.
	return fyne.NewMainMenu(fyne.NewMenu("cerebrai", preferencesItem, fyne.NewMenuItemSeparator(), quitItem))
}

// applyTelemetry (re)configures global telemetry export based on the
// current OTLP and log level preferences, replacing any previously running
// providers.
func (a *App) applyTelemetry() {
	// Held across the whole reconfiguration so two preference toggles in
	// quick succession cannot interleave and leave telemetryShutdown
	// pointing at providers other than the globally installed ones.
	a.telemetryMu.Lock()
	defer a.telemetryMu.Unlock()

	otlp := a.fyneApp.Preferences().Bool(prefOTLPKey)

	// Install the new providers before tearing the old ones down. Setup
	// replaces the global tracer/meter/logger and slog.Default(), so on
	// failure the previous configuration simply keeps running; shutting down
	// first would leave slog.Default() bound to dead providers and silently
	// drop every log line from here on, including this error.
	// Without OTLP the desktop app still prints spans and metrics to stderr,
	// visible when it's launched from a terminal (make run-desktop).
	shutdown, err := telemetry.Setup(a.ctx, "cerebrai-desktop", config.Version, telemetry.Options{OTLP: otlp, PrintToStderr: true, LogLevel: logLevel()})
	if err != nil {
		reportTelemetryProblem("setup telemetry", err)
		return
	}

	previous := a.telemetryShutdown
	a.telemetryShutdown = shutdown

	if previous != nil {
		if err := previous(a.ctx); err != nil {
			reportTelemetryProblem("telemetry shutdown", err)
		}
	}
}

func (a *App) shutdownTelemetry() {
	a.telemetryMu.Lock()
	defer a.telemetryMu.Unlock()

	if a.telemetryShutdown == nil {
		return
	}
	if err := a.telemetryShutdown(a.ctx); err != nil {
		reportTelemetryProblem("telemetry shutdown", err)
	}
	a.telemetryShutdown = nil
}

// reportTelemetryProblem prints a telemetry failure directly to stderr rather
// than logging it via slog. In OTLP mode slog.Default() ships records through
// the very providers being set up or torn down here, so a slog call could be
// silently lost — the same reasoning as internal/cli/root.go. A telemetry
// backend being unreachable must never be invisible.
func reportTelemetryProblem(what string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", what, err)
}
