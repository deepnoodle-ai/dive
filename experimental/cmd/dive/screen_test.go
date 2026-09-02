package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/termtest"
	"github.com/deepnoodle-ai/wonton/tui"
)

// renderScreen renders the managed screen at a fixed size, the way the runtime
// does: measure at (width, height), then draw. Height is mandatory here — the
// viewport is flexible, so without one it would have no size to work from.
func renderScreen(t *testing.T, app *App, width, height int) *termtest.Screen {
	t.Helper()
	return tui.SprintScreen(app.View(), tui.WithWidth(width), tui.WithHeight(height))
}

// newScreenApp returns an app in managed-screen mode holding n numbered
// exchanges, scrolled to the bottom.
func newScreenApp(t *testing.T, n int) *App {
	t.Helper()
	app, _ := newFakeApp(t)
	app.screenMode = true
	for i := range n {
		app.appendMessage(Message{Role: roleUser, Content: fmt.Sprintf("question %d", i)})
		app.appendMessage(Message{Role: roleAssistant, Content: fmt.Sprintf("answer %d", i)})
	}
	app.viewport.ScrollToBottom()
	return app
}

func TestScreenShowsTheLatestTranscriptAboveTheInput(t *testing.T) {
	app := newScreenApp(t, 20)

	screen := renderScreen(t, app, 80, 24)
	text := screen.Text()

	assert.Contains(t, text, "answer 19", "the newest message should be on screen")
	assert.NotContains(t, text, "answer 0", "an old message should have scrolled off")
	assert.Contains(t, text, "❯", "the input prompt is pinned to the bottom")

	// The input area owns the bottom of the screen, not the transcript.
	assert.True(t, rowOf(t, screen, 24, "Type a message") > rowOf(t, screen, 24, "answer 19"),
		"the input must sit below the transcript")
}

// TestScreenFrameLayout is the structural golden: where every part of the
// managed screen lands in a frame. A transcript shorter than the viewport grows
// down from the top rather than clinging to the bottom, and the footer — the
// dividers, the prompt, the status line — owns the last rows whatever is above
// it.
func TestScreenFrameLayout(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true
	app.gitBranch = "main"
	app.appendMessage(Message{Role: roleUser, Content: "what changed?"})
	app.appendMessage(Message{Role: roleAssistant, Content: "Two files, both **small**."})
	app.appendMessage(Message{
		Role:       roleAssistant,
		Type:       MessageTypeToolCall,
		ToolName:   "read_file",
		ToolTitle:  "Read",
		ToolInput:  `{"path":"main.go"}`,
		ToolResult: "42 lines",
		ToolDone:   true,
	})
	app.viewport.ScrollToBottom()

	screen := renderScreen(t, app, 60, 14)
	want := []string{
		" ❯ what changed?",
		"",
		" ⏺ Two files, both small.", // markdown, rendered
		"",
		` ⏺ Read(path: "main.go")`,
		"",
		"",
		"",
		strings.Repeat("─", 60),
		" ❯  Type a message... (@filename, or drop a file to attach)",
		strings.Repeat("─", 60),
		" test-model in test on main",
	}
	for y, row := range want {
		assert.Equal(t, row, strings.TrimRight(screen.Row(y), " "), "row %d", y)
	}
}

func TestScreenIndicatesWhatIsBelowWhenScrolledUp(t *testing.T) {
	app := newScreenApp(t, 40)

	renderScreen(t, app, 80, 24) // gives the viewport its size
	app.viewport.PageUp()
	screen := renderScreen(t, app, 80, 24)

	assert.False(t, app.viewport.AtBottom)
	assert.True(t, app.viewport.LinesBelow > 0)
	assert.Contains(t, screen.Text(), "new line")
	assert.Contains(t, screen.Text(), "End to jump")

	// And it goes away once the user is back at the end.
	app.viewport.ScrollToBottom()
	assert.NotContains(t, renderScreen(t, app, 80, 24).Text(), "End to jump")
}

func TestScreenScrollKeys(t *testing.T) {
	tests := []struct {
		name     string
		draft    string
		key      tui.KeyEvent
		consumed bool
		wantTop  bool // else: expect the bottom
	}{
		{name: "PageUp", key: tui.KeyEvent{Key: tui.KeyPageUp}, consumed: true},
		{name: "Home on an empty input", key: tui.KeyEvent{Key: tui.KeyHome}, consumed: true, wantTop: true},
		{name: "Ctrl+Home despite a draft", draft: "half a thought", key: tui.KeyEvent{Key: tui.KeyHome, Ctrl: true}, consumed: true, wantTop: true},
		{name: "Home with a draft belongs to the input", draft: "half a thought", key: tui.KeyEvent{Key: tui.KeyHome}},
		{name: "End with a draft belongs to the input", draft: "half a thought", key: tui.KeyEvent{Key: tui.KeyEnd}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newScreenApp(t, 40)
			renderScreen(t, app, 80, 24)
			app.inputText = tc.draft

			assert.Equal(t, tc.consumed, app.handleScreenKey(tc.key))
			if !tc.consumed {
				return
			}
			renderScreen(t, app, 80, 24)
			if tc.wantTop {
				item, line := app.viewport.Anchor()
				assert.Equal(t, 0, item)
				assert.Equal(t, 0, line)
			} else {
				assert.False(t, app.viewport.AtBottom, "%s should have moved off the end", tc.name)
			}
		})
	}
}

func TestScreenKeysDoNothingInInlineMode(t *testing.T) {
	app := newScreenApp(t, 40)
	app.screenMode = false
	assert.False(t, app.handleScreenKey(tui.KeyEvent{Key: tui.KeyPageUp}))
}

func TestScrollKeysReachTheAppThroughTheFocusedInput(t *testing.T) {
	// The input field owns its keystrokes, so the scroll keys have to arrive
	// through its OnKey hook. handleInputNavKey is that hook.
	app := newScreenApp(t, 40)
	renderScreen(t, app, 80, 24)

	assert.True(t, app.handleInputNavKey(tui.KeyEvent{Key: tui.KeyPageUp}))
	renderScreen(t, app, 80, 24)
	assert.False(t, app.viewport.AtBottom)

	assert.True(t, app.handleInputNavKey(tui.KeyEvent{Key: tui.KeyEnd}))
	renderScreen(t, app, 80, 24)
	assert.True(t, app.viewport.AtBottom)
}

func TestAutocompleteStillWinsOverTheScrollKeys(t *testing.T) {
	app := newScreenApp(t, 40)
	renderScreen(t, app, 80, 24)
	app.autocompleteMatches = []string{"/clear", "/compact"}

	// Escape belongs to the open completion list, not the transcript.
	app.viewport.PageUp()
	renderScreen(t, app, 80, 24)
	assert.True(t, app.handleInputNavKey(tui.KeyEvent{Key: tui.KeyEscape}))
	assert.Equal(t, 0, len(app.autocompleteMatches))
	assert.False(t, app.viewport.AtBottom, "Escape dismissing autocomplete should not also scroll")
}

func TestEscapeReturnsToTheBottomWhenIdle(t *testing.T) {
	app := newScreenApp(t, 40)
	renderScreen(t, app, 80, 24)
	app.viewport.PageUp()
	renderScreen(t, app, 80, 24)
	assert.False(t, app.viewport.AtBottom)

	app.handleKeyEvent(tui.KeyEvent{Key: tui.KeyEscape})
	renderScreen(t, app, 80, 24)
	assert.True(t, app.viewport.AtBottom)
}

func TestWheelScrollsThreeLines(t *testing.T) {
	app := newScreenApp(t, 40)
	renderScreen(t, app, 80, 24)
	assert.True(t, app.viewport.AtBottom)

	// Read the position straight after each event rather than re-rendering
	// between them: the "N new lines" row appears as soon as the transcript is
	// scrolled and costs the viewport a line, which would move the end.
	app.HandleEvent(tui.MouseEvent{Type: tui.MouseScroll, Button: tui.MouseButtonWheelUp})
	assert.Equal(t, 3, app.viewport.LinesBelow, "one notch is three lines")
	app.HandleEvent(tui.MouseEvent{Type: tui.MouseScroll, Button: tui.MouseButtonWheelUp})
	assert.Equal(t, 6, app.viewport.LinesBelow)

	app.HandleEvent(tui.MouseEvent{Type: tui.MouseScroll, Button: tui.MouseButtonWheelDown})
	assert.Equal(t, 3, app.viewport.LinesBelow)
	app.HandleEvent(tui.MouseEvent{Type: tui.MouseScroll, Button: tui.MouseButtonWheelDown})
	assert.True(t, app.viewport.AtBottom, "back where it started")
}

func TestWheelIsIgnoredInInlineMode(t *testing.T) {
	app := newScreenApp(t, 40)
	renderScreen(t, app, 80, 24)
	app.screenMode = false

	app.HandleEvent(tui.MouseEvent{Type: tui.MouseScroll, Button: tui.MouseButtonWheelUp})
	assert.True(t, app.viewport.AtBottom)
}

func TestTypingAndSubmittingReturnToTheBottom(t *testing.T) {
	app := newScreenApp(t, 40)
	renderScreen(t, app, 80, 24)
	app.viewport.PageUp()
	renderScreen(t, app, 80, 24)
	assert.False(t, app.viewport.AtBottom)

	app.snapToBottom()
	renderScreen(t, app, 80, 24)
	assert.True(t, app.viewport.AtBottom, "typing should return to the live end")
}

func TestResizeWhileScrolledUpKeepsTheSameMessageInView(t *testing.T) {
	app := newScreenApp(t, 60)
	renderScreen(t, app, 80, 24)
	app.viewport.PageUp()
	app.viewport.PageUp()
	renderScreen(t, app, 80, 24)

	item, _ := app.viewport.Anchor()
	assert.True(t, item > 0, "should be anchored partway up the transcript")
	// Pin that message to the first row so "still in view" is unambiguous.
	app.viewport.ScrollToItem(item)
	renderScreen(t, app, 80, 24)
	anchored := app.messages[item].Content

	app.HandleEvent(tui.ResizeEvent{Width: 100, Height: 40})
	wide := renderScreen(t, app, 100, 40)

	sameItem, _ := app.viewport.Anchor()
	assert.Equal(t, item, sameItem, "a resize must not move the anchor")
	assert.Contains(t, wide.Text(), anchored)
	assert.Equal(t, 100, app.termWidth)
	assert.Equal(t, 40, app.termHeight)

	// And narrower, where every message rewraps.
	narrow := renderScreen(t, app, 50, 20)
	sameItem, _ = app.viewport.Anchor()
	assert.Equal(t, item, sameItem)
	assert.Contains(t, narrow.Text(), anchored)
}

func TestResizeWhileStreamingKeepsFollowingTheReply(t *testing.T) {
	app := newScreenApp(t, 10)
	renderScreen(t, app, 80, 24)

	app.handleProcessingStart(processingStartEvent{baseEvent: newBaseEvent(), userInput: "explain"})
	for i := range 30 {
		app.handleStreamText(fmt.Sprintf("streamed line %d\n", i))
	}
	app.flushStreamBuffer()
	assert.Contains(t, renderScreen(t, app, 80, 24).Text(), "streamed line 29")

	app.HandleEvent(tui.ResizeEvent{Width: 60, Height: 30})
	assert.Contains(t, renderScreen(t, app, 60, 30).Text(), "streamed line 29",
		"a resize while streaming should stay pinned to the end")
}

func TestManagedScreenRefusesANonTerminal(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true

	// Capture anything written to stdout, so the test can prove no alternate
	// screen escape ever reaches a pipe.
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	assert.NoError(t, err)
	realStdout := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = realStdout }()

	restore := isTerminal
	defer func() { isTerminal = restore }()
	isTerminal = func(f *os.File) bool { return false }

	err = app.Run()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--print")
	assert.Contains(t, err.Error(), "terminal")

	assert.NoError(t, out.Close())
	written, err := os.ReadFile(out.Name())
	assert.NoError(t, err)
	assert.Equal(t, "", string(written), "nothing may be written to a non-terminal stdout")
}

func TestManagedScreenChecksBothEnds(t *testing.T) {
	tests := []struct {
		name          string
		stdin, stdout bool
	}{
		{"input redirected", false, true},
		{"output piped", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newFakeApp(t)
			app.screenMode = true
			restore := isTerminal
			defer func() { isTerminal = restore }()
			isTerminal = func(f *os.File) bool {
				if f == os.Stdin {
					return tc.stdin
				}
				return tc.stdout
			}
			assert.Error(t, app.Run())
		})
	}
}

// TestFrameCostAtTwoHundredMessages is the Phase 4 frame budget: building and
// drawing a full screen of a long transcript has to stay cheap enough for a
// 30 fps runtime. The numbers are ceilings with room to spare, not targets —
// they exist to catch a return to the O(transcript) rendering this replaced.
func TestFrameCostAtTwoHundredMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("timing-sensitive")
	}
	app := newScreenApp(t, 100) // 100 exchanges = 200 messages
	renderScreen(t, app, 80, 40)

	allocs := testing.AllocsPerRun(20, func() {
		tui.SprintScreen(app.View(), tui.WithWidth(80), tui.WithHeight(40))
	})
	// A viewport frame costs what a screenful costs. The inline renderer built
	// every message every frame and ran to ~35k allocations at this size.
	assert.True(t, allocs < 5000, "a frame allocated %.0f objects", allocs)
}

// BenchmarkScreenFrame200 is BenchmarkView200's managed-screen counterpart:
// the same 200-message transcript, the same width, one whole frame. The inline
// renderer built every message every frame; this one builds a screenful.
func BenchmarkScreenFrame200(b *testing.B) {
	app := NewApp(&dive.Agent{}, nil, "/tmp/test", "test-model", "", nil, "", nil, "")
	app.runner = &fakeRunner{}
	app.gitBranch = "main"
	app.screenMode = true
	for range 100 {
		app.appendMessage(Message{Role: roleUser, Content: "please look at the thing"})
		app.appendMessage(Message{
			Role:    roleAssistant,
			Content: "Here is a **paragraph** of prose with `code` in it.\n\n- one\n- two\n",
		})
	}
	app.viewport.ScrollToBottom()
	tui.SprintScreen(app.View(), tui.WithWidth(120), tui.WithHeight(40))

	b.ReportAllocs()
	for b.Loop() {
		tui.SprintScreen(app.View(), tui.WithWidth(120), tui.WithHeight(40))
	}
}

// rowOf returns the index of the first row containing want.
func rowOf(t *testing.T, screen *termtest.Screen, rows int, want string) int {
	t.Helper()
	for y := range rows {
		if strings.Contains(screen.Row(y), want) {
			return y
		}
	}
	t.Fatalf("no row contains %q", want)
	return -1
}

func TestFrameMetricsLineOnlyAppearsWhenAsked(t *testing.T) {
	app := newScreenApp(t, 4)
	assert.Nil(t, app.frameMetricsView(), "off by default")

	// It needs a terminal for the flush numbers, which a rendered-to-string
	// test does not have; what it must never do is appear uninvited.
	app.frameMetrics = true
	app.recordViewTime(2 * time.Millisecond)
	assert.Nil(t, app.frameMetricsView(), "no terminal, no metrics")
	assert.NotContains(t, renderScreen(t, app, 80, 24).Text(), "fps")
}

func TestViewTimeTracksAverageAndWorstCase(t *testing.T) {
	app := newScreenApp(t, 4)
	app.recordViewTime(2 * time.Millisecond)
	app.recordViewTime(8 * time.Millisecond)
	app.recordViewTime(2 * time.Millisecond)

	assert.Equal(t, int64(3), app.viewTimeCount)
	assert.Equal(t, 12*time.Millisecond, app.viewTimeTotal)
	assert.Equal(t, 8*time.Millisecond, app.viewTimeMax)
}

func TestFooterDoesNotRepeatWhatTheTranscriptAlreadyShows(t *testing.T) {
	// Inline mode's live region redraws the in-flight tool calls because they
	// are not in scrollback yet. In the managed screen they are already in the
	// transcript, so the footer shows only the progress line.
	app := newScreenApp(t, 2)
	app.handleProcessingStart(processingStartEvent{baseEvent: newBaseEvent(), userInput: "run the tests"})
	app.appendMessage(Message{
		Role:      roleAssistant,
		Type:      MessageTypeToolCall,
		ToolName:  "Bash",
		ToolTitle: "Bash",
		ToolInput: `{"command":"go test ./..."}`,
	})

	screen := renderScreen(t, app, 80, 24)
	assert.Equal(t, 1, strings.Count(screen.Text(), "go test ./..."),
		"the running tool call belongs to the transcript, not to both")
}

func TestClearInScreenModeEmptiesTheViewport(t *testing.T) {
	app := newScreenApp(t, 20)
	renderScreen(t, app, 80, 24)

	assert.True(t, app.handleCommand("/clear", nil))
	screen := renderScreen(t, app, 80, 24)

	assert.Equal(t, 1, len(app.messages), "only the fresh intro is left")
	assert.Equal(t, roleIntro, app.messages[0].Role)
	assert.NotContains(t, screen.Text(), "answer 19")
	assert.Contains(t, screen.Text(), "Dive")
	assert.True(t, app.viewport.AtBottom)
}
