package desktopui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/cerebrai-app/urban-carnival/internal/app"
)

// TestScrollTranscriptToBottom covers the auto-scroll fix: after content is
// added to the transcript, the scroll container has not yet resized its content
// to fit, so a bare ScrollToBottom would be a no-op. scrollTranscriptToBottom
// resizes the content first and lands on the real bottom.
func TestScrollTranscriptToBottom(t *testing.T) {
	test.NewApp()

	box := container.NewVBox()
	scroll := container.NewVScroll(box)

	w := test.NewWindow(scroll)
	defer w.Close()
	w.Resize(fyne.NewSize(300, 200))

	// A transcript far taller than the 200px viewport.
	for i := 0; i < 40; i++ {
		l := widget.NewLabel(strings.Repeat("line ", 8))
		l.Wrapping = fyne.TextWrapWord
		box.Add(l)
	}
	box.Refresh()

	scrollTranscriptToBottom(scroll)

	want := box.MinSize().Height - scroll.Size().Height
	if want <= 0 {
		t.Fatalf("test setup: content (%v) not taller than viewport (%v)", box.MinSize().Height, scroll.Size().Height)
	}
	if got := scroll.Offset.Y; got < want-1 {
		t.Fatalf("Offset.Y = %v, want >= %v (scrolled to bottom)", got, want)
	}
}

// TestStreamingBubbleReset covers the stale-resume recovery on the UI side: a
// Reset chunk (emitted when the provider abandons a partial attempt and
// retries) must wipe everything streamed so far so the retry's tokens don't
// append onto a half-written reply.
func TestStreamingBubbleReset(t *testing.T) {
	test.NewApp()

	b := newStreamingBubble()
	b.append(app.ReplyChunk{Thought: "half a thought"})
	b.append(app.ReplyChunk{Answer: "half an answer"})
	if b.thought.Text != "half a thought" || b.answer.Text != "half an answer" {
		t.Fatalf("pre-reset: thought=%q answer=%q", b.thought.Text, b.answer.Text)
	}
	if b.thoughtAcc.Hidden {
		t.Fatal("pre-reset: thought section should be visible once a thought arrives")
	}

	b.append(app.ReplyChunk{Reset: true})
	if b.thought.Text != "" || b.answer.Text != "" {
		t.Errorf("after reset: thought=%q answer=%q, want both empty", b.thought.Text, b.answer.Text)
	}
	if !b.thoughtAcc.Hidden {
		t.Error("after reset: thought section should be hidden again")
	}

	b.append(app.ReplyChunk{Answer: "clean "})
	b.append(app.ReplyChunk{Answer: "answer"})
	if b.answer.Text != "clean answer" {
		t.Errorf("post-reset answer = %q, want %q", b.answer.Text, "clean answer")
	}
}
