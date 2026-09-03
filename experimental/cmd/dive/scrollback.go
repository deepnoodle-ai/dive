package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/deepnoodle-ai/wonton/tui"
)

// /scrollback is the managed screen's answer to the two things the alternate
// screen took away: your terminal's find bar, and selecting more than a
// screenful at a time.
//
// It leaves the alternate screen, writes the conversation into the terminal's
// own scrollback, and waits. While it is up, everything the terminal has always
// been able to do to text applies — Cmd+F, click-drag across pages, the
// scrollbar — because it is the terminal's text, not ours. That is a far
// smaller thing to build than a search and a selection model inside the app,
// and a far better one to use.
//
//	/scrollback      the conversation as it was rendered
//	/scrollback raw  the source behind it, for pasting somewhere else

// handleScrollbackCommand is /scrollback.
func (a *App) handleScrollbackCommand(args string) {
	if a.runtime == nil {
		a.appendNotice("/scrollback needs the managed screen; this session is running with --inline.")
		return
	}
	raw := strings.EqualFold(strings.TrimSpace(args), "raw")
	if len(a.messages) == 0 {
		a.appendNotice("Nothing to show yet.")
		return
	}

	err := a.runtime.Suspend(func(keys <-chan tui.Event) {
		a.writeScrollback(os.Stdout, raw)
		// Enter, not any key: leaving the alternate screen also leaves raw
		// mode, so the terminal line-buffers and nothing reaches us until the
		// user presses it.
		fmt.Fprint(os.Stdout, "\n[ Select and copy above. Press Enter to go back. ]\n")
		<-keys
	})
	if err != nil {
		a.appendNotice("Returning from /scrollback: %v", err)
	}
}

// writeScrollback writes the conversation to w. Rendered, it is what was on
// screen; raw, it is the text behind it — markdown as the model wrote it, tool
// results unwrapped — which is the version worth pasting into anything else.
func (a *App) writeScrollback(w io.Writer, raw bool) {
	if raw {
		fmt.Fprint(w, a.rawTranscript())
		return
	}
	fmt.Fprint(w, a.exitTranscript(a.dumpWidth()))
}

// rawTranscript is the conversation as plain text, with each message labelled
// by who said it. No styling and no box drawing: a paste of this is a
// transcript, not a screenshot of one.
func (a *App) rawTranscript() string {
	var b strings.Builder
	for _, msg := range a.messages {
		body := a.rawMessage(msg)
		if strings.TrimSpace(body) == "" {
			continue
		}
		if label := rawRoleLabel(msg); label != "" {
			fmt.Fprintf(&b, "\n%s\n", label)
		}
		b.WriteString(strings.TrimRight(body, "\n"))
		b.WriteString("\n")
	}
	return b.String()
}

// rawRoleLabel names the speaker, or returns "" for the messages that are the
// app talking to itself rather than part of the conversation.
func rawRoleLabel(msg Message) string {
	if msg.Type == MessageTypeToolCall {
		return "--- tool ---"
	}
	switch msg.Role {
	case roleUser:
		return "--- you ---"
	case roleAssistant:
		return "--- assistant ---"
	case roleReasoning:
		return "--- thinking ---"
	case roleSystem:
		return "--- error ---"
	}
	return ""
}

// rawMessage is one message's text. A report has no text behind it — it was
// built as a view — so it is rendered through a screen to get its plain form.
func (a *App) rawMessage(msg Message) string {
	if msg.Type == MessageTypeToolCall {
		var b strings.Builder
		fmt.Fprintf(&b, "%s\n", formatToolCall(msg.ToolTitle, msg.ToolName, msg.ToolInput))
		for _, line := range msg.ToolResultLines {
			fmt.Fprintf(&b, "  %s\n", line)
		}
		return b.String()
	}
	if msg.Role == roleReport {
		if msg.View == nil {
			return ""
		}
		return plainText(msg.View, a.dumpWidth())
	}
	return msg.Content
}

// plainText renders a view and returns its text with the styling dropped.
func plainText(view tui.View, width int) string {
	screen := tui.SprintScreen(view, tui.WithWidth(width))
	lines := strings.Split(screen.Text(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// repaint discards what the terminal believes is on screen, so the next frame
// writes every cell instead of diffing against a picture something else has
// since changed. Ctrl+L, and the answer to a host that leaves fragments behind.
func (a *App) repaint() {
	if a.terminal != nil {
		a.terminal.Invalidate()
	}
}
