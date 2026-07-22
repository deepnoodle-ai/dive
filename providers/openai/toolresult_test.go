package openai

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestEncodeToolResultTextBlocksFlattened verifies typed tool result blocks
// are flattened to plain text rather than JSON-marshaled into the output
// string.
func TestEncodeToolResultTextBlocksFlattened(t *testing.T) {
	items, err := encodeMessages([]*llm.Message{
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: "call_1",
			Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "line one"},
				{Type: dive.ToolResultContentTypeText, Text: "line two"},
			},
		}),
	})
	assert.NoError(t, err)
	data, err := json.Marshal(items)
	assert.NoError(t, err)
	assert.Equal(t, `[{"call_id":"call_1","output":"line one\n\nline two","type":"function_call_output"}]`, string(data))
}

// TestEncodeToolResultWithImageBlocks verifies a tool result carrying an
// image is emitted as a content-part list so the model can see the image.
func TestEncodeToolResultWithImageBlocks(t *testing.T) {
	items, err := encodeMessages([]*llm.Message{
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: "call_1",
			Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "captured screenshot"},
				{Type: dive.ToolResultContentTypeImage, Data: "aW1nZGF0YQ==", MimeType: "image/png"},
			},
		}),
	})
	assert.NoError(t, err)
	data, err := json.Marshal(items)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"type":"input_text"`)
	assert.Contains(t, string(data), `"text":"captured screenshot"`)
	assert.Contains(t, string(data), `"type":"input_image"`)
	assert.Contains(t, string(data), `"image_url":"data:image/png;base64,aW1nZGF0YQ=="`)
}

// TestEncodeToolResultErrorKeepsTextForm verifies error results keep the
// string output form with the "Error: " prefix, even when images are present.
func TestEncodeToolResultErrorKeepsTextForm(t *testing.T) {
	items, err := encodeMessages([]*llm.Message{
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: "call_1",
			IsError:   true,
			Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "boom"},
				{Type: dive.ToolResultContentTypeImage, Data: "aW1n", MimeType: "image/png"},
			},
		}),
	})
	assert.NoError(t, err)
	data, err := json.Marshal(items)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"output":"Error: boom\n\n[image content omitted]"`)
}

// TestEncodeToolResultBlocksSurviveJSONRoundTrip verifies session-replayed
// tool results (blocks arriving as []any) get the same flattening.
func TestEncodeToolResultBlocksSurviveJSONRoundTrip(t *testing.T) {
	original := llm.NewToolResultMessage(&llm.ToolResultContent{
		ToolUseID: "call_1",
		Content: []*dive.ToolResultContent{
			{Type: dive.ToolResultContentTypeText, Text: "replayed"},
		},
	})
	body, err := json.Marshal(original)
	assert.NoError(t, err)
	var replayed llm.Message
	assert.NoError(t, json.Unmarshal(body, &replayed))

	items, err := encodeMessages([]*llm.Message{&replayed})
	assert.NoError(t, err)
	data, err := json.Marshal(items)
	assert.NoError(t, err)
	assert.Equal(t, `[{"call_id":"call_1","output":"replayed","type":"function_call_output"}]`, string(data))
}
