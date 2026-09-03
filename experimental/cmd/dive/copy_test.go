package main

import (
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/tui"
)

func TestCodeBlocksReadsFencedBlocksInOrder(t *testing.T) {
	blocks := codeBlocks("Here you go:\n\n```go\nfunc main() {}\n```\n\nand to run it:\n\n```sh\ngo test ./...\n```\n")

	assert.Equal(t, len(blocks), 2)
	assert.Equal(t, blocks[0].lang, "go")
	assert.Equal(t, blocks[0].text, "func main() {}")
	assert.Equal(t, blocks[1].lang, "sh")
	assert.Equal(t, blocks[1].text, "go test ./...")
}

func TestCodeBlocksKeepsAFenceNestedInsideALongerOne(t *testing.T) {
	blocks := codeBlocks("````md\nSome markdown:\n```go\nx := 1\n```\n````\n")

	assert.Equal(t, len(blocks), 1, "the inner fence is content, not a boundary")
	assert.Equal(t, blocks[0].lang, "md")
	assert.Contains(t, blocks[0].text, "```go")
}

func TestCodeBlocksIgnoresInlineCodeAndTakesAnUnclosedFence(t *testing.T) {
	assert.Equal(t, len(codeBlocks("call `go test` and see")), 0,
		"an inline span is not a fence")

	blocks := codeBlocks("still writing:\n\n```go\nfunc half(")
	assert.Equal(t, len(blocks), 1, "a reply cut off mid-stream still has its code")
	assert.Equal(t, blocks[0].text, "func half(")
}

func TestCodeBlocksTakesTildeFencesAndBlocksWithNoLanguage(t *testing.T) {
	blocks := codeBlocks("~~~\nplain\n~~~\n\n```\nalso plain\n```\n")

	assert.Equal(t, len(blocks), 2)
	assert.Equal(t, blocks[0].lang, "")
	assert.Equal(t, blocks[0].text, "plain")
	assert.Equal(t, blocks[1].text, "also plain")
}

func TestCopyNTakesTheNthCodeBlockOfTheLastReply(t *testing.T) {
	app, clip := newSelectingApp(t, 0)
	app.appendMessage(Message{Role: roleUser, Content: "how?"})
	app.appendMessage(Message{
		Role:    roleAssistant,
		Content: "Like this:\n\n```go\nfunc main() {}\n```\n\nor:\n\n```sh\nmake\n```\n",
	})

	app.handleCopyCommand("2")
	assert.Equal(t, clip.next(t), "make")

	app.handleCopyCommand("1")
	assert.Equal(t, clip.next(t), "func main() {}")
}

func TestCopyAllJoinsEveryCodeBlock(t *testing.T) {
	app, clip := newSelectingApp(t, 0)
	app.appendMessage(Message{
		Role:    roleAssistant,
		Content: "```go\nfunc main() {}\n```\n\n```sh\nmake\n```\n",
	})

	app.handleCopyCommand("all")
	assert.Equal(t, clip.next(t), "func main() {}\nmake")
}

func TestCopyOutOfRangeSaysSoRatherThanCopyingTheWrongThing(t *testing.T) {
	app, clip := newSelectingApp(t, 0)
	app.appendMessage(Message{Role: roleAssistant, Content: "```go\nx := 1\n```\n"})

	app.handleCopyCommand("7")
	clip.nothing(t)
	assert.Contains(t, app.messages[len(app.messages)-1].Content, "No code block 7")
}

func TestCopyWithNoSelectionListsTheBlocksToChooseFrom(t *testing.T) {
	app, clip := newSelectingApp(t, 0)
	app.appendMessage(Message{
		Role:    roleAssistant,
		Content: "```go\nfunc main() {}\n```\n\n```sh\nmake\n```\n",
	})

	app.handleCopyCommand("")
	clip.nothing(t)

	last := app.messages[len(app.messages)-1]
	assert.Equal(t, last.Role, roleReport)
	text := tui.SprintScreen(last.View, tui.WithWidth(80)).Text()
	assert.Contains(t, text, "func main() {}")
	assert.Contains(t, text, "/copy 1-2")
}

func TestCopyPrefersTheSelectionOverTheCodeBlocks(t *testing.T) {
	app, clip := newSelectingApp(t, 0)
	app.appendMessage(Message{Role: roleAssistant, Content: "```go\nx := 1\n```\n"})
	app.viewport.ScrollToBottom()

	screen := renderScreen(t, app, 80, 24)
	x, y, width := findText(t, screen, "x := 1")
	press(app, x, y)
	drag(app, x+width, y)
	release(app, x+width, y)
	clip.next(t) // the copy-on-select

	app.handleCopyCommand("")
	assert.Equal(t, clip.next(t), "x := 1", "a standing selection is what /copy means")
}

func TestCopyLooksNoFurtherBackThanTheLastQuestion(t *testing.T) {
	app, clip := newSelectingApp(t, 0)
	app.appendMessage(Message{Role: roleAssistant, Content: "```go\nold := 1\n```\n"})
	app.appendMessage(Message{Role: roleUser, Content: "something else"})

	app.handleCopyCommand("1")
	clip.nothing(t)
	assert.Contains(t, app.messages[len(app.messages)-1].Content, "no code blocks")
}

func TestCopyingNothingSaysSoInsteadOfSendingAnEmptyClipboard(t *testing.T) {
	app, clip := newSelectingApp(t, 2)
	before := len(app.messages)

	app.copyToClipboard("   \n  ")
	clip.nothing(t)
	assert.Contains(t, app.activeFlash(), "Nothing to copy")
	// A flash, so the transcript is untouched and the view does not move.
	assert.Equal(t, len(app.messages), before)
}

func TestAnUnverifiableCopyIsNotDescribedAsADoneDeal(t *testing.T) {
	// Which rung did the work is not the user's problem, so it is not in the
	// line — only whether the copy happened or was merely asked for.
	verified := clipboardReport{lines: 12, via: "pbcopy", verified: true}
	assert.Equal(t, verified.notice(), "Copied 12 lines")

	// OSC 52 draws no reply from the terminal, so there is nothing to confirm.
	sent := clipboardReport{lines: 12, via: "OSC 52", verified: false}
	assert.Equal(t, sent.notice(), "Sent 12 lines to the terminal clipboard — if nothing landed, your terminal may not allow it; /scrollback always works.")
}

func TestForcingOSC52WritesTheSequenceAndSkipsEveryNativeTool(t *testing.T) {
	t.Setenv("DIVE_CLIPBOARD", "osc52")
	// A native tool would win the ladder if the override were not honoured.
	t.Setenv("PATH", "")

	var out strings.Builder
	report, err := newClipboardCopier(&out)("hello")

	assert.NoError(t, err)
	assert.Equal(t, report.via, "OSC 52")
	assert.False(t, report.verified)
	assert.Equal(t, out.String(), "\x1b]52;c;aGVsbG8=\a")
}

func TestTheLadderFallsToOSC52WhenNothingNativeCanWork(t *testing.T) {
	t.Setenv("DIVE_CLIPBOARD", "")
	t.Setenv("PATH", "")
	t.Setenv("TMUX", "")
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")

	var out strings.Builder
	report, err := newClipboardCopier(&out)("over ssh")

	assert.NoError(t, err)
	assert.Equal(t, report.via, "OSC 52", "the rung that crosses an SSH connection")
	assert.Equal(t, report.lines, 1)
	assert.Contains(t, out.String(), "\x1b]52;c;")
}

func TestCountLinesCountsRowsRatherThanNewlines(t *testing.T) {
	assert.Equal(t, countLines(""), 0)
	assert.Equal(t, countLines("one"), 1)
	assert.Equal(t, countLines("one\n"), 1, "a trailing newline does not add a row")
	assert.Equal(t, countLines("one\ntwo"), 2)
	assert.Equal(t, countLines("one\ntwo\n"), 2)
}

func TestTheNativeSelectionModifierFollowsTheTerminal(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	assert.Equal(t, nativeSelectionModifier(), "Option")

	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	assert.Equal(t, nativeSelectionModifier(), "Fn")

	t.Setenv("TERM_PROGRAM", "WezTerm")
	assert.Equal(t, nativeSelectionModifier(), "Shift")

	t.Setenv("TERM_PROGRAM", "")
	assert.Equal(t, nativeSelectionModifier(), "Shift")
}
