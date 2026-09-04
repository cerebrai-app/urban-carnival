// Package desktopui implements cerebrai's native macOS desktop UI: a
// chat surface for conversing with the assistant and an automation
// management surface, per DESIGN.md §3.
package desktopui

import (
	"context"
	"log/slog"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/cerebrai-app/urban-carnival/internal/telemetry"
	"github.com/cerebrai-app/urban-carnival/internal/version"
	"github.com/cerebrai-app/urban-carnival/internal/workerclient"
)

// App is the desktop application: a native window over the background
// worker's local API. It holds no automation, memory, or LLM logic itself.
type App struct {
	fyneApp fyne.App
	window  fyne.Window
	client  workerclient.Client
	ctx     context.Context

	preferencesWindow fyne.Window

	telemetryMu       sync.Mutex
	telemetryShutdown telemetry.Shutdown
}

// New builds the desktop App, wiring it to client.
func New(client workerclient.Client) *App {
	fyneApp := app.NewWithID("app.cerebrai.desktop")
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
	a.window.ShowAndRun()
}

func (a *App) buildMainMenu() *fyne.MainMenu {
	preferencesItem := fyne.NewMenuItem("Preferences…", a.showPreferencesWindow)
	preferencesItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierSuper}

	// The first menu becomes the app menu on macOS.
	return fyne.NewMainMenu(fyne.NewMenu("cerebrai", preferencesItem))
}

// applyTelemetry (re)configures global telemetry export based on the
// current OTLP preference, replacing any previously running providers.
func (a *App) applyTelemetry() {
	otlp := a.fyneApp.Preferences().Bool(prefOTLPKey)

	a.telemetryMu.Lock()
	defer a.telemetryMu.Unlock()

	if a.telemetryShutdown != nil {
		if err := a.telemetryShutdown(a.ctx); err != nil {
			slog.Warn("telemetry shutdown", "error", err)
		}
		a.telemetryShutdown = nil
	}

	shutdown, err := telemetry.Setup(a.ctx, "cerebrai-desktop", version.Version, telemetry.Options{OTLP: otlp})
	if err != nil {
		slog.Error("setup telemetry", "error", err)
		return
	}
	a.telemetryShutdown = shutdown
}

func (a *App) shutdownTelemetry() {
	a.telemetryMu.Lock()
	defer a.telemetryMu.Unlock()

	if a.telemetryShutdown == nil {
		return
	}
	if err := a.telemetryShutdown(a.ctx); err != nil {
		slog.Warn("telemetry shutdown", "error", err)
	}
	a.telemetryShutdown = nil
}
