package desktopui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
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
