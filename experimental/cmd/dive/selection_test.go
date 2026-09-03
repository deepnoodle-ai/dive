package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/termtest"
	"github.com/deepnoodle-ai/wonton/tui"
)

// recordingClipboard stands in for the ladder. Copying happens on a goroutine,
// so the channel is how a test waits for it without a sleep.
type recordingClipboard struct {
	copied chan string
	err    error
}

func newRecordingClipboard() *recordingClipboard {
	return &recordingClipboard{copied: make(chan string, 4)}
}

func (c *recordingClipboard) copier() clipboardCopier {
	return func(text string) (clipboardReport, error) {
		c.copied <- text
		if c.err != nil {
			return clipboardReport{}, c.err
		}
		return clipboardReport{lines: countLines(text), via: "test", verified: true}, nil
	}
}

// next returns the next text copied, failing the test if nothing arrives.
func (c *recordingClipboard) next(t *testing.T) string {
	t.Helper()
	select {
	case text := <-c.copied:
		return text
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was copied")
		return ""
	}
}

// nothing asserts that no copy happened. The window is short on purpose: this
// runs on every copy-on-select-off test and a real copy is dispatched at once.
func (c *recordingClipboard) nothing(t *testing.T) {
	t.Helper()
	select {
	case text := <-c.copied:
		t.Fatalf("copied %q with nothing to copy", text)
	case <-time.After(50 * time.Millisecond):
	}
}

// lockedRunner records events sent from any goroutine. The copy path needs it:
// the ladder runs off the event loop and reports back through SendEvent, which
// the plain fakeRunner appends to a slice without a lock.
type lockedRunner struct {
	mu     sync.Mutex
	events []tui.Event
}

func (r *lockedRunner) SendEvent(e tui.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *lockedRunner) Stop() {}

// waitForNotice waits for a notice containing want, and returns it.
func (r *lockedRunner) waitForNotice(t *testing.T, want string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, e := range r.events {
			if n, ok := e.(noticeEvent); ok && strings.Contains(n.text, want) {
				r.mu.Unlock()
				return n.text
			}
		}
		r.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no notice containing %q arrived", want)
	return ""
}

// newSelectingApp is a managed-screen app with mouse gestures on and a
// clipboard that records instead of copying.
func newSelectingApp(t *testing.T, n int) (*App, *recordingClipboard) {
	t.Helper()
	app := newScreenApp(t, n)
	// The managed screen owns its transcript; nothing goes to scrollback.
	app.runner, app.scrollback = &lockedRunner{}, nil
	app.mouseEnabled = true
	app.copyOnSelect = true
	clip := newRecordingClipboard()
	app.clipboard = clip.copier()
	return app, clip
}

// findText returns the column and row of the first occurrence of want, and how
// many columns it covers. Tests drag across real rendered text rather than
// guessing at rows, since a message's height depends on how its markdown
// wrapped.
//
// It walks cells rather than indexing the screen's text, because a byte offset
// into that text is not a column: the "⏺" in front of every reply is three
// bytes and one cell.
func findText(t *testing.T, screen *termtest.Screen, want string) (x, y, width int) {
	t.Helper()
	w, h := screen.Size()
	for row := range h {
		for col := range w {
			var seen strings.Builder
			for c := col; c < w && seen.Len() < len(want); c++ {
				seen.WriteString(screen.CellGlyph(c, row))
				if seen.String() == want {
					return col, row, c - col + 1
				}
			}
		}
	}
	t.Fatalf("%q is not on screen:\n%s", want, screen.Text())
	return 0, 0, 0
}

// press, drag and release script a mouse gesture in screen coordinates, in the
// order the runtime delivers them.
func press(app *App, x, y int) {
	app.HandleEvent(tui.MouseEvent{Type: tui.MousePress, Button: tui.MouseButtonLeft, X: x, Y: y})
}

func drag(app *App, x, y int) {
	app.HandleEvent(tui.MouseEvent{Type: tui.MouseDrag, Button: tui.MouseButtonLeft, X: x, Y: y})
}

func release(app *App, x, y int) {
	app.HandleEvent(tui.MouseEvent{Type: tui.MouseRelease, Button: tui.MouseButtonLeft, X: x, Y: y})
}

// click is a press and release at one spot, with the synthetic click the
// runtime inserts between them.
func click(app *App, x, y, count int) {
	press(app, x, y)
	app.HandleEvent(tui.MouseEvent{
		Type: tui.MouseClick, Button: tui.MouseButtonLeft, X: x, Y: y, ClickCount: count,
	})
	release(app, x, y)
}

// reversedText is the text of every reverse-video cell on screen, which is
// where the selection highlight lands.
func reversedText(screen *termtest.Screen, width, height int) string {
	var out strings.Builder
	for y := range height {
		for x := range width {
			if cell := screen.Cell(x, y); cell.Style.Reverse {
				out.WriteRune(cell.Char)
			}
		}
	}
	return out.String()
}

func TestDraggingAcrossAMessageSelectsAndCopiesIt(t *testing.T) {
	app, clip := newSelectingApp(t, 3)

	screen := renderScreen(t, app, 80, 24)
	x, y, width := findText(t, screen, "answer 2")

	press(app, x, y)
	drag(app, x+width, y)
	release(app, x+width, y)

	assert.Equal(t, clip.next(t), "answer 2")
	assert.True(t, app.viewport.HasSelection(), "the highlight stays up after the copy")

	screen = renderScreen(t, app, 80, 24)
	assert.Equal(t, reversedText(screen, 80, 24), "answer 2",
		"exactly the dragged cells are highlighted")
}

func TestDoubleClickTakesTheWordAndTripleClickTheLine(t *testing.T) {
	app, clip := newSelectingApp(t, 2)

	screen := renderScreen(t, app, 80, 24)
	x, y, _ := findText(t, screen, "answer 1")

	click(app, x, y, 2)
	assert.Equal(t, clip.next(t), "answer", "a double click takes one word")

	click(app, x, y, 3)
	assert.Equal(t, clip.next(t), "⏺ answer 1", "a triple click takes the whole line")
}

func TestAClickWithNothingSelectedDoesNotCopy(t *testing.T) {
	app, clip := newSelectingApp(t, 2)

	screen := renderScreen(t, app, 80, 24)
	x, y, _ := findText(t, screen, "answer 1")

	click(app, x, y, 1)
	clip.nothing(t)
	assert.False(t, app.viewport.HasSelection())
}

func TestAClickDismissesTheSelectionAndRestoresFollowing(t *testing.T) {
	app, clip := newSelectingApp(t, 3)

	screen := renderScreen(t, app, 80, 24)
	x, y, width := findText(t, screen, "answer 2")

	press(app, x, y)
	drag(app, x+width, y)
	release(app, x+width, y)
	clip.next(t)
	assert.False(t, app.viewport.Follow, "a selection pins the viewport while it stands")

	click(app, x, y, 1)
	assert.False(t, app.viewport.HasSelection())
	assert.True(t, app.viewport.Follow, "dismissing it hands following back")
}

func TestCopyOnSelectOffLeavesTheHighlightForCtrlC(t *testing.T) {
	app, clip := newSelectingApp(t, 3)
	app.copyOnSelect = false

	screen := renderScreen(t, app, 80, 24)
	x, y, width := findText(t, screen, "answer 2")

	press(app, x, y)
	drag(app, x+width, y)
	release(app, x+width, y)
	clip.nothing(t)
	assert.True(t, app.viewport.HasSelection(), "the drag still highlights")

	app.HandleEvent(tui.KeyEvent{Key: tui.KeyCtrlC})
	assert.Equal(t, clip.next(t), "answer 2")
	assert.False(t, app.viewport.HasSelection(), "copying clears it")
	assert.False(t, app.showExitHint, "Ctrl+C copied rather than starting to quit")
}

func TestCtrlCStillQuitsWhenNothingIsSelected(t *testing.T) {
	app, _ := newSelectingApp(t, 2)
	app.copyOnSelect = false

	app.HandleEvent(tui.KeyEvent{Key: tui.KeyCtrlC})
	assert.True(t, app.showExitHint, "with no selection Ctrl+C means what it always did")
}

func TestAPressInTheFooterDoesNotStartASelection(t *testing.T) {
	app, _ := newSelectingApp(t, 3)
	renderScreen(t, app, 80, 24)

	// The row after the viewport is the footer's first, where the input lives.
	press(app, 4, app.viewport.Height)
	assert.False(t, app.viewport.SelectionActive(), "the footer is not the transcript")
}

func TestMouseOffHandsGesturesBackToTheTerminal(t *testing.T) {
	app, clip := newSelectingApp(t, 3)

	screen := renderScreen(t, app, 80, 24)
	x, y, width := findText(t, screen, "answer 2")

	app.setMouseReporting(false)
	press(app, x, y)
	drag(app, x+width, y)
	release(app, x+width, y)

	clip.nothing(t)
	assert.False(t, app.viewport.HasSelection())

	// Scrolling is not gated on mouseEnabled, so a wheel event that does reach
	// us is still honoured. In practice the terminal stops sending them —
	// reporting off means no mouse bytes at all — which is why turning it off
	// says where scrolling went.
	before, _ := app.viewport.Anchor()
	app.HandleEvent(tui.MouseEvent{Type: tui.MouseScroll, Button: tui.MouseButtonWheelUp})
	after, _ := app.viewport.Anchor()
	assert.True(t, after <= before, "a wheel event that arrives still scrolls")
}

func TestClickingAToolCallHeaderExpandsItsOutput(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true
	app.mouseEnabled = true
	app.appendMessage(Message{
		Type:            MessageTypeToolCall,
		ToolName:        "Bash",
		ToolTitle:       "Bash",
		ToolID:          "bash-1",
		ToolDone:        true,
		ToolResult:      "first line",
		ToolResultLines: []string{"first line", "second line", "third line"},
	})
	app.viewport.ScrollToBottom()

	screen := renderScreen(t, app, 80, 24)
	assert.NotContains(t, screen.Text(), "third line", "a tool result starts collapsed")
	_, y, _ := findText(t, screen, "Bash")

	click(app, 4, y, 1)
	assert.True(t, app.messages[0].Expanded)
	assert.Contains(t, renderScreen(t, app, 80, 24).Text(), "third line")

	click(app, 4, y, 1)
	assert.False(t, app.messages[0].Expanded, "clicking again puts it back")
}

func TestClickingAReportTitleCollapsesIt(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true
	app.mouseEnabled = true
	app.appendReport(tui.Stack(
		tui.Text("Token usage"),
		tui.Text("  input   1,024"),
		tui.Text("  output    512"),
	))
	app.viewport.ScrollToBottom()

	screen := renderScreen(t, app, 80, 24)
	_, y, _ := findText(t, screen, "Token usage")

	click(app, 4, y, 1)
	assert.True(t, app.messages[0].Collapsed)
	text := renderScreen(t, app, 80, 24).Text()
	assert.Contains(t, text, "Token usage", "the title stays as the handle to reopen it")
	assert.NotContains(t, text, "1,024", "the body is gone")

	click(app, 4, y, 1)
	assert.Contains(t, renderScreen(t, app, 80, 24).Text(), "1,024")
}

func TestClickingTheScrollIndicatorJumpsToTheBottom(t *testing.T) {
	app, _ := newSelectingApp(t, 40)

	renderScreen(t, app, 80, 24)
	app.viewport.ScrollBy(-20)
	renderScreen(t, app, 80, 24)
	assert.False(t, app.viewport.AtBottom, "scrolled up, so the indicator is showing")

	click(app, 4, app.viewport.Height, 1)
	assert.True(t, app.viewport.AtBottom, "the indicator is also the way back")
}

func TestADragHeldPastTheBottomKeepsSelecting(t *testing.T) {
	app, _ := newSelectingApp(t, 40)

	renderScreen(t, app, 80, 24)
	app.viewport.ScrollBy(-10)
	screen := renderScreen(t, app, 80, 24)

	// Whatever is at the top of the viewport now: the drag runs downward from
	// there, off the bottom edge.
	x, y, _ := findText(t, screen, "question 3")
	press(app, x, y)
	// Held below the viewport: no more mouse events arrive, only frames.
	drag(app, x, app.viewport.Height+2)

	start, _, _ := app.viewport.Selection()
	for range 5 {
		app.HandleEvent(tui.TickEvent{})
		renderScreen(t, app, 80, 24)
	}
	_, end, ok := app.viewport.Selection()
	assert.True(t, ok, "the selection survived the auto-scroll")
	assert.True(t, end.Item > start.Item, "and grew past where the drag stopped moving")
}

func TestCopyFailureIsReportedRatherThanSwallowed(t *testing.T) {
	app, clip := newSelectingApp(t, 3)
	clip.err = errors.New("pbcopy: no such file")
	runner := app.runner.(*lockedRunner)

	screen := renderScreen(t, app, 80, 24)
	x, y, width := findText(t, screen, "answer 2")
	press(app, x, y)
	drag(app, x+width, y)
	release(app, x+width, y)
	clip.next(t)

	assert.Contains(t, runner.waitForNotice(t, "Copy failed"), "pbcopy: no such file")
}

func TestASuccessfulCopySaysHowManyLinesAndByWhat(t *testing.T) {
	app, clip := newSelectingApp(t, 3)
	runner := app.runner.(*lockedRunner)

	screen := renderScreen(t, app, 80, 24)
	x, y, width := findText(t, screen, "answer 2")
	press(app, x, y)
	drag(app, x+width, y)
	release(app, x+width, y)
	clip.next(t)

	assert.Equal(t, runner.waitForNotice(t, "Copied"), "Copied 1 line (test)")
}

func TestEscapeDismissesASelectionBeforeAnythingElse(t *testing.T) {
	app, clip := newSelectingApp(t, 3)
	app.copyOnSelect = false
	app.processing = true
	turn, cancel := context.WithCancel(t.Context())
	app.cancel = cancel

	screen := renderScreen(t, app, 80, 24)
	x, y, width := findText(t, screen, "answer 2")
	press(app, x, y)
	drag(app, x+width, y)
	release(app, x+width, y)
	clip.nothing(t)

	app.HandleEvent(tui.KeyEvent{Key: tui.KeyEscape})
	assert.False(t, app.viewport.HasSelection(), "Escape drops the highlight")
	assert.NoError(t, turn.Err(), "and does not also cancel the turn")

	app.HandleEvent(tui.KeyEvent{Key: tui.KeyEscape})
	assert.Error(t, turn.Err(), "a second Escape cancels, as it always did")
}

func TestAlternateScrollIsOnlyRestoredIfWeTurnedItOff(t *testing.T) {
	app, _ := newSelectingApp(t, 2)

	// No terminal, so nothing is written either way; the flag is the record of
	// what we would owe the user's terminal on the way out.
	app.setMouseReporting(false)
	assert.False(t, app.altScrollDisabled, "no terminal, nothing changed, nothing owed")

	app.altScrollDisabled = true
	app.restoreAlternateScroll()
	assert.True(t, app.altScrollDisabled, "still owed while there is no terminal to pay it to")
}
