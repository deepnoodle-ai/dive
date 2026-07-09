package main

import (
	"bytes"
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
	assert.True(t, app.showCompactionStats, "showCompactionStats should be true")

	// Verify the event values
	assert.Equal(t, 100000, app.lastCompactionEvent.TokensBefore)
	assert.Equal(t, 5000, app.lastCompactionEvent.TokensAfter)
	assert.Equal(t, 50, app.lastCompactionEvent.MessagesCompacted)

	// Verify timestamps are recent
	assert.True(t, time.Since(app.compactionEventTime) < time.Second,
		"compactionEventTime should be recent")
	assert.True(t, time.Since(app.compactionStatsStartTime) < time.Second,
		"compactionStatsStartTime should be recent")
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

func TestHandleStreamThinkingCreatesReasoningMessage(t *testing.T) {
	app := NewApp(&dive.Agent{}, nil, "/tmp/test", "test-model", "", nil, "", nil, "")
	var buf bytes.Buffer
	app.runner = tui.NewInlineApp(
		tui.WithInlineWidth(80),
		tui.WithInlineOutput(&buf),
	)
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

func TestConvertLLMMessageToViewsShowsThinkingContent(t *testing.T) {
	app := NewApp(&dive.Agent{}, nil, "/tmp/test", "test-model", "", nil, "", nil, "")
	views := app.convertLLMMessageToViews(&llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ThinkingContent{Thinking: "I considered the API contract."},
			&llm.TextContent{Text: "The final answer."},
		},
	}, nil)

	var buf bytes.Buffer
	tui.Fprint(&buf, tui.Stack(views...), tui.WithWidth(80))
	out := buf.String()
	assert.Contains(t, out, "I considered the API contract.")
	assert.Contains(t, out, "The final answer.")
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
