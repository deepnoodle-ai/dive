package llm

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func toolUse(ids ...string) *Message {
	msg := &Message{Role: Assistant}
	for _, id := range ids {
		msg.Content = append(msg.Content, &ToolUseContent{ID: id, Name: "t", Input: []byte(`{}`)})
	}
	return msg
}

func resultIDs(msg *Message) []string {
	var ids []string
	for _, c := range msg.Content {
		if r, ok := c.(*ToolResultContent); ok {
			ids = append(ids, r.ToolUseID)
		}
	}
	return ids
}

func TestAnswerUnansweredToolCallsNothingMissing(t *testing.T) {
	msgs := []*Message{
		NewUserTextMessage("hi"),
		toolUse("a"),
		NewToolResultMessage(&ToolResultContent{ToolUseID: "a", Content: "ok"}),
		NewAssistantTextMessage("done"),
	}
	out := AnswerUnansweredToolCalls(msgs)
	assert.Equal(t, len(out), len(msgs))
	for i := range msgs {
		assert.True(t, out[i] == msgs[i])
	}
}

func TestAnswerUnansweredToolCallsMiddleAndTail(t *testing.T) {
	partial := NewToolResultMessage(&ToolResultContent{ToolUseID: "a", Content: "ok"})
	partial.Content = append(partial.Content, &TextContent{Text: "note"})
	msgs := []*Message{
		NewUserTextMessage("hi"),
		toolUse("a", "b"),
		partial,
		NewAssistantTextMessage("thinking"),
		toolUse("c"),
	}
	out := AnswerUnansweredToolCalls(msgs)

	// The partial result message gains b after a, before the text.
	assert.Equal(t, len(out), 6)
	assert.Equal(t, resultIDs(out[2]), []string{"a", "b"})
	assert.Equal(t, out[2].Content[2].(*TextContent).Text, "note")
	b := out[2].Content[1].(*ToolResultContent)
	assert.True(t, b.IsError)
	assert.Equal(t, b.Content, ToolCallUnknownText)

	// The trailing call gets a new tool-result message.
	assert.Equal(t, out[5].Role, User)
	assert.Equal(t, resultIDs(out[5]), []string{"c"})

	// The caller's messages are unchanged.
	assert.Equal(t, len(msgs), 5)
	assert.Equal(t, resultIDs(partial), []string{"a"})
	assert.Equal(t, len(partial.Content), 2)
}

func TestAnswerUnansweredToolCallsBeforeAssistantMessage(t *testing.T) {
	msgs := []*Message{
		NewUserTextMessage("hi"),
		toolUse("a"),
		NewAssistantTextMessage("answer"),
	}
	out := AnswerUnansweredToolCalls(msgs)
	assert.Equal(t, len(out), 4)
	assert.Equal(t, out[2].Role, User)
	assert.Equal(t, resultIDs(out[2]), []string{"a"})
	assert.True(t, out[3] == msgs[2])
}

func TestAnswerUnansweredToolCallsIgnoresServerToolCalls(t *testing.T) {
	msgs := []*Message{
		NewUserTextMessage("search"),
		{Role: Assistant, Content: []Content{&ServerToolUseContent{ID: "srv", Name: "web_search"}}},
	}
	out := AnswerUnansweredToolCalls(msgs)
	assert.Equal(t, len(out), 2)
	assert.True(t, out[1] == msgs[1])
}

func TestAnswerUnansweredToolCallsKeepsToolsetName(t *testing.T) {
	call := &Message{Role: Assistant, Content: []Content{
		&ToolUseContent{ID: "a", Name: "click", ToolsetName: "computer", Input: []byte(`{}`)},
	}}
	out := AnswerUnansweredToolCalls([]*Message{NewUserTextMessage("go"), call})
	assert.Equal(t, out[2].Content[0].(*ToolResultContent).ToolsetName, "computer")
}
