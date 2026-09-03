package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestExitTranscriptHoldsTheWholeConversation(t *testing.T) {
	app := newScreenApp(t, 3)

	dump := app.exitTranscript(80)

	for i := range 3 {
		assert.Contains(t, dump, fmt.Sprintf("question %d", i))
		assert.Contains(t, dump, fmt.Sprintf("answer %d", i))
	}
}

func TestExitTranscriptKeepsTheTailAndSaysWhatItDropped(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true
	for i := range exitTranscriptMaxLines {
		app.appendMessage(Message{Role: roleNotice, Content: fmt.Sprintf("line %d", i)})
	}

	dump := app.exitTranscript(80)
	lines := strings.Split(strings.TrimRight(dump, "\n"), "\n")

	assert.Equal(t, len(lines), exitTranscriptMaxLines+1, "the cap plus the note above it")
	assert.Contains(t, lines[0], "earlier lines omitted")
	assert.Contains(t, dump, fmt.Sprintf("line %d", exitTranscriptMaxLines-1),
		"the end of the conversation is the part worth keeping")
	assert.NotContains(t, dump, "line 0\n")
}

func TestAnEmptyTranscriptWritesNothingAtAll(t *testing.T) {
	app, _ := newFakeApp(t)
	app.screenMode = true

	var out strings.Builder
	app.printExitTranscript(&out)

	assert.Equal(t, out.String(), "", "no conversation, no dump")
}

func TestTheResumeLineNamesTheSession(t *testing.T) {
	app := newScreenApp(t, 1)
	sess, err := session.NewMemoryStore().Open(t.Context(), "abc123")
	assert.NoError(t, err)
	app.currentSession = sess

	var out strings.Builder
	app.printResumeLine(&out)

	assert.Contains(t, out.String(), "Session abc123 saved")
	assert.Contains(t, out.String(), "dive --resume abc123")
}

func TestTheResumeLineIsSkippedWithNoSession(t *testing.T) {
	app := newScreenApp(t, 1)
	app.currentSession = nil

	var out strings.Builder
	app.printResumeLine(&out)

	assert.Equal(t, out.String(), "")
}
