package desktopui

import (
	"context"
	"fmt"
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/cerebrai-app/urban-carnival/internal/workerclient"
)

// newChatView builds the conversational surface: the primary way users
// describe automations and query the memory store (DESIGN.md §2).
func newChatView(ctx context.Context, client workerclient.Client) fyne.CanvasObject {
	history := widget.NewRichTextFromMarkdown("")
	history.Wrapping = fyne.TextWrapWord
	historyScroll := container.NewVScroll(history)

	var transcript string
	appendLine := func(prefix, content string) {
		transcript += fmt.Sprintf("\n\n**%s:** %s", prefix, content)
		history.ParseMarkdown(transcript)
		historyScroll.ScrollToBottom()
	}

	input := widget.NewEntry()
	input.SetPlaceHolder("Ask cerebrai, or describe an automation…")

	send := func() {
		text := input.Text
		if text == "" {
			return
		}
		input.SetText("")
		appendLine("You", text)

		go func() {
			reply, err := client.SendMessage(ctx, text)
			fyne.Do(func() {
				if err != nil {
					slog.Error("send message", "error", err)
					appendLine("cerebrai", "(error contacting background worker)")
					return
				}
				slog.Debug("chat exchange", "input", text, "response", reply.Content)
				appendLine("cerebrai", reply.Content)
			})
		}()
	}
	input.OnSubmitted = func(string) { send() }

	sendButton := widget.NewButton("Send", send)

	inputRow := container.NewBorder(nil, nil, nil, sendButton, input)
	return container.NewBorder(nil, inputRow, nil, nil, historyScroll)
}
