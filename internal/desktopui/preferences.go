package desktopui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// prefOTLPKey is the fyne.Preferences key for the OTLP telemetry toggle.
const prefOTLPKey = "otlp"

// prefLogLevelKey is the fyne.Preferences key for the slog level, mirroring
// the cerebrai CLI's --log-level flag (internal/cli/root.go).
const prefLogLevelKey = "logLevel"

// defaultLogLevel matches the CLI's --log-level default.
const defaultLogLevel = "info"

// logLevelOptions are the values accepted by telemetry.Setup, matching the
// CLI's --log-level flag description (debug, info, warn, error).
var logLevelOptions = []string{"debug", "info", "warn", "error"}

// logLevel returns the configured slog level, defaulting to defaultLogLevel.
func (a *App) logLevel() string {
	level := a.fyneApp.Preferences().StringWithFallback(prefLogLevelKey, defaultLogLevel)
	if level == "" {
		return defaultLogLevel
	}
	return level
}

// showPreferencesWindow opens (or focuses) the app's preferences window.
func (a *App) showPreferencesWindow() {
	if a.preferencesWindow != nil {
		a.preferencesWindow.RequestFocus()
		return
	}

	w := a.fyneApp.NewWindow("Preferences")
	w.Resize(fyne.NewSize(440, 200))

	otlpCheck := widget.NewCheck("Export telemetry via OTLP", nil)
	otlpCheck.SetChecked(a.fyneApp.Preferences().Bool(prefOTLPKey))
	otlpCheck.OnChanged = func(checked bool) {
		a.fyneApp.Preferences().SetBool(prefOTLPKey, checked)
		go a.applyTelemetry()
	}

	help := widget.NewLabel(
		"When enabled, spans and metrics are exported via OTLP/gRPC instead of\n" +
			"printed to stderr. Configure the destination with the standard\n" +
			"OTEL_EXPORTER_OTLP_* environment variables (defaults to localhost:4317).",
	)
	help.Wrapping = fyne.TextWrapWord

	logLevelSelect := widget.NewSelect(logLevelOptions, nil)
	logLevelSelect.SetSelected(a.logLevel())
	logLevelSelect.OnChanged = func(level string) {
		a.fyneApp.Preferences().SetString(prefLogLevelKey, level)
		go a.applyTelemetry()
	}

	w.SetContent(container.NewVBox(
		otlpCheck,
		help,
		widget.NewLabel("Log level"),
		logLevelSelect,
	))
	w.SetOnClosed(func() { a.preferencesWindow = nil })

	a.preferencesWindow = w
	w.Show()
}
