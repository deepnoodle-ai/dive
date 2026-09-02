package main

import (
	"errors"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/deepnoodle-ai/wonton/tui"
)

// The managed screen renders the transcript as application state inside the
// alternate screen, instead of handing each finished message to the terminal's
// scrollback. What that buys is a conversation that reflows on resize, streams
// markdown in place, and can be scrolled and (from Phase 5) selected from
// within the app. What it costs is the terminal's own scrollback, which is why
// it is opt-in behind --screen for now.

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
	// Idempotent, and it also runs if the runtime re-panics after restoring.
	defer terminal.Close()

	terminal.EnableAlternateScreen()
	terminal.HideCursor()
	terminal.EnableBracketedPaste()
	defer terminal.DisableBracketedPaste()
	terminal.EnableMouseDrag()
	defer terminal.DisableMouseTracking()

	if a.frameMetrics {
		terminal.EnableMetrics()
	}

	a.startTime = time.Now()
	runtime := tui.NewRuntime(terminal, a, 30)
	// Match the inline runner: enable the Kitty keyboard protocol outright
	// rather than probing for it, so Shift+Enter still inserts a newline under
	// tmux (where the probe is skipped) and startup does not wait on a reply.
	runtime.SetKittyKeyboard(true)
	a.terminal = terminal
	a.runtime = runtime
	a.runner = runtime
	a.viewport.Follow = true

	a.refreshGitBranch()
	if a.resumeSessionID != "" {
		a.appendSessionHistory()
	} else {
		a.appendIntro()
	}

	if a.initialPrompt != "" {
		// The runtime's event channel exists before Run, so unlike the inline
		// path this needs no delay: the event is queued now and handled after
		// Init.
		a.runner.SendEvent(initialPromptEvent{baseEvent: newBaseEvent(), prompt: a.initialPrompt})
	}

	return runtime.Run()
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

// handleScreenMouse turns wheel events into scrolling.
func (a *App) handleScreenMouse(e tui.MouseEvent) {
	if !a.screenMode || e.Type != tui.MouseScroll {
		return
	}
	switch e.Button {
	case tui.MouseButtonWheelUp:
		a.viewport.ScrollBy(-wheelLines)
	case tui.MouseButtonWheelDown:
		a.viewport.ScrollBy(wheelLines)
	}
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
