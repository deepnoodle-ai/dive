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

func TestConvertMessagesKeepsProviderMetadataOutOfAnthropicWire(t *testing.T) {
	messages := []*llm.Message{{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ThinkingContent{
				Thinking: "Gemini summary",
				Metadata: llm.ProviderMetadata{"google.thought": "true"},
			},
			&llm.TextContent{
				Text:     "answer",
				Metadata: llm.ProviderMetadata{"google.thought_signature": "signature"},
			},
			&llm.ToolUseContent{
				ID:       "call_1",
				Name:     "lookup",
				Input:    []byte(`{}`),
				Metadata: llm.ProviderMetadata{"google.thought_signature": "signature"},
			},
			&llm.ThinkingContent{
				Thinking:  "Anthropic thought",
				Signature: "anthropic-signature",
			},
		},
	}}

	converted, err := convertMessages(messages)
	assert.NoError(t, err)
	assert.Len(t, converted, 1)
	assert.Len(t, converted[0].Content, 3)

	text := converted[0].Content[0].(*llm.TextContent)
	assert.Nil(t, text.Metadata)
	thinking := converted[0].Content[1].(*llm.ThinkingContent)
	assert.Equal(t, "Anthropic thought", thinking.Thinking)
	assert.Nil(t, thinking.Metadata)
	toolUse := converted[0].Content[2].(*llm.ToolUseContent)
	assert.Nil(t, toolUse.Metadata)

	// The durable caller-owned blocks retain their originating provider state.
	assert.NotNil(t, messages[0].Content[0].(*llm.ThinkingContent).Metadata)
	assert.NotNil(t, messages[0].Content[1].(*llm.TextContent).Metadata)
	assert.NotNil(t, messages[0].Content[2].(*llm.ToolUseContent).Metadata)
}
