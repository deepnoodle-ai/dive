package main

import (
	"fmt"
	"time"

	"github.com/deepnoodle-ai/wonton/tui"
)

// Message roles. Everything the CLI shows the user is a message in a.messages,
// so the transcript is a single, orderable list rather than a mix of app state
// and terminal side effects.
const (
	roleUser      = "user"      // what the user typed
	roleAssistant = "assistant" // model prose, rendered as markdown
	roleReasoning = "reasoning" // streamed thinking
	roleSystem    = "system"    // errors, shown in the warning style
	roleContext   = "context"   // input the app injected on the user's behalf
	roleIntro     = "intro"     // the splash box
	roleNotice    = "notice"    // a dim one-liner (warnings, status, hints)
	roleReport    = "report"    // a pre-built view (/usage, /help, /todos, /context)
)

// viewOpts carries what differs between renderings of the same message. There
// is one message renderer; the caller says whether it is drawing a live frame
// or a final one.
type viewOpts struct {
	// animate pulses the marker of a still-running tool call. False for
	// anything final: scrollback, the exit dump, goldens.
	animate bool

	// expanded shows a tool result in full instead of its first line plus a
	// "… +N lines" count.
	expanded bool
}

// uiRunner is the part of the UI runner the app drives from any goroutine.
// *tui.InlineApp satisfies it today; the managed screen's *tui.Runtime will
// satisfy it too, and tests inject a recording fake.
type uiRunner interface {
	SendEvent(tui.Event)
	Stop()
}

// scrollbackWriter is the inline runner's transcript sink: messages become
// terminal scrollback as they are finalized. The managed screen owns its
// transcript instead and leaves this nil, which turns every emit into a no-op.
type scrollbackWriter interface {
	Print(tui.View)
	ClearScrollback()
}

// noticeEvent carries a notice from a background goroutine to the event loop,
// where it can be appended to the transcript safely.
type noticeEvent struct {
	baseEvent
	text string
}

// appendMessage appends a message and returns its index. Event-loop goroutine
// only, like every other write to a.messages.
func (a *App) appendMessage(msg Message) int {
	if msg.Time.IsZero() {
		msg.Time = time.Now()
	}
	a.messages = append(a.messages, msg)
	return len(a.messages) - 1
}

// touch records an in-place mutation of message i. Phase 4 forwards this to
// the viewport's per-item render cache; for now it only bumps Rev.
func (a *App) touch(i int) {
	if i < 0 || i >= len(a.messages) {
		return
	}
	a.messages[i].Rev++
}

// appendNotice adds a dim one-line notice and shows it right away. Event-loop
// goroutine only — use postNotice from anywhere else.
func (a *App) appendNotice(format string, args ...any) {
	a.emit(a.appendMessage(Message{Role: roleNotice, Content: fmt.Sprintf(format, args...)}))
}

// appendMarkedNotice is appendNotice with an accented glyph in front of the text.
func (a *App) appendMarkedNotice(marker, format string, args ...any) {
	a.emit(a.appendMessage(Message{
		Role:    roleNotice,
		Marker:  marker,
		Content: fmt.Sprintf(format, args...),
	}))
}

// appendSystem adds a message in the warning style — errors the user needs to
// see, not incidental status. Event-loop goroutine only.
func (a *App) appendSystem(format string, args ...any) {
	a.emit(a.appendMessage(Message{Role: roleSystem, Content: fmt.Sprintf(format, args...)}))
}

// appendReport adds a pre-built view (a command's output) to the transcript.
// Event-loop goroutine only. A nil view is ignored.
func (a *App) appendReport(view tui.View) {
	if view == nil {
		return
	}
	a.emit(a.appendMessage(Message{Role: roleReport, View: view}))
}

// postNotice queues a notice from any goroutine. The event loop appends it.
func (a *App) postNotice(format string, args ...any) {
	if a.runner == nil {
		return
	}
	a.runner.SendEvent(noticeEvent{baseEvent: newBaseEvent(), text: fmt.Sprintf(format, args...)})
}

// emit writes message i to inline scrollback if it has not been written yet.
// No-op when there is no scrollback sink, which is what makes every append site
// safe to reuse from the managed screen.
func (a *App) emit(i int) {
	if i < 0 || i >= len(a.messages) || a.messages[i].emitted {
		return
	}
	a.messages[i].emitted = true
	if a.scrollback == nil {
		return
	}
	if view := a.messageView(a.messages[i], viewOpts{}); view != nil {
		a.scrollback.Print(tui.Stack(tui.Text(""), view))
	}
}

// emitPending writes every not-yet-emitted message to inline scrollback as a
// single Print, which is what makes a completed turn appear in one repaint.
func (a *App) emitPending() {
	views := make([]tui.View, 0, 8)
	for i := range a.messages {
		if a.messages[i].emitted {
			continue
		}
		a.messages[i].emitted = true
		if a.scrollback == nil {
			continue
		}
		if view := a.messageView(a.messages[i], viewOpts{}); view != nil {
			views = append(views, tui.Text(""), view)
		}
	}
	if len(views) > 0 {
		a.scrollback.Print(tui.Stack(views...))
	}
}
