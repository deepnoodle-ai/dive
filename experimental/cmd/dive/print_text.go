package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/deepnoodle-ai/dive"
)

// passageKind names what a streamed passage holds in print text mode.
type passageKind int

const (
	passageNone passageKind = iota
	passageThinking
	passageText
)

// textPrinter streams an agent run to w for `dive -p --output-format text`.
//
// An agent run spans several LLM turns. Text from different turns is split
// into passages at turn boundaries (a completed message or a tool call), and
// passages are separated by one blank line so a preamble such as "Reading the
// file." does not run into the final answer. Newlines at the edges of a
// passage are dropped so the spacing stays uniform. Once thinking has been
// shown, a "Thinking:" or "Response:" header opens each passage whose kind
// differs from the one before it.
type textPrinter struct {
	w            io.Writer
	showThinking bool

	current      passageKind // kind of the passage being written
	boundary     bool        // a turn boundary was seen since the last write
	pendingNL    int         // trailing newlines held back until more text follows
	wroteAny     bool        // any text or thinking was written
	wroteText    bool        // any response text was written
	wroteThought bool        // any thinking was written
}

func newTextPrinter(w io.Writer, showThinking bool) *textPrinter {
	return &textPrinter{w: w, showThinking: showThinking}
}

// handle consumes one response item from the agent's event callback.
func (p *textPrinter) handle(item *dive.ResponseItem) {
	switch item.Type {
	case dive.ResponseItemTypeMessage, dive.ResponseItemTypeToolCall:
		p.boundary = true
	case dive.ResponseItemTypeModelEvent:
		if item.Event == nil || item.Event.Delta == nil {
			return
		}
		if p.showThinking && item.Event.Delta.Thinking != "" {
			p.write(passageThinking, item.Event.Delta.Thinking)
		}
		if item.Event.Delta.Text != "" {
			p.write(passageText, item.Event.Delta.Text)
		}
	}
}

func (p *textPrinter) write(kind passageKind, s string) {
	if kind != p.current || p.boundary {
		// A passage starts at its first non-newline character.
		s = strings.TrimLeft(s, "\n")
		if s == "" {
			return
		}
		p.startPassage(kind)
	}
	body := strings.TrimRight(s, "\n")
	if body != "" {
		fmt.Fprint(p.w, strings.Repeat("\n", p.pendingNL)+body)
		p.pendingNL = 0
	}
	p.pendingNL += len(s) - len(body)
	p.wroteAny = true
	if kind == passageText {
		p.wroteText = true
	}
}

// startPassage ends the current passage with a blank line and, once thinking
// has been shown and the kind changes, prints the new passage's header.
func (p *textPrinter) startPassage(kind passageKind) {
	if p.wroteAny {
		fmt.Fprint(p.w, "\n\n")
	}
	if kind != p.current {
		switch {
		case kind == passageThinking:
			fmt.Fprintln(p.w, "Thinking:")
			p.wroteThought = true
		case kind == passageText && p.wroteThought:
			fmt.Fprintln(p.w, "Response:")
		}
	}
	p.current = kind
	p.boundary = false
	p.pendingNL = 0
}

// finish ends the output with a newline. When no response text streamed, it
// first prints fallback (the response's final output text) as the response.
func (p *textPrinter) finish(fallback string) {
	if !p.wroteText && fallback != "" {
		p.write(passageText, fallback)
	}
	fmt.Fprintln(p.w)
}
