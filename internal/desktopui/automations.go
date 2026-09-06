package desktopui

import (
	"context"
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/cerebrai-app/urban-carnival/internal/app"
)

// newAutomationsView builds the automation management surface: the list of
// automations the engine owns, their triggers, and enable/disable controls
// (DESIGN.md §2, §4).
func newAutomationsView(ctx context.Context, client app.Client) fyne.CanvasObject {
	// automations backs the list and is only ever touched on the Fyne main
	// goroutine, via fyne.Do from the background calls below.
	var automations []app.Automation

	// setEnabled updates the cached state for one automation and re-binds the
	// list so the row reflects it. Main goroutine only.
	var list *widget.List
	setEnabled := func(id string, enabled bool) {
		for i := range automations {
			if automations[i].ID == id {
				automations[i].Enabled = enabled
				list.Refresh()
				return
			}
		}
	}

	list = widget.NewList(
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
						// The engine rejected the change, so put the row back
						// the way it was rather than leaving the UI claiming a
						// state the engine does not have.
						fyne.Do(func() { setEnabled(id, !checked) })
						return
					}
					if checked {
						slog.Info("automation enabled", "id", id)
					} else {
						slog.Info("automation disabled", "id", id)
					}
					// Record the new state locally: the list re-binds rows from
					// this slice on every scroll and Refresh, so without this the
					// checkbox snaps back to the stale value.
					fyne.Do(func() { setEnabled(id, checked) })
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
