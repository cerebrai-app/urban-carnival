package desktopui

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/cerebrai-app/urban-carnival/internal/app"
	"github.com/cerebrai-app/urban-carnival/internal/chat"
	"github.com/cerebrai-app/urban-carnival/internal/telemetry"
)

// newChatView builds the conversational surface: the primary way users
// describe automations and query the memory store (DESIGN.md §2). A session
// list down the left lets a user hold several conversations at once, each
// with its own persisted history; the transcript and input on the right
// operate on whichever session is selected.
//
// All of the state below (sessions, messages, currentSessionID) is only
// ever touched on the Fyne main goroutine, via fyne.Do from the background
// client calls.
func newChatView(ctx context.Context, client app.Client) fyne.CanvasObject {
	var sessions []app.Session
	var messages []app.Message
	var currentSessionID string

	history := widget.NewRichTextFromMarkdown("")
	history.Wrapping = fyne.TextWrapWord
	historyScroll := container.NewVScroll(history)

	refreshHistory := func() {
		var transcript string
		for _, m := range messages {
			prefix := "cerebrai"
			if m.Role == "user" {
				prefix = "You"
			}
			transcript += fmt.Sprintf("\n\n**%s:** %s", prefix, m.Content)
		}
		history.ParseMarkdown(transcript)
		historyScroll.ScrollToBottom()
	}

	sessionList := widget.NewList(
		func() int { return len(sessions) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(sessions[i].Title)
		},
	)

	// modelSelect lets the user pick which model the current session's
	// replies are generated with (DESIGN.md §5's provider abstraction).
	// suppressModelChange guards SetSelected calls that merely reflect the
	// newly activated session's stored model, so they don't themselves
	// trigger a (redundant) SetSessionModel write.
	var suppressModelChange bool
	modelSelect := widget.NewSelect(chat.AvailableModels(), func(selected string) {
		if suppressModelChange || currentSessionID == "" {
			return
		}
		sessionID := currentSessionID
		for i := range sessions {
			if sessions[i].ID == sessionID {
				sessions[i].Model = selected
			}
		}
		go func() {
			if err := client.SetSessionModel(ctx, sessionID, selected); err != nil {
				slog.Error("set session model", "session_id", sessionID, "error", err)
			}
		}()
	})
	modelSelect.PlaceHolder = "Model"

	// activateSession fetches a session's messages and, once loaded, makes
	// it the active conversation. A response is discarded if the user has
	// since switched to a different session. Idempotent: a no-op if session
	// is already current, so it's safe to call unconditionally alongside
	// sessionList.Select.
	activateSession := func(session app.Session) {
		if session.ID == currentSessionID {
			return
		}
		currentSessionID = session.ID
		messages = nil
		refreshHistory()
		suppressModelChange = true
		modelSelect.SetSelected(session.Model)
		suppressModelChange = false
		go func() {
			result, err := client.ListMessages(ctx, session.ID)
			if err != nil {
				slog.Error("list messages", "session_id", session.ID, "error", err)
				return
			}
			fyne.Do(func() {
				if currentSessionID != session.ID {
					return
				}
				messages = result
				refreshHistory()
			})
		}()
	}

	// selectSession highlights sessions[i] in the list and activates it.
	// It does not rely on sessionList.Select to invoke OnSelected: Fyne's
	// List skips that callback whenever i matches whatever index was
	// selected before (e.g. a new session prepended at index 0 when index 0
	// was already selected), which would otherwise leave the transcript
	// showing a stale session despite the list's highlight looking right.
	selectSession := func(i int) {
		if i < 0 || i >= len(sessions) {
			return
		}
		sessionList.Select(i)
		activateSession(sessions[i])
	}

	sessionList.OnSelected = func(id widget.ListItemID) { selectSession(id) }

	// refreshSessions re-fetches the session list (e.g. after sending a
	// message retitles or reorders it) and re-selects selectID if it's
	// still present, or the first session otherwise.
	refreshSessions := func(selectID string) {
		go func() {
			result, err := client.ListSessions(ctx)
			if err != nil {
				slog.Error("list sessions", "error", err)
				return
			}
			fyne.Do(func() {
				sessions = result
				sessionList.Refresh()
				for i, sess := range sessions {
					if sess.ID == selectID {
						selectSession(i)
						return
					}
				}
				selectSession(0)
			})
		}()
	}

	newChatButton := widget.NewButton("New Chat", func() {
		go func() {
			session, err := client.CreateSession(ctx, "")
			if err != nil {
				slog.Error("create session", "error", err)
				return
			}
			fyne.Do(func() {
				sessions = append([]app.Session{session}, sessions...)
				sessionList.Refresh()
				selectSession(0)
			})
		}()
	})

	// Populate the session list on first load, starting a session if none
	// exist yet so there's always something to chat in.
	go func() {
		result, err := client.ListSessions(ctx)
		if err != nil {
			slog.Error("list sessions", "error", err)
			return
		}
		if len(result) == 0 {
			session, err := client.CreateSession(ctx, "")
			if err != nil {
				slog.Error("create session", "error", err)
				return
			}
			result = []app.Session{session}
		}
		fyne.Do(func() {
			sessions = result
			sessionList.Refresh()
			selectSession(0)
		})
	}()

	input := widget.NewEntry()
	input.SetPlaceHolder("Ask cerebrai, or describe an automation…")

	send := func() {
		text := input.Text
		if text == "" || currentSessionID == "" {
			return
		}
		sessionID := currentSessionID
		input.SetText("")
		messages = append(messages, app.Message{Role: "user", Content: text, CreatedAt: time.Now()})
		refreshHistory()

		go func() {
			reply, err := client.SendMessage(ctx, sessionID, text)
			fyne.Do(func() {
				if err != nil {
					slog.Error("send message", "error", err)
					if currentSessionID == sessionID {
						messages = append(messages, app.Message{Role: "assistant", Content: "(error contacting background worker)"})
						refreshHistory()
					}
					return
				}
				telemetry.LogChatExchange(text, reply.Content)
				if currentSessionID == sessionID {
					messages = append(messages, reply)
					refreshHistory()
					// The reply may have retitled or reordered this session
					// in the list, so refresh it; keep it selected. Only do
					// this while sessionID is still current — otherwise the
					// user has since switched away, and refreshing would
					// yank them back to the session they just left.
					refreshSessions(sessionID)
				}
			})
		}()
	}
	input.OnSubmitted = func(string) { send() }

	sendButton := widget.NewButton("Send", send)

	inputRow := container.NewBorder(nil, nil, nil, sendButton, input)
	chatColumn := container.NewBorder(modelSelect, inputRow, nil, nil, historyScroll)
	sessionsColumn := container.NewBorder(newChatButton, nil, nil, nil, sessionList)

	split := container.NewHSplit(sessionsColumn, chatColumn)
	split.SetOffset(0.25)
	return split
}
