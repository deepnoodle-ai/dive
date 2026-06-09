package anthropic

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestConvertMessagesDoesNotMutateCaller(t *testing.T) {
	// An assistant message with a tool_use block before a text block triggers
	// the reorder workaround. The reorder must apply to the copies only, not
	// the caller's messages.
	toolUse := &llm.ToolUseContent{
		ID:    "toolu_1",
		Name:  "calculator",
		Input: []byte(`{"expression":"1+1"}`),
	}
	text := &llm.TextContent{Text: "Let me calculate that."}
	messages := []*llm.Message{
		llm.NewUserTextMessage("What is 1+1?"),
		{
			Role:    llm.Assistant,
			Content: []llm.Content{toolUse, text},
		},
	}

	copied, err := convertMessages(messages)
	assert.NoError(t, err)

	// Caller's message content order is unchanged
	assert.Len(t, messages[1].Content, 2)
	assert.Equal(t, messages[1].Content[0].Type(), llm.ContentTypeToolUse)
	assert.Equal(t, messages[1].Content[1].Type(), llm.ContentTypeText)

	// The copies have the reordered content (text before tool_use)
	assert.Len(t, copied[1].Content, 2)
	assert.Equal(t, copied[1].Content[0].Type(), llm.ContentTypeText)
	assert.Equal(t, copied[1].Content[1].Type(), llm.ContentTypeToolUse)
}
