package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/compaction"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/tui"
)

func TestHandleCompaction(t *testing.T) {
	// Create a mock agent (we won't use it for this test)
	agent := &dive.Agent{}

	app := NewApp(agent, nil, "/tmp/test", "test-model", "", nil, "", nil, "")

	// Create a compaction event
	event := &compaction.CompactionEvent{
		TokensBefore:      100000,
		TokensAfter:       5000,
		Summary:           "Test summary",
		MessagesCompacted: 50,
	}

	// Handle the compaction (between-turns path)
	app.handleCompaction(event, false)

	// Verify state was updated correctly
	assert.NotNil(t, app.lastCompactionEvent, "lastCompactionEvent should be set")
	assert.Equal(t, event, app.lastCompactionEvent, "lastCompactionEvent should match the event")
	assert.True(t, app.showCompactionSummary, "showCompactionSummary should take the turn-summary slot")

	// Verify the event values
	assert.Equal(t, 100000, app.lastCompactionEvent.TokensBefore)
	assert.Equal(t, 5000, app.lastCompactionEvent.TokensAfter)
	assert.Equal(t, 50, app.lastCompactionEvent.MessagesCompacted)

	// Verify timestamps are recent
	assert.True(t, time.Since(app.compactionEventTime) < time.Second,
		"compactionEventTime should be recent")

	// The result lands in the scrollback too.
	assert.Contains(t, app.messages[len(app.messages)-1].Content, "Context compacted")

	// The next turn reclaims the turn-summary slot.
	app.handleProcessingStart(processingStartEvent{baseEvent: newBaseEvent(), userInput: "hi"})
	assert.False(t, app.showCompactionSummary, "a new turn should clear the compaction summary")
}

func TestManualCompactionRunsOffTheEventLoop(t *testing.T) {
	app, fake := newFakeApp(t)

	// Starting compaction flips the flag; the live view shows progress
	// instead of freezing on the last frame.
	app.HandleEvent(compactionStartEvent{baseEvent: newBaseEvent()})
	assert.True(t, app.compacting, "compacting should be true after start event")

	var buf bytes.Buffer
	tui.Fprint(&buf, app.buildLiveView(false), tui.WithWidth(80))
	// The "compacting" label is per-character animated, so ANSI codes sit
	// between its letters; the spinner glyph proves the progress line rendered.
	assert.Contains(t, buf.String(), "⣾")

	// A second /compact while one is running is refused, not queued.
	app.compactionConfig = &compaction.CompactionConfig{}
	before := len(fake.events)
	app.handleCompactCommand()
	assert.True(t, app.compacting, "second compact must not clear the flag")
	assert.Equal(t, before, len(fake.events), "second compact must not start another run")
	assert.Contains(t, app.messages[len(app.messages)-1].Content, "Already compacting")

	// Failure clears the flag and lands in the transcript.
	app.HandleEvent(compactionEndEvent{baseEvent: newBaseEvent(), err: errors.New("boom")})
	assert.False(t, app.compacting, "compacting should be false after end event")
	assert.Contains(t, app.messages[len(app.messages)-1].Content, "Compaction failed")
}

func TestShouldDisplayToolError(t *testing.T) {
	t.Run("non-error remains non-error", func(t *testing.T) {
		assert.False(t, shouldDisplayToolError("AskUserQuestion", false, ""))
	})

	t.Run("non-askuser error remains error", func(t *testing.T) {
		assert.True(t, shouldDisplayToolError("Read", true, `{"ok":true}`))
	})

	t.Run("askuser error stays red (including custom deny feedback)", func(t *testing.T) {
		assert.True(t, shouldDisplayToolError("AskUserQuestion", true, "Ask me another question please."))
		assert.True(t, shouldDisplayToolError("request_user_input", true, "Ask me another question please."))
		assert.True(t, shouldDisplayToolError("AskUserQuestion", true, "No options provided for selection"))
	})
}

func TestFailedReadRendersTheErrorInsteadOfAFalseLineCount(t *testing.T) {
	app := NewApp(&dive.Agent{}, nil, "/tmp/test", "test-model", "", nil, "", nil, "")
	app.messages = append(app.messages, Message{
		Type:     MessageTypeToolCall,
		ToolID:   "read-1",
		ToolName: "Read",
	})
	app.toolCallIndex["read-1"] = 0
	app.handleToolResult(&dive.ToolCallResult{
		ID:     "read-1",
		Name:   "Read",
		Result: dive.NewToolResultError("file does not exist"),
	})

	msg := app.messages[0]
	assert.True(t, msg.ToolError)
	assert.Equal(t, 0, msg.ToolReadLines)
	var output bytes.Buffer
	tui.Fprint(&output, app.formatToolResultView(msg, viewOpts{}), tui.WithWidth(80))
	assert.Contains(t, output.String(), "file does not exist")
	assert.NotContains(t, output.String(), "Read 1 line")
}

func TestSuccessfulReadUsesCorrectSingularLineCount(t *testing.T) {
	app := NewApp(&dive.Agent{}, nil, "/tmp/test", "test-model", "", nil, "", nil, "")
	var output bytes.Buffer
	tui.Fprint(&output, app.formatToolResultView(Message{
		Type:          MessageTypeToolCall,
		ToolName:      "Read",
		ToolReadLines: 1,
	}, viewOpts{}), tui.WithWidth(80))
	assert.Contains(t, output.String(), "Read 1 line")
	assert.NotContains(t, output.String(), "Read 1 lines")
}

func TestContextDemoTraceAndReportExposeExactPayloads(t *testing.T) {
	app, fake := newFakeApp(t)
	app.contextDemos = allContextDemos()
	reminder, err := dive.NewOperatorReminder("verification-debt", "Run the relevant checks before completion.")
	assert.NoError(t, err)
	app.handleContextDemoNotice(contextDemoNotice{
		Reminder: reminder,
		Delivery: contextDemoModelOnly,
		Action:   "queued",
	})
	app.printContextDemoReport()

	rendered := fake.scrollback(180)
	assert.Contains(t, rendered, "verification-debt queued")
	assert.Contains(t, rendered, "operator")
	assert.Contains(t, rendered, "model-only")
	assert.Contains(t, rendered, "Run the relevant checks before completion.")
	assert.Contains(t, rendered, "not saved to conversation history")
	assert.NotContains(t, rendered, "<system-reminder")
}

func TestHandleStreamThinkingCreatesReasoningMessage(t *testing.T) {
	app, _ := newFakeApp(t)
	app.handleProcessingStart(processingStartEvent{baseEvent: newBaseEvent(), userInput: "explain this"})

	app.handleStreamThinking("I should compare the code paths.")
	app.flushThinkingStreamBuffer()
	app.handleStreamText("The code paths differ here.")
	app.flushStreamBuffer()

	reasoningIdx := -1
	answerIdx := -1
	for i, msg := range app.messages {
		switch msg.Role {
		case "reasoning":
			if strings.Contains(msg.Content, "compare the code paths") {
				reasoningIdx = i
			}
		case "assistant":
			if strings.Contains(msg.Content, "code paths differ") {
				answerIdx = i
			}
		}
	}
	assert.True(t, reasoningIdx >= 0, "expected reasoning message")
	assert.True(t, answerIdx >= 0, "expected assistant answer message")
	assert.True(t, reasoningIdx < answerIdx, "reasoning should render before answer")
}

func TestConvertLLMMessageShowsThinkingContent(t *testing.T) {
	app, _ := newFakeApp(t)
	msgs := app.convertLLMMessage(&llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ThinkingContent{Thinking: "I considered the API contract."},
			&llm.TextContent{Text: "The final answer."},
		},
	}, nil)

	assert.Equal(t, 2, len(msgs))
	assert.Equal(t, roleReasoning, msgs[0].Role)
	assert.Equal(t, roleAssistant, msgs[1].Role)

	var views []tui.View
	for _, m := range msgs {
		views = append(views, app.messageView(m, viewOpts{}))
	}
	var buf bytes.Buffer
	tui.Fprint(&buf, tui.Stack(views...), tui.WithWidth(80))
	out := buf.String()
	assert.Contains(t, out, "I considered the API contract.")
	assert.Contains(t, out, "The final answer.")
}

// A resumed session has to look like the one it resumed. A Read renders as
// "Read N lines" while it is live, so replay has to count the lines too rather
// than falling back to dumping the file's first line.
func TestReplayingAReadCollapsesItTheWayTheLiveOneDid(t *testing.T) {
	app, _ := newFakeApp(t)
	call := &llm.ToolUseContent{ID: "call-1", Name: "Read", Input: []byte(`{"file_path":"a.go"}`)}
	results := map[string]*llm.ToolResultContent{
		"call-1": {ToolUseID: "call-1", Content: "package main\n\nfunc main() {}"},
	}

	msgs := app.convertLLMMessage(&llm.Message{
		Role: llm.Assistant, Content: []llm.Content{call},
	}, results)

	assert.Equal(t, len(msgs), 1)
	assert.Equal(t, msgs[0].ToolReadLines, 3)

	var buf bytes.Buffer
	tui.Fprint(&buf, app.formatToolResultView(msgs[0], viewOpts{}), tui.WithWidth(80))
	assert.Contains(t, buf.String(), "Read 3 lines")
	assert.NotContains(t, buf.String(), "package main", "the file body is not the summary")
}

// An errored Read has something to say, so it keeps its message rather than
// being collapsed into a line count of the error text.
func TestReplayingAFailedReadKeepsTheError(t *testing.T) {
	app, _ := newFakeApp(t)
	call := &llm.ToolUseContent{ID: "call-1", Name: "Read", Input: []byte(`{"file_path":"nope.go"}`)}
	results := map[string]*llm.ToolResultContent{
		"call-1": {ToolUseID: "call-1", IsError: true, Content: "no such file: nope.go"},
	}

	msgs := app.convertLLMMessage(&llm.Message{
		Role: llm.Assistant, Content: []llm.Content{call},
	}, results)

	assert.Equal(t, msgs[0].ToolReadLines, 0)
	assert.Equal(t, msgs[0].ToolResult, "no such file: nope.go")
}

func TestMessageThinkingText(t *testing.T) {
	text := messageThinkingText(&llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ThinkingContent{Thinking: " first thought "},
			&llm.TextContent{Text: "answer"},
			&llm.ThinkingContent{Thinking: "second thought"},
		},
	})

	assert.Equal(t, "first thought\n\nsecond thought", text)
}
