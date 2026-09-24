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
	assert.Equal(t, `[{"output":"line one\n\nline two","call_id":"call_1","type":"function_call_output"}]`, string(data))
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

// TestEncodeToolResultErrorWithImage verifies an error result still shows the
// model its image: the output is a content-part list whose leading text item
// carries the "Error: " signal the string form would have.
func TestEncodeToolResultErrorWithImage(t *testing.T) {
	tests := []struct {
		name   string
		blocks []*dive.ToolResultContent
		want   string
	}{
		{
			name: "text then image",
			blocks: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "boom"},
				{Type: dive.ToolResultContentTypeImage, Data: "aW1n", MimeType: "image/png"},
			},
			want: `[{"output":[{"text":"Error: boom","type":"input_text"},{"image_url":"data:image/png;base64,aW1n","type":"input_image"}],"call_id":"call_1","type":"function_call_output"}]`,
		},
		{
			name: "image only",
			blocks: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeImage, Data: "aW1n", MimeType: "image/png"},
			},
			want: `[{"output":[{"text":"Error:","type":"input_text"},{"image_url":"data:image/png;base64,aW1n","type":"input_image"}],"call_id":"call_1","type":"function_call_output"}]`,
		},
		{
			name: "already prefixed",
			blocks: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "Error: boom"},
				{Type: dive.ToolResultContentTypeImage, Data: "aW1n", MimeType: "image/png"},
			},
			want: `[{"output":[{"text":"Error: boom","type":"input_text"},{"image_url":"data:image/png;base64,aW1n","type":"input_image"}],"call_id":"call_1","type":"function_call_output"}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := llm.NewToolResultMessage(&llm.ToolResultContent{
				ToolUseID: "call_1",
				IsError:   true,
				Content:   tt.blocks,
			})
			// The same output after a JSON round trip (session replay).
			body, err := json.Marshal(message)
			assert.NoError(t, err)
			var replayed llm.Message
			assert.NoError(t, json.Unmarshal(body, &replayed))

			for _, m := range []*llm.Message{message, &replayed} {
				items, err := encodeMessages([]*llm.Message{m})
				assert.NoError(t, err)
				data, err := json.Marshal(items)
				assert.NoError(t, err)
				assert.Equal(t, tt.want, string(data))
			}
		})
	}
}

// TestEncodeToolResultExplicitErrorEnvelopePreserved verifies self-describing
// error output is not changed on its way to a Responses API model.
func TestEncodeToolResultExplicitErrorEnvelopePreserved(t *testing.T) {
	const output = "<error>Exit code 3\nhello stdout\nhello stderr</error>"
	items, err := encodeMessages([]*llm.Message{
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: "call_1",
			IsError:   true,
			Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: output},
			},
		}),
	})
	assert.NoError(t, err)
	data, err := json.Marshal(items)
	assert.NoError(t, err)
	var decoded []map[string]any
	assert.NoError(t, json.Unmarshal(data, &decoded))
	assert.Len(t, decoded, 1)
	assert.Equal(t, output, decoded[0]["output"])
}

// TestEncodeToolResultEmptyOutput verifies a tool result with nothing to
// render produces an explicit placeholder rather than an empty output string.
func TestEncodeToolResultEmptyOutput(t *testing.T) {
	tests := []struct {
		name    string
		content any
	}{
		{"single empty text block", []*dive.ToolResultContent{{Type: dive.ToolResultContentTypeText, Text: ""}}},
		{"no blocks at all", []*dive.ToolResultContent{}},
		{"no blocks after JSON round trip", []any{}},
		{"nil content", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := encodeMessages([]*llm.Message{
				llm.NewToolResultMessage(&llm.ToolResultContent{
					ToolUseID: "call_1",
					Content:   tt.content,
				}),
			})
			assert.NoError(t, err)
			data, err := json.Marshal(items)
			assert.NoError(t, err)
			assert.Equal(t, `[{"output":"(no output)","call_id":"call_1","type":"function_call_output"}]`, string(data))
		})
	}
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
	assert.Equal(t, `[{"output":"replayed","call_id":"call_1","type":"function_call_output"}]`, string(data))
}
