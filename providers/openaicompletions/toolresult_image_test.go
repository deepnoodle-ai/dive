package openaicompletions

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// Chat Completions tool messages are text-only, so a tool-result image is
// sent in a user message right after the run of tool messages, with a pointer
// in the tool message and a label naming the call before each image.
func TestToolResultImagesFollowToolMessages(t *testing.T) {
	history := []*llm.Message{
		llm.NewUserTextMessage("Compare the two screens."),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_1", Name: "screenshot", Input: []byte(`{}`)},
			&llm.ToolUseContent{ID: "call_2", Name: "screenshot", Input: []byte(`{}`)},
		}},
		{Role: llm.User, Content: []llm.Content{
			&llm.ToolResultContent{ToolUseID: "call_1", Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "left screen"},
				{Type: dive.ToolResultContentTypeImage, Data: "aW1nMQ==", MimeType: "image/png"},
			}},
			&llm.ToolResultContent{ToolUseID: "call_2", IsError: true, Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeImage, Data: "aW1nMg==", MimeType: "image/jpeg"},
			}},
		}},
	}
	body, err := json.Marshal(history)
	assert.NoError(t, err)
	var replayed []*llm.Message
	assert.NoError(t, json.Unmarshal(body, &replayed))

	for name, in := range map[string][]*llm.Message{"typed": history, "round-tripped": replayed} {
		t.Run(name, func(t *testing.T) {
			converted, err := convertMessages(in)
			assert.NoError(t, err)
			assert.Len(t, converted, 5)
			assert.Equal(t, "user", converted[0].Role)
			assert.Len(t, converted[1].ToolCalls, 2)

			assert.Equal(t, "tool", converted[2].Role)
			assert.Equal(t, "call_1", converted[2].ToolCallID)
			assert.Equal(t, "left screen\n(The image this call returned follows the tool results.)", converted[2].Content)
			assert.Equal(t, "tool", converted[3].Role)
			assert.Equal(t, "call_2", converted[3].ToolCallID)
			assert.Equal(t, "(The image this call returned follows the tool results.)", converted[3].Content)

			assert.Equal(t, "user", converted[4].Role)
			assert.Equal(t, []ContentPart{
				{Type: "text", Text: "The image from tool call call_1:"},
				{Type: "image_url", ImageURL: &ImageURLPart{URL: "data:image/png;base64,aW1nMQ=="}},
				{Type: "text", Text: "The image from tool call call_2:"},
				{Type: "image_url", ImageURL: &ImageURLPart{URL: "data:image/jpeg;base64,aW1nMg=="}},
			}, converted[4].ContentParts)
		})
	}

	// The stored history still holds each image in its tool result, where
	// a provider that takes tool-result images natively finds it.
	blocks := history[2].Content[0].(*llm.ToolResultContent).Content.([]*dive.ToolResultContent)
	assert.Len(t, blocks, 2)
	assert.Equal(t, dive.ToolResultContentTypeImage, blocks[1].Type)
}

// Other content riding with the tool results (a reminder, a hook's additional
// context) stays ahead of the lifted images in the same user message.
func TestToolResultImagesFollowAuxiliaryText(t *testing.T) {
	converted, err := convertMessages([]*llm.Message{
		{Role: llm.User, Content: []llm.Content{
			&llm.ToolResultContent{ToolUseID: "call_1", Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeImage, Data: "iVBORw0KGgo="},
			}},
			&llm.TextContent{Text: "Respond with a final answer now."},
		}},
	})
	assert.NoError(t, err)
	assert.Len(t, converted, 2)
	assert.Equal(t, "tool", converted[0].Role)
	assert.Equal(t, []ContentPart{
		{Type: "text", Text: "Respond with a final answer now."},
		{Type: "text", Text: "The image from tool call call_1:"},
		{Type: "image_url", ImageURL: &ImageURLPart{URL: "data:image/png;base64,iVBORw0KGgo="}},
	}, converted[1].ContentParts)
}
