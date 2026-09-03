package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/deepnoodle-ai/wonton/tui"
)

// /copy is the keyboard's way to what a drag does with the mouse, and the only
// way to get at a code block without dragging across every line of it.
//
//	/copy        the current selection, or a list of the last reply's code blocks
//	/copy N      the Nth code block of the last reply
//	/copy all    every code block of the last reply, in order
//
// The text copied is the source the model sent, not the cells it was drawn
// into: a fenced block that wrapped on screen copies back as the lines the
// model wrote, which is the only version that can be pasted into a file.

// handleCopyCommand is /copy.
func (a *App) handleCopyCommand(args string) {
	arg := strings.TrimSpace(args)

	if arg == "" && a.viewport.HasSelection() {
		a.copySelection()
		return
	}

	blocks := codeBlocks(a.lastAssistantText())
	if len(blocks) == 0 {
		if arg == "" {
			a.appendNotice("Nothing selected, and the last reply has no code blocks.")
			return
		}
		a.appendNotice("The last reply has no code blocks to copy.")
		return
	}

	switch {
	case arg == "":
		a.appendReport(codeBlockListView(blocks))
	case strings.EqualFold(arg, "all"):
		texts := make([]string, 0, len(blocks))
		for _, b := range blocks {
			texts = append(texts, b.text)
		}
		a.copyToClipboard(strings.Join(texts, "\n"))
	default:
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > len(blocks) {
			a.appendNotice("No code block %s — the last reply has %d. Try /copy for the list.", arg, len(blocks))
			return
		}
		a.copyToClipboard(blocks[n-1].text)
	}
}

// lastAssistantText is the prose of the most recent reply, which is where
// /copy looks for code blocks. Notices, reports and tool calls in between do
// not displace it; another user message does, since a reply older than the
// user's last question is not "the last reply" in any useful sense.
func (a *App) lastAssistantText() string {
	for i := len(a.messages) - 1; i >= 0; i-- {
		switch a.messages[i].Role {
		case roleAssistant:
			return a.messages[i].Content
		case roleUser:
			return ""
		}
	}
	return ""
}

// codeBlock is one fenced block of a markdown reply.
type codeBlock struct {
	lang string
	text string
}

// lines is how many lines the block holds.
func (b codeBlock) lines() int { return countLines(b.text) }

// summary is the block's first line with anything on it, for the list.
func (b codeBlock) summary() string {
	for _, line := range strings.Split(b.text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// codeBlocks pulls the fenced blocks out of markdown, in order.
//
// The scan is deliberately literal: a fence is a line whose first non-space run
// is three or more backticks or tildes, and it closes on a line whose fence is
// at least as long and of the same character. That is what CommonMark says, and
// it is what makes a block containing ``` inside a ```` fence come out whole.
func codeBlocks(markdown string) []codeBlock {
	if markdown == "" {
		return nil
	}
	var blocks []codeBlock
	var open bool
	var fenceChar byte
	var fenceLen int
	var lang string
	var body []string

	for _, line := range strings.Split(markdown, "\n") {
		char, length, rest, isFence := parseFence(line)
		switch {
		case !open && isFence:
			open, fenceChar, fenceLen = true, char, length
			lang = ""
			if fields := strings.Fields(rest); len(fields) > 0 {
				lang = fields[0]
			}
			body = body[:0]
		case open && isFence && char == fenceChar && length >= fenceLen && strings.TrimSpace(rest) == "":
			blocks = append(blocks, codeBlock{lang: lang, text: strings.Join(body, "\n")})
			open = false
		case open:
			body = append(body, line)
		}
	}
	// An unclosed fence is still a block: a reply cut off mid-stream is exactly
	// when someone wants the code out of it.
	if open && len(body) > 0 {
		blocks = append(blocks, codeBlock{lang: lang, text: strings.Join(body, "\n")})
	}
	return blocks
}

// parseFence reports whether line opens or closes a fence, along with the fence
// character, its length, and whatever follows it on the line.
func parseFence(line string) (char byte, length int, rest string, ok bool) {
	trimmed := strings.TrimLeft(line, " ")
	// More than three leading spaces is an indented code block, not a fence.
	if len(line)-len(trimmed) > 3 || trimmed == "" {
		return 0, 0, "", false
	}
	char = trimmed[0]
	if char != '`' && char != '~' {
		return 0, 0, "", false
	}
	for length < len(trimmed) && trimmed[length] == char {
		length++
	}
	if length < 3 {
		return 0, 0, "", false
	}
	rest = trimmed[length:]
	// An info string may not contain a backtick, which is what keeps inline
	// code spans from being read as fences.
	if char == '`' && strings.ContainsRune(rest, '`') {
		return 0, 0, "", false
	}
	return char, length, rest, true
}

// codeBlockListView is what /copy shows when there is nothing selected: the
// blocks it could copy, numbered the way /copy N wants them.
func codeBlockListView(blocks []codeBlock) tui.View {
	views := []tui.View{
		tui.Text("Code blocks in the last reply:").Bold(),
	}
	for i, b := range blocks {
		label := b.lang
		if label == "" {
			label = "text"
		}
		views = append(views, tui.Group(
			tui.Text("  %d  ", i+1).Style(hintStyle()),
			tui.Text("%-10s", label).Style(hintStyle()),
			tui.Text("%s", truncateRunes(b.summary(), 48)),
			tui.Text("  %d line%s", b.lines(), pluralSuffix(b.lines())).Style(hintStyle()),
		))
	}
	views = append(views,
		tui.Text(""),
		tui.Text("  /copy %s to copy one, /copy all for every block.", numberRange(len(blocks))).Style(hintStyle()),
	)
	return tui.Stack(views...).Gap(0)
}

// numberRange renders the choices /copy N accepts.
func numberRange(n int) string {
	if n == 1 {
		return "1"
	}
	return fmt.Sprintf("1-%d", n)
}

// truncateRunes shortens s to at most n display runes, with an ellipsis. Runes,
// not bytes: slicing bytes splits a multi-byte character into garbage.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}
