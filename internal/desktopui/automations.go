package desktopui

import (
	"context"
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/cerebrai-app/urban-carnival/internal/workerclient"
)

// newAutomationsView builds the automation management surface: the list of
// automations the worker owns, their triggers, and enable/disable controls
// (DESIGN.md §2, §4).
func newAutomationsView(ctx context.Context, client workerclient.Client) fyne.CanvasObject {
	var automations []workerclient.Automation

	list := widget.NewList(
		func() int { return len(automations) },
		func() fyne.CanvasObject {
			name := widget.NewLabel("")
			name.TextStyle = fyne.TextStyle{Bold: true}
			trigger := widget.NewLabel("")
			enabled := widget.NewCheck("Enabled", nil)
			return container.NewBorder(nil, nil, nil, enabled,
				container.NewVBox(name, trigger))
		},
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			a := automations[i]
			row := obj.(*fyne.Container)
			labels := row.Objects[0].(*fyne.Container)
			labels.Objects[0].(*widget.Label).SetText(a.Name)
			labels.Objects[1].(*widget.Label).SetText(a.Description + "  —  " + a.Trigger)

			enabled := row.Objects[1].(*widget.Check)
			enabled.OnChanged = nil // avoid firing while we set the initial state
			enabled.SetChecked(a.Enabled)
			enabled.OnChanged = func(checked bool) {
				id := a.ID
				go func() {
					if err := client.SetAutomationEnabled(ctx, id, checked); err != nil {
						slog.Error("set automation enabled", "id", id, "enabled", checked, "error", err)
						return
					}
					if checked {
						slog.Info("automation enabled", "id", id)
					} else {
						slog.Info("automation disabled", "id", id)
					}
				}()
			}
		},
	)

	refresh := func() {
		slog.Info("refreshing automations")
		result, err := client.ListAutomations(ctx)
		if err != nil {
			slog.Error("list automations", "error", err)
			return
		}
		slog.Info("refreshed automations", "count", len(result))
		fyne.Do(func() {
			automations = result
			list.Refresh()
		})
	}
	go refresh()

	refreshButton := widget.NewButton("Refresh", func() { go refresh() })
	toolbar := container.NewHBox(refreshButton)

	return container.NewBorder(toolbar, nil, nil, nil, list)
}
