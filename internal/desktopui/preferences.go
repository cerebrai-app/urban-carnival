package desktopui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/cerebrai-app/urban-carnival/internal/config"
)

// prefOTLPKey is the fyne.Preferences key for the OTLP telemetry toggle.
const prefOTLPKey = "otlp"

// showPreferencesWindow opens (or focuses) the app's preferences window.
func (a *App) showPreferencesWindow() {
	if a.preferencesWindow != nil {
		a.preferencesWindow.Show()
		a.preferencesWindow.RequestFocus()
		return
	}

	w := a.fyneApp.NewWindow("Preferences")
	w.Resize(fyne.NewSize(440, 200))

	content := container.NewVBox()
	if config.DevEnabled() {
		content.Add(a.developerSettings())
	} else {
		// Everything here is currently a developer control. User-facing
		// preferences arrive with the background worker.
		content.Add(widget.NewLabel("No preferences yet."))
	}

	w.SetContent(content)
	w.SetOnClosed(func() { a.preferencesWindow = nil })

	a.preferencesWindow = w
	w.Show()
}

// developerSettings builds the Developer section, shown only when
// config.EnvDevMode is set. These are diagnostic controls, not things a
// user should have to reason about.
func (a *App) developerSettings() fyne.CanvasObject {
	heading := widget.NewLabel("Developer")
	heading.TextStyle = fyne.TextStyle{Bold: true}

	otlpCheck := widget.NewCheck("Export telemetry via OTLP", nil)
	otlpCheck.SetChecked(a.fyneApp.Preferences().Bool(prefOTLPKey))
	// Assigned after SetChecked so restoring the saved state does not itself
	// trigger a telemetry restart.
	otlpCheck.OnChanged = func(checked bool) {
		a.fyneApp.Preferences().SetBool(prefOTLPKey, checked)
		go a.applyTelemetry()
	}

	otlpHelp := widget.NewLabel(
		"When enabled, spans and metrics are exported via OTLP/gRPC instead of\n" +
			"printed to stderr. Configure the destination with the standard\n" +
			"OTEL_EXPORTER_OTLP_* environment variables (defaults to localhost:4317).",
	)
	otlpHelp.Wrapping = fyne.TextWrapWord

	// The log level is deliberately absent: it is fixed at startup by
	// CEREBRAI_LOG_LEVEL, so the UI neither sets it nor reports it.
	section := container.NewVBox(
		heading,
		widget.NewSeparator(),
		otlpCheck,
		otlpHelp,
	)

	// A cerebrai_dev build logs raw conversation content at debug level. Say
	// so plainly: these logs leave the machine in OTLP mode, and a dev build
	// is otherwise indistinguishable from a normal one.
	if chatContentLogging {
		warning := widget.NewLabel(
			"Development build: at the debug log level, full chat message and " +
				"reply text is written to the logs, which are exported off this " +
				"machine when OTLP is enabled.",
		)
		warning.Wrapping = fyne.TextWrapWord
		warning.Importance = widget.WarningImportance
		section.Add(warning)
	}

	return section
}
