package main

import (
	"bytes"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/tui"
)

// fakeRunner records everything the app sends the UI runner, so tests can drive
// App without a terminal. It satisfies both uiRunner and scrollbackWriter.
type fakeRunner struct {
	events  []tui.Event
	printed []tui.View
	stops   int
	clears  int
}

func (f *fakeRunner) SendEvent(e tui.Event) { f.events = append(f.events, e) }
func (f *fakeRunner) Stop()                 { f.stops++ }
func (f *fakeRunner) Print(v tui.View)      { f.printed = append(f.printed, v) }
func (f *fakeRunner) ClearScrollback()      { f.clears++; f.printed = nil }

// scrollback renders everything printed so far as plain text.
func (f *fakeRunner) scrollback(width int) string {
	var buf bytes.Buffer
	for _, v := range f.printed {
		tui.Fprint(&buf, v, tui.WithWidth(width))
	}
	return buf.String()
}

// newFakeApp returns an app wired to a recording runner.
func newFakeApp(t *testing.T) (*App, *fakeRunner) {
	t.Helper()
	app := NewApp(&dive.Agent{}, nil, "/tmp/test", "test-model", "", nil, "", nil, "")
	fake := &fakeRunner{}
	app.runner = fake
	app.scrollback = fake
	return app, fake
}

func TestAppendedMessagesReachScrollbackExactlyOnce(t *testing.T) {
	app, fake := newFakeApp(t)

	app.appendNotice("Did not attach %s: too large.", "clip.mov")
	assert.Equal(t, 1, len(app.messages))
	assert.True(t, app.messages[0].emitted)
	assert.Contains(t, fake.scrollback(80), "Did not attach clip.mov")

	// A second flush must not reprint what has already been emitted.
	before := len(fake.printed)
	app.emitPending()
	assert.Equal(t, before, len(fake.printed))
}

func TestTurnMessagesEmitAtTurnEnd(t *testing.T) {
	app, fake := newFakeApp(t)

	app.handleProcessingStart(processingStartEvent{baseEvent: newBaseEvent(), userInput: "explain this"})
	assert.Contains(t, fake.scrollback(80), "explain this")

	app.handleStreamText("Here is the answer.")
	app.flushStreamBuffer()

	// Mid-turn the reply is still only app state.
	assert.NotContains(t, fake.scrollback(80), "Here is the answer")

	app.handleProcessingEnd(nil)
	assert.Contains(t, fake.scrollback(80), "Here is the answer")

	for i, msg := range app.messages {
		assert.True(t, msg.emitted, "message %d (%s) should be emitted after the turn", i, msg.Role)
	}
}

func TestMidTurnNoticeDoesNotFlushTheStreamingReply(t *testing.T) {
	app, fake := newFakeApp(t)

	app.handleProcessingStart(processingStartEvent{baseEvent: newBaseEvent(), userInput: "go"})
	app.handleStreamText("half a sentence")
	app.flushStreamBuffer()

	app.appendNotice("Context compacted mid-turn.")

	out := fake.scrollback(80)
	assert.Contains(t, out, "Context compacted mid-turn")
	assert.NotContains(t, out, "half a sentence")
}

func TestTouchBumpsRevAtInPlaceMutationSites(t *testing.T) {
	app, _ := newFakeApp(t)

	app.handleProcessingStart(processingStartEvent{baseEvent: newBaseEvent(), userInput: "go"})
	idx := app.streamingMessageIndex

	app.handleStreamText("one")
	app.flushStreamBuffer()
	assert.Equal(t, uint32(1), app.messages[idx].Rev)

	app.handleStreamText(" two")
	app.flushStreamBuffer()
	assert.Equal(t, uint32(2), app.messages[idx].Rev)

	// An append is not a mutation: the new message starts at Rev 0.
	app.handleToolCall(toolUse("t1", "Bash", `{"command":"ls"}`))
	toolIdx := app.toolCallIndex["t1"]
	assert.Equal(t, uint32(0), app.messages[toolIdx].Rev)

	app.handleToolProgress(toolProgressEvent{baseEvent: newBaseEvent(), toolCallID: "t1", display: "running"})
	assert.Equal(t, uint32(1), app.messages[toolIdx].Rev)

	app.handleToolStream(toolStreamEvent{baseEvent: newBaseEvent(), toolCallID: "t1", text: "a.go\n"})
	assert.Equal(t, uint32(2), app.messages[toolIdx].Rev)
}

func TestClearResetsTranscriptAndShowsAFreshIntro(t *testing.T) {
	app, fake := newFakeApp(t)
	app.appendNotice("something earlier")

	app.handleCommand("/clear", nil)

	assert.Equal(t, 1, fake.clears)
	assert.Equal(t, 1, len(app.messages))
	assert.Equal(t, roleIntro, app.messages[0].Role)
	out := fake.scrollback(80)
	assert.NotContains(t, out, "something earlier")
	assert.Contains(t, out, "Dive")
}

func TestReportsAreStoredAsMessagesNotJustPrinted(t *testing.T) {
	app, fake := newFakeApp(t)

	app.printHelp()

	assert.Equal(t, 1, len(app.messages))
	assert.Equal(t, roleReport, app.messages[0].Role)
	assert.NotNil(t, app.messages[0].View)
	assert.Contains(t, fake.scrollback(80), "Built-in Commands")

	// A report re-renders at whatever width it is later given.
	var narrow bytes.Buffer
	tui.Fprint(&narrow, app.messageView(app.messages[0], viewOpts{}), tui.WithWidth(40))
	assert.Contains(t, narrow.String(), "Built-in Commands")
}

func TestStatusLineDoesNotShellOut(t *testing.T) {
	app, _ := newFakeApp(t)
	app.gitBranch = "docs/cli-managed-screen"

	var buf bytes.Buffer
	tui.Fprint(&buf, app.statusLineView(), tui.WithWidth(120))
	assert.Contains(t, buf.String(), "docs/cli-managed-screen")

	// The cached value is what renders; nothing re-reads it per frame.
	app.gitBranch = "other"
	buf.Reset()
	tui.Fprint(&buf, app.statusLineView(), tui.WithWidth(120))
	assert.Contains(t, buf.String(), "other")
}

func TestToolCallRendersStaticMarkerWhenNotAnimating(t *testing.T) {
	app, _ := newFakeApp(t)
	msg := Message{
		Type:      MessageTypeToolCall,
		ToolName:  "Bash",
		ToolTitle: "Bash",
		ToolInput: `{"command":"ls"}`,
	}

	var live, static bytes.Buffer
	tui.Fprint(&live, app.toolCallView(msg, viewOpts{animate: true}), tui.WithWidth(80))
	tui.Fprint(&static, app.toolCallView(msg, viewOpts{}), tui.WithWidth(80))

	assert.NotEqual(t, live.String(), static.String(), "a running call should only pulse on live frames")
	assert.Contains(t, static.String(), "Bash(ls)")
}

func TestExpandedToolResultShowsEveryLine(t *testing.T) {
	app, _ := newFakeApp(t)
	msg := Message{
		Type:            MessageTypeToolCall,
		ToolName:        "Grep",
		ToolResultLines: []string{"one", "two", "three"},
	}

	var collapsed, expanded bytes.Buffer
	tui.Fprint(&collapsed, app.formatToolResultView(msg, viewOpts{}), tui.WithWidth(80))
	tui.Fprint(&expanded, app.formatToolResultView(msg, viewOpts{expanded: true}), tui.WithWidth(80))

	assert.Contains(t, collapsed.String(), "… +2 lines")
	assert.NotContains(t, collapsed.String(), "three")
	assert.Contains(t, expanded.String(), "three")
}

// BenchmarkView200 is the pre-migration frame-cost number: one LiveView build
// plus a full render of a 200-message transcript at 120 columns.
func BenchmarkView200(b *testing.B) {
	app := NewApp(&dive.Agent{}, nil, "/tmp/test", "test-model", "", nil, "", nil, "")
	app.runner = &fakeRunner{}
	app.gitBranch = "main"
	for i := 0; i < 100; i++ {
		app.appendMessage(Message{Role: roleUser, Content: "please look at the thing"})
		app.appendMessage(Message{
			Role:    roleAssistant,
			Content: "Here is a **paragraph** of prose with `code` in it.\n\n- one\n- two\n",
		})
	}

	views := make([]tui.View, 0, len(app.messages)+1)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		views = views[:0]
		for _, msg := range app.messages {
			if v := app.messageView(msg, viewOpts{}); v != nil {
				views = append(views, v)
			}
		}
		views = append(views, app.LiveView())
		tui.Fprint(discard{}, tui.Stack(views...).Gap(1), tui.WithWidth(120))
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func toolUse(id, name, input string) *llm.ToolUseContent {
	return &llm.ToolUseContent{ID: id, Name: name, Input: []byte(input)}
}
