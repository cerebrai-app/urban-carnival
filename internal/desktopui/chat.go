package desktopui

import (
	"context"
	"fmt"
	"image/color"
	"log/slog"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
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

	// The transcript is a column of per-message bubbles rather than one
	// widget: each bubble's body is a selectable Label so its text (code
	// included) can be highlighted and copied, and an assistant reply can
	// carry a collapsed "Thought process" section above it.
	historyBox := container.NewVBox()
	historyScroll := container.NewVScroll(historyBox)

	refreshHistory := func() {
		historyBox.RemoveAll()
		for _, m := range messages {
			historyBox.Add(messageBubble(m))
		}
		historyBox.Refresh()
		historyScroll.ScrollToBottom()
	}

	sessionList := widget.NewList(
		func() int { return len(sessions) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(sessions[i].Title)
		},
	)

	// modelSelect lets the user pick which model the current session's replies
	// are generated with (DESIGN.md §5's provider abstraction). The handler
	// ignores a selection that already matches the current session's stored
	// model — that's exactly what activateSession's SetSelected produces when
	// it merely reflects the activated session — so only a real user change
	// writes back. It's shown only when there are models to choose between.
	availableModels := chat.AvailableModels()
	modelSelect := widget.NewSelect(availableModels, func(selected string) {
		if currentSessionID == "" {
			return
		}
		sessionID := currentSessionID
		for i := range sessions {
			if sessions[i].ID == sessionID {
				if sessions[i].Model == selected {
					return
				}
				sessions[i].Model = selected
				break
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
		modelSelect.SetSelected(session.Model)
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

		// A live bubble the reply streams into: the thought section starts
		// expanded and hidden, appearing once the first reasoning token
		// arrives, and the answer Label fills in token by token. It's
		// replaced by the persisted message once the turn completes.
		live := newStreamingBubble()
		historyBox.Add(live.root)
		historyBox.Refresh()

		onChunk := func(c app.ReplyChunk) {
			fyne.Do(func() {
				if currentSessionID != sessionID {
					return
				}
				live.append(c)
				historyScroll.ScrollToBottom()
			})
		}

		go func() {
			reply, err := client.StreamMessage(ctx, sessionID, text, onChunk)
			fyne.Do(func() {
				historyBox.Remove(live.root)
				if err != nil {
					slog.Error("send message", "error", err)
					if currentSessionID == sessionID {
						messages = append(messages, app.Message{Role: "assistant", Content: chatErrorMessage(err)})
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
	// Only surface the model picker when there's a choice to make; production
	// builds have no models to offer yet (chat.AvailableModels is empty).
	var chatHeader fyne.CanvasObject
	if len(availableModels) > 0 {
		chatHeader = modelSelect
	}
	chatColumn := container.NewBorder(chatHeader, inputRow, nil, nil, historyScroll)
	sessionsColumn := container.NewBorder(newChatButton, nil, nil, nil, sessionList)

	split := container.NewHSplit(sessionsColumn, chatColumn)
	split.SetOffset(0.25)
	return split
}

// bubbleMaxWidthFrac caps a chat bubble at this fraction of the transcript
// width, so a short message stays a short bubble and a long one still leaves
// room on the opposite side.
const bubbleMaxWidthFrac = 0.8

// selectableBody is a wrapping, selectable Label for a message's text, so the
// user can highlight and copy any part of it (Ctrl+C or the right-click menu).
func selectableBody(text string, align fyne.TextAlign) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	l.Alignment = align
	l.Selectable = true
	return l
}

// bubbleColor mixes the theme's background toward its foreground by frac,
// yielding a gray that tracks the light/dark system theme automatically. User
// and assistant bubbles pass different fracs so they read as distinct hues.
func bubbleColor(frac float32) color.Color {
	bg := color.NRGBAModel.Convert(theme.Color(theme.ColorNameBackground)).(color.NRGBA)
	fg := color.NRGBAModel.Convert(theme.Color(theme.ColorNameForeground)).(color.NRGBA)
	mix := func(a, b uint8) uint8 { return uint8(float32(a)*(1-frac) + float32(b)*frac) }
	return color.NRGBA{R: mix(bg.R, fg.R), G: mix(bg.G, fg.G), B: mix(bg.B, fg.B), A: 255}
}

// bubble wraps a message's content in a rounded, tinted card and aligns it to
// one side of the transcript (user right, assistant left). naturalWidth is the
// unwrapped width of the content's text; the bubble hugs that up to
// bubbleMaxWidthFrac of the transcript, past which the text wraps. Pass 0 to
// always use the full fraction (e.g. content that isn't a single label).
func bubble(content fyne.CanvasObject, role string, naturalWidth float32) fyne.CanvasObject {
	right := role == "user"
	frac := float32(0.07)
	if right {
		frac = 0.16
	}
	rect := canvas.NewRectangle(bubbleColor(frac))
	rect.CornerRadius = theme.Padding() * 2
	card := container.NewStack(rect, container.NewPadded(content))
	return container.New(&sideAlign{right: right, naturalWidth: naturalWidth}, card)
}

// naturalTextWidth is the width of the widest line of s laid out unwrapped,
// plus the padding a bubble adds around its label. It's what lets a short
// message stay a short bubble.
func naturalTextWidth(s string) float32 {
	var widest float32
	for _, line := range strings.Split(s, "\n") {
		widest = fyne.Max(widest, fyne.MeasureText(line, theme.TextSize(), fyne.TextStyle{}).Width)
	}
	return widest + 6*theme.Padding()
}

// messageBubble renders one persisted message: an assistant reply's collapsed
// "Thought process" section when it has one, then the body, tinted and aligned
// by role.
func messageBubble(m app.Message) fyne.CanvasObject {
	align := fyne.TextAlignLeading
	if m.Role == "user" {
		align = fyne.TextAlignTrailing
	}
	if m.Thoughts != "" {
		item := widget.NewAccordionItem("Thought process", selectableBody(m.Thoughts, fyne.TextAlignLeading))
		content := container.NewVBox(widget.NewAccordion(item), selectableBody(m.Content, align)) // accordion collapsed by default
		return bubble(content, m.Role, 0)
	}
	return bubble(selectableBody(m.Content, align), m.Role, naturalTextWidth(m.Content))
}

// streamingBubble is the in-progress assistant reply: a thought section that
// stays hidden until the first reasoning token, plus an answer Label that
// fills in as tokens arrive. append accumulates each chunk.
type streamingBubble struct {
	root       fyne.CanvasObject
	thoughtAcc *widget.Accordion
	thought    *widget.Label
	answer     *widget.Label
	thoughtBuf strings.Builder
	answerBuf  strings.Builder
}

func newStreamingBubble() *streamingBubble {
	b := &streamingBubble{
		thought: selectableBody("", fyne.TextAlignLeading),
		answer:  selectableBody("", fyne.TextAlignLeading),
	}
	item := widget.NewAccordionItem("Thinking…", b.thought)
	item.Open = true
	b.thoughtAcc = widget.NewAccordion(item)
	b.thoughtAcc.Hide()
	b.root = bubble(container.NewVBox(b.thoughtAcc, b.answer), "assistant", 0)
	return b
}

func (b *streamingBubble) append(c app.ReplyChunk) {
	if c.Thought != "" {
		b.thoughtBuf.WriteString(c.Thought)
		b.thought.SetText(b.thoughtBuf.String())
		b.thoughtAcc.Show()
	}
	if c.Answer != "" {
		b.answerBuf.WriteString(c.Answer)
		b.answer.SetText(b.answerBuf.String())
	}
}

// sideAlign lays out a single child flush to one side of the row (left, or
// right when right is set). Its width is bubbleMaxWidthFrac of the row, or
// naturalWidth when that is smaller and positive — so a short message is a
// short bubble and a long one wraps. It's how chat bubbles hug their side of
// the transcript.
type sideAlign struct {
	right        bool
	naturalWidth float32
}

func (s *sideAlign) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	// Report only height: the row's width comes from the parent, and forcing
	// the child's full unwrapped width here would stop its text wrapping.
	return fyne.NewSize(0, objs[0].MinSize().Height)
}

func (s *sideAlign) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) == 0 {
		return
	}
	o := objs[0]
	w := size.Width * bubbleMaxWidthFrac
	if s.naturalWidth > 0 && s.naturalWidth < w {
		w = s.naturalWidth
	}
	o.Resize(fyne.NewSize(w, o.MinSize().Height))
	x := float32(0)
	if s.right {
		x = size.Width - w
	}
	o.Move(fyne.NewPos(x, 0))
}

// chatErrorMessage turns a failed StreamMessage into the line shown in the
// transcript where the reply would have been. It quotes the underlying error
// rather than a fixed "something went wrong": the client is in-process today
// (internal/storage), so the cause is almost always local and actionable —
// no model provider configured, a stale database, the Claude Code CLI
// missing — and hiding it just sends the user to the logs.
func chatErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("⚠️ Couldn't generate a reply: %v", err)
}
