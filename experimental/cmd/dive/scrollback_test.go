package main

import (
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/tui"
)

func TestRawScrollbackIsTheSourceBehindTheScreen(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true
	app.appendMessage(Message{Role: roleUser, Content: "how do I run the tests?"})
	app.appendMessage(Message{
		Role:    roleAssistant,
		Content: "Like this:\n\n```sh\ngo test ./...\n```\n",
	})

	raw := app.rawTranscript()

	assert.Contains(t, raw, "--- you ---")
	assert.Contains(t, raw, "how do I run the tests?")
	assert.Contains(t, raw, "--- assistant ---")
	assert.Contains(t, raw, "```sh", "the markdown fence is the point of the raw form")
	assert.Contains(t, raw, "go test ./...")
	assert.NotContains(t, raw, "\x1b[", "raw means no styling")
}

func TestRawScrollbackIncludesToolCallsAndTheirOutput(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true
	app.appendMessage(Message{
		Type:            MessageTypeToolCall,
		ToolName:        "Bash",
		ToolTitle:       "Bash",
		ToolDone:        true,
		ToolInput:       `{"command":"go build ./..."}`,
		ToolResultLines: []string{"first line", "second line"},
	})

	raw := app.rawTranscript()

	assert.Contains(t, raw, "--- tool ---")
	assert.Contains(t, raw, "first line")
	assert.Contains(t, raw, "second line", "collapsed on screen, whole here")
}

func TestRawScrollbackSkipsTheAppTalkingToItself(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true
	app.appendMessage(Message{Role: roleNotice, Content: "Copied 3 lines (pbcopy)"})
	app.appendMessage(Message{Role: roleUser, Content: "carry on"})

	raw := app.rawTranscript()

	// The notice is still there — it was on screen — but unlabelled, because
	// nobody said it.
	assert.Contains(t, raw, "Copied 3 lines")
	assert.Equal(t, strings.Count(raw, "---"), 2, "only the user turn is attributed")
}

func TestRawScrollbackFlattensAReportToItsText(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true
	app.appendReport(tui.Stack(tui.Text("Token usage").Bold(), tui.Text("  input 1,024")))

	raw := app.rawTranscript()

	assert.Contains(t, raw, "Token usage")
	assert.Contains(t, raw, "input 1,024")
	assert.NotContains(t, raw, "\x1b[")
}

func TestScrollbackWithoutTheManagedScreenSaysSo(t *testing.T) {
	app, _ := newFakeApp(t)
	app.appendMessage(Message{Role: roleUser, Content: "hello"})

	app.handleScrollbackCommand("")

	assert.Contains(t, app.messages[len(app.messages)-1].Content, "--inline")
}

func TestRepaintWithoutATerminalIsHarmless(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true

	app.repaint()
	app.HandleEvent(tui.KeyEvent{Key: tui.KeyCtrlL})
}
