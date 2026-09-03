package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/deepnoodle-ai/wonton/tui"
)

// The managed screen renders the transcript as application state inside the
// alternate screen, instead of handing each finished message to the terminal's
// scrollback. What that buys is a conversation that reflows on resize, streams
// markdown in place, and can be scrolled, selected and copied from within the
// app. What it costs is the terminal's own scrollback and its find bar, which
// is what --inline is for, and what the exit-time transcript dump gives back to
// everyone else.

// isTerminal reports whether f is a terminal. A variable so the TTY guard can
// be exercised from a test, which may itself be attached to one.
var isTerminal = func(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// Len implements tui.ViewportItems.
func (a *App) Len() int { return len(a.messages) }

// Item implements tui.ViewportItems. The viewport calls this at most once per
// message until touch() invalidates it, so a message's markdown is parsed once
// rather than every frame.
func (a *App) Item(i int) tui.View {
	if i < 0 || i >= len(a.messages) {
		return nil
	}
	return a.messageView(a.messages[i], viewOpts{animate: true})
}

// View implements tui.Application: the scrollable transcript with the same
// footer the inline mode puts under scrollback.
func (a *App) View() tui.View {
	if a.frameMetrics {
		defer func(start time.Time) { a.recordViewTime(time.Since(start)) }(time.Now())
	}
	return tui.Stack(
		tui.PaddingLTRB(1, 0, 1, 0, tui.Viewport(&a.viewport, a).Gap(1)),
		a.footerView(),
	).Gap(0)
}

// recordViewTime keeps a rolling average and worst case of how long building
// the view tree takes. Only the build: measuring and drawing it happen inside
// the runtime afterwards, and the terminal's own metrics cover the flush.
func (a *App) recordViewTime(d time.Duration) {
	a.viewTimeTotal += d
	a.viewTimeCount++
	if d > a.viewTimeMax {
		a.viewTimeMax = d
	}
}

// frameMetricsView reports what a frame costs, for DIVE_DEBUG_FRAMES=1. The
// two numbers are separate stages: building the view tree in this process, and
// the terminal diffing and writing it.
func (a *App) frameMetricsView() tui.View {
	if !a.frameMetrics || a.terminal == nil || a.viewTimeCount == 0 {
		return nil
	}
	m := a.terminal.GetMetrics()
	avgView := a.viewTimeTotal / time.Duration(a.viewTimeCount)
	fps := 0.0
	if elapsed := time.Since(a.startTime).Seconds(); elapsed > 0 {
		fps = float64(m.TotalFrames) / elapsed
	}
	return tui.Text(" view %.1fms/%.1fms max · flush %.1fms/%.1fms max · %.1f fps · %d msgs",
		float64(avgView.Microseconds())/1000,
		float64(a.viewTimeMax.Microseconds())/1000,
		float64(m.AvgTimePerFrame.Microseconds())/1000,
		float64(m.MaxFrameTime.Microseconds())/1000,
		fps,
		len(a.messages),
	).Style(hintStyle())
}

// footerView is the fixed bottom of the managed screen: the dialog when one is
// up, otherwise the scroll indicator, the live progress line, and the input
// area. It keeps the same focus IDs as the inline footer, so dialogs and the
// input behave identically in both modes.
func (a *App) footerView() tui.View {
	if a.dialogState != nil && a.dialogState.Active {
		return tui.Stack(tui.Text(""), a.dialogView())
	}

	views := make([]tui.View, 0, 8)

	// Tell the user what they are not looking at, and how to get back to it.
	if !a.viewport.AtBottom && a.viewport.LinesBelow > 0 {
		views = append(views, tui.Group(
			tui.Text(" ↓ %d new line%s", a.viewport.LinesBelow, pluralSuffix(a.viewport.LinesBelow)).
				Style(tui.NewStyle().WithFgRGB(accentDim)),
			tui.Text(" · End to jump to the latest").Style(hintStyle()),
		))
	}

	if a.processing {
		if live := a.buildLiveView(false); live != nil {
			views = append(views, live)
		}
	} else if a.showTodos && len(a.todos) > 0 {
		views = append(views, a.todoListView(viewOpts{animate: true}))
	}

	views = append(views, a.inputAreaView()...)
	return tui.Stack(views...).Gap(0)
}

// runScreen runs the CLI in the managed screen.
//
// The runtime is built by hand rather than through tui.Run because the app
// needs the *Runtime (agent goroutines send events through it) and the
// *Terminal (for the drag-capable mouse mode, frame metrics, and the
// exit-time ordering of Close before the transcript dump).
func (a *App) runScreen() (err error) {
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		// tui.Run tolerates a non-TTY (it skips raw mode for tests), so unlike
		// the inline runner nothing else will stop us from writing alternate
		// screen escapes into a pipe.
		return errors.New("interactive mode needs a terminal; use --print for piped or redirected use")
	}

	terminal, err := tui.NewTerminal()
	if err != nil {
		return err
	}
	// Runs on the way out however we leave, a panic included. Close is
	// idempotent, so the ordinary path can close early to get the transcript
	// dump onto the restored screen; the resume line goes last either way,
	// because a session the user cannot find again is the one thing a crash
	// must not also cost them.
	defer func() {
		a.restoreAlternateScroll()
		terminal.Close()
		a.printResumeLine(os.Stdout)
	}()

	terminal.EnableAlternateScreen()
	terminal.HideCursor()
	terminal.EnableBracketedPaste()
	defer terminal.DisableBracketedPaste()
	defer terminal.DisableMouseTracking()

	if a.frameMetrics {
		terminal.EnableMetrics()
	}

	a.terminal = terminal
	a.clipboard = newClipboardCopier(terminal)
	// DIVE_DISABLE_MOUSE is for a terminal or a user the gestures do not suit.
	// It is the same switch /mouse throws, thrown before the first frame.
	a.setMouseReporting(os.Getenv("DIVE_DISABLE_MOUSE") != "1")

	a.startTime = time.Now()
	runtime := tui.NewRuntime(terminal, a, 30)
	// Match the inline runner: enable the Kitty keyboard protocol outright
	// rather than probing for it, so Shift+Enter still inserts a newline under
	// tmux (where the probe is skipped) and startup does not wait on a reply.
	runtime.SetKittyKeyboard(true)
	a.runtime = runtime
	a.runner = runtime
	a.viewport.Follow = true

	a.refreshGitBranch()
	if a.resumeSessionID != "" {
		a.appendSessionHistory()
	} else {
		a.appendIntro()
	}
	a.appendSelectionHint()

	if a.initialPrompt != "" {
		// The runtime's event channel exists before Run, so unlike the inline
		// path this needs no delay: the event is queued now and handled after
		// Init.
		a.runner.SendEvent(initialPromptEvent{baseEvent: newBaseEvent(), prompt: a.initialPrompt})
	}

	err = runtime.Run()
	if err != nil {
		return err
	}

	// Close first. The alternate screen's contents land in scrollback nowhere,
	// so anything written before the restore leaves no trace at all — the dump
	// has to go to the screen the user is left looking at.
	a.restoreAlternateScroll()
	terminal.Close()
	a.printExitTranscript(os.Stdout)
	return nil
}

// exitTranscriptMaxLines caps what the dump writes. A long session can hold far
// more than any terminal keeps, and the tail is the part anyone scrolls back to.
const exitTranscriptMaxLines = 2000

// printExitTranscript writes the conversation to the restored screen on the way
// out, which is the managed screen's answer to having no scrollback of its own.
// Static: no pulsing markers, no live progress, just what was said.
func (a *App) printExitTranscript(w io.Writer) {
	text := a.exitTranscript(a.dumpWidth())
	if text == "" {
		return
	}
	fmt.Fprint(w, text)
}

// exitTranscript renders every message at width and returns the last
// exitTranscriptMaxLines of it, with a note above when anything was dropped.
func (a *App) exitTranscript(width int) string {
	views := make([]tui.View, 0, len(a.messages)*2)
	for _, msg := range a.messages {
		if view := a.messageView(msg, viewOpts{}); view != nil {
			views = append(views, tui.Text(""), view)
		}
	}
	if len(views) == 0 {
		return ""
	}
	rendered := tui.Sprint(tui.Stack(views...).Gap(0), tui.WithWidth(width))
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	if len(lines) > exitTranscriptMaxLines {
		lines = append(
			[]string{fmt.Sprintf("… %d earlier lines omitted", len(lines)-exitTranscriptMaxLines)},
			lines[len(lines)-exitTranscriptMaxLines:]...,
		)
	}
	return strings.Join(lines, "\n") + "\n"
}

// printResumeLine says where the conversation went. It prints even when the run
// ended badly: a session the user cannot find again is the one thing an error
// must not also cost them.
func (a *App) printResumeLine(w io.Writer) {
	if a.currentSession == nil {
		return
	}
	id := a.currentSession.ID()
	if id == "" {
		return
	}
	fmt.Fprintf(w, "\nSession %s saved — resume with dive --resume %s\n", id, id)
}

// dumpWidth is the width the exit transcript is rendered at: the terminal's,
// as it was at the last resize, falling back to the conventional 80.
func (a *App) dumpWidth() int {
	if a.termWidth > 0 {
		return a.termWidth
	}
	return 80
}

// handleResize records the terminal size. The viewport works out its own
// height from what the footer leaves, so there is nothing to recompute here —
// but the app needs the size for the frame-metrics line, and the runtime sends
// a ResizeEvent before the first render, which is where the initial size comes
// from.
func (a *App) handleResize(e tui.ResizeEvent) {
	a.termWidth, a.termHeight = e.Width, e.Height
}

// handleScreenKey handles the keys that only mean something when there is a
// scrollable transcript. Returns true if the key was consumed.
//
// The focused input sees keys first and consumes Home/End (and Home/End with
// any modifier), so this is reached through the input's OnKey hook — see
// handleInputNavKey, which is also consulted for keys that arrive at the app
// directly. PgUp/PgDn the input never wants and would reach either way.
func (a *App) handleScreenKey(e tui.KeyEvent) bool {
	if !a.screenMode {
		return false
	}
	switch e.Key {
	case tui.KeyPageUp:
		a.viewport.PageUp()
		return true
	case tui.KeyPageDown:
		a.viewport.PageDown()
		return true
	case tui.KeyHome:
		// Ctrl+Home is the transcript's; plain Home is only ours when there is
		// no cursor in the input for it to move.
		if e.Ctrl || a.inputText == "" {
			a.viewport.ScrollToTop()
			return true
		}
	case tui.KeyEnd:
		if e.Ctrl || a.inputText == "" {
			a.viewport.ScrollToBottom()
			return true
		}
	}
	return false
}

// snapToBottom returns the user to the live end of the transcript. Typing,
// pasting, and submitting all mean "I am done reading back", so they follow
// again even if the user had scrolled up.
func (a *App) snapToBottom() {
	if a.screenMode {
		a.viewport.ScrollToBottom()
	}
}

// wheelLines is how far one wheel notch scrolls. Pagers conventionally use
// three, but a trackpad delivers notches continuously, so three overshoots on
// exactly the hardware most of this CLI's users have.
const wheelLines = 1

// Where View() puts the viewport on screen. The transcript is the first child
// of the outer Stack, inset by one column on each side, so a mouse position
// becomes a viewport position by subtracting these. Change View()'s padding and
// these change with it.
const (
	viewportOriginX = 1
	viewportOriginY = 0
)

// handleScreenMouse drives scrolling, selection, and the transcript's own click
// targets. It returns commands the way HandleEvent does, though today every
// path either scrolls or hands the copy off to a goroutine.
func (a *App) handleScreenMouse(e tui.MouseEvent) {
	if !a.screenMode {
		return
	}

	if e.Type == tui.MouseScroll {
		switch e.Button {
		case tui.MouseButtonWheelUp:
			a.viewport.ScrollBy(-wheelLines)
		case tui.MouseButtonWheelDown:
			a.viewport.ScrollBy(wheelLines)
		}
		return
	}

	if !a.mouseEnabled {
		return
	}

	// The viewport works in its own coordinates, and it needs to see positions
	// outside itself: a drag held past an edge is what auto-scroll reads.
	local := e
	local.X, local.Y = e.X-viewportOriginX, e.Y-viewportOriginY

	// A press below the transcript is the footer's — the input field, or the
	// scroll indicator. Anchoring a selection there would select the last line
	// of the transcript from a click the user aimed somewhere else.
	if e.Type == tui.MousePress && !a.inViewport(local.X, local.Y) {
		return
	}

	if a.viewport.HandleMouse(local) {
		// A gesture that has finished and left a highlight behind is a copy:
		// the end of a drag, a double-click's word, a triple-click's line.
		if a.copyOnSelect && !a.viewport.SelectionActive() && a.viewport.HasSelection() {
			a.copySelection()
		}
		return
	}

	// The viewport declined it, so it is ours to place.
	if e.Type == tui.MouseClick && e.Button == tui.MouseButtonLeft {
		a.handleScreenClick(e.Y, local.X, local.Y)
	}
}

// inViewport reports whether a viewport-local position is inside the transcript
// rather than the footer beneath it.
func (a *App) inViewport(x, y int) bool {
	return x >= 0 && x < a.viewport.Width && y >= 0 && y < a.viewport.Height
}

// handleScreenClick acts on a left click the viewport did not want. It takes
// two coordinate systems because the two targets live in different ones: the
// transcript's are the viewport's, the scroll indicator's are the screen's.
func (a *App) handleScreenClick(screenY, x, y int) {
	// The indicator is the footer's first row, and saying "N new lines below"
	// while not being a way to reach them would be a poor joke.
	if !a.viewport.AtBottom && a.viewport.LinesBelow > 0 && screenY == a.viewport.Height {
		a.viewport.ScrollToBottom()
		return
	}

	item, line, ok := a.viewport.ItemAt(x, y)
	if !ok || item >= len(a.messages) {
		return
	}
	msg := a.messages[item]
	switch {
	case msg.Type == MessageTypeToolCall && line == 0 && len(msg.ToolResultLines) > 1:
		a.messages[item].Expanded = !msg.Expanded
		a.touch(item)
	case msg.Role == roleReport && line == 0:
		a.toggleReportCollapse(item)
	}
}

// toggleReportCollapse folds a command's report down to a single line, or opens
// it back up. A report is a pre-built view with no text behind it, so the line
// left in its place is captured by rendering it once, here, rather than on
// every frame it spends collapsed.
func (a *App) toggleReportCollapse(i int) {
	msg := a.messages[i]
	if msg.Collapsed {
		a.messages[i].Collapsed = false
		a.touch(i)
		return
	}
	if msg.CollapsedTitle == "" {
		a.messages[i].CollapsedTitle = firstNonBlankLine(msg.View, a.reportWidth())
	}
	a.messages[i].Collapsed = true
	a.touch(i)
}

// reportWidth is the width a report is rendered at when capturing its title.
// The viewport's own width once it has rendered; a sane guess before that.
func (a *App) reportWidth() int {
	if a.viewport.Width > 0 {
		return a.viewport.Width
	}
	return 80
}

// copySelection puts the current selection on the clipboard. The text comes
// from the viewport, which re-renders the selected messages rather than reading
// the screen, so a selection that ran off an edge during a drag copies whole.
func (a *App) copySelection() {
	a.copyToClipboard(a.viewport.SelectedText())
}

// copyToClipboard runs the clipboard ladder and reports what happened. Called
// from the event loop; the ladder forks a process on its first two rungs, so
// the work goes to a goroutine and the answer comes back as a notice.
func (a *App) copyToClipboard(text string) {
	if strings.TrimSpace(text) == "" {
		a.appendNotice("Nothing to copy.")
		return
	}
	copier := a.clipboard
	if copier == nil {
		a.appendNotice("Copying needs the managed screen; this session is running with --inline.")
		return
	}
	go func() {
		report, err := copier(text)
		if err != nil {
			a.postNotice("Copy failed: %v", err)
			return
		}
		a.postNotice("%s", report.notice())
	}()
}

// setMouseReporting turns the app's mouse gestures on or off, which is the only
// way to hand selection back to the terminal for users whose terminal has no
// bypass modifier — or who would rather not hold one.
//
// Alternate scroll (CSI ?1007) goes with it: with reporting off the terminal
// translates the wheel into arrow keys, and in the alternate screen those would
// land in the input field as history recall rather than scrolling anything. It
// is only ever turned back on if we were the ones who turned it off — there is
// no way to ask a terminal what the mode was, so the alternative is leaving a
// setting changed behind us on the strength of a guess.
func (a *App) setMouseReporting(on bool) {
	a.mouseEnabled = on
	if a.terminal == nil {
		return
	}
	if on {
		a.terminal.EnableMouseDrag()
		a.restoreAlternateScroll()
		return
	}
	a.viewport.ClearSelection()
	a.terminal.DisableMouseTracking()
	_ = a.terminal.WriteRaw([]byte("\x1b[?1007l"))
	a.altScrollDisabled = true
}

// restoreAlternateScroll undoes a ?1007l we sent, and does nothing otherwise.
func (a *App) restoreAlternateScroll() {
	if !a.altScrollDisabled || a.terminal == nil {
		return
	}
	_ = a.terminal.WriteRaw([]byte("\x1b[?1007h"))
	a.altScrollDisabled = false
}

// appendSelectionHint says how to select, once, under the intro. The managed
// screen takes the terminal's own drag-to-select away and puts its own in the
// same gesture; the part worth stating up front is the way back out, since a
// user who does not know the modifier will conclude that copying is broken.
func (a *App) appendSelectionHint() {
	if !a.mouseEnabled {
		a.appendNotice("Mouse reporting is off — your terminal's own selection is in charge. /mouse to change that.")
		return
	}
	a.appendNotice("Drag to select and copy · hold %s to select with your terminal instead · /help for more",
		nativeSelectionModifier())
}

// toggleMouse is /mouse.
func (a *App) toggleMouse() {
	if !a.screenMode {
		a.appendNotice("Mouse reporting is only used by the managed screen.")
		return
	}
	a.setMouseReporting(!a.mouseEnabled)
	if a.mouseEnabled {
		a.appendNotice("Mouse on — drag to select, double-click a word, triple-click a line.")
		return
	}
	// Reporting off means no mouse bytes at all, wheel included: iTerm2 and
	// most others only translate the wheel into arrow keys with alternate
	// scroll on, which we have just turned off precisely so it does not land
	// in the input. So say where scrolling went.
	a.appendNotice("Mouse off — your terminal's own selection is back. Scroll with PgUp/PgDn; /mouse turns it on again.")
}

// firstNonBlankLine is the first line of a rendered view that has anything on
// it. The view is rendered through a screen rather than to a string so the text
// comes back without the styling a string render carries.
func firstNonBlankLine(view tui.View, width int) string {
	if view == nil {
		return ""
	}
	screen := tui.SprintScreen(view, tui.WithWidth(width))
	for _, line := range strings.Split(screen.Text(), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// appendSessionHistory loads a resumed session's messages into the transcript,
// so the managed screen can scroll back through them like any other message.
// Inline mode prints them to scrollback instead (printSessionHistoryToScrollback).
func (a *App) appendSessionHistory() {
	a.appendIntro()
	if a.currentSession == nil {
		return
	}
	sessionMsgs, err := a.currentSession.Messages(a.ctx)
	if err != nil {
		return
	}
	for _, msg := range sessionMsgs {
		for _, m := range a.convertLLMMessage(msg, toolResultsByID(sessionMsgs)) {
			a.appendMessage(m)
		}
	}
	a.viewport.ScrollToBottom()
}
