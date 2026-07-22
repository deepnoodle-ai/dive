package openaicompletions

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestMessageMarshalStringContent verifies text-only messages keep the exact
// wire shape they had before content-part support was added.
func TestMessageMarshalStringContent(t *testing.T) {
	data, err := json.Marshal(Message{Role: "user", Content: "hi"})
	assert.NoError(t, err)
	assert.Equal(t, `{"role":"user","content":"hi"}`, string(data))

	data, err = json.Marshal(Message{
		Role:       "tool",
		Content:    "4",
		ToolCallID: "call_123",
	})
	assert.NoError(t, err)
	assert.Equal(t, `{"role":"tool","content":"4","tool_call_id":"call_123"}`, string(data))
}

func TestMessageMarshalContentParts(t *testing.T) {
	data, err := json.Marshal(Message{
		Role: "user",
		ContentParts: []ContentPart{
			{Type: "text", Text: "What is in this image?"},
			{Type: "image_url", ImageURL: &ImageURLPart{URL: "data:image/png;base64,aW1n"}},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, `{"role":"user","content":[{"type":"text","text":"What is in this image?"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1n"}}]}`, string(data))
}

func TestMessageUnmarshalContentShapes(t *testing.T) {
	// String content (the usual response shape)
	var m Message
	assert.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":"hello"}`), &m))
	assert.Equal(t, "assistant", m.Role)
	assert.Equal(t, "hello", m.Content)
	assert.Len(t, m.ContentParts, 0)

	// Null content with tool calls
	var m2 Message
	assert.NoError(t, json.Unmarshal([]byte(`{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"f","arguments":"{}"}}]}`), &m2))
	assert.Equal(t, "", m2.Content)
	assert.Len(t, m2.ToolCalls, 1)

	// Content-part array round-trips
	var m3 Message
	assert.NoError(t, json.Unmarshal([]byte(`{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}`), &m3))
	assert.Len(t, m3.ContentParts, 2)
	assert.Equal(t, "hi", m3.ContentParts[0].Text)
	assert.Equal(t, "https://example.com/a.png", m3.ContentParts[1].ImageURL.URL)
}

func TestConvertMessagesImageBase64(t *testing.T) {
	converted, err := convertMessages([]*llm.Message{
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.TextContent{Text: "What is in this image?"},
				&llm.ImageContent{
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeBase64,
						MediaType: "image/png",
						Data:      "aW1nZGF0YQ==",
					},
				},
			},
		},
	})
	assert.NoError(t, err)
	assert.Len(t, converted, 1)
	assert.Equal(t, "user", converted[0].Role)
	assert.Len(t, converted[0].ContentParts, 2)
	assert.Equal(t, "text", converted[0].ContentParts[0].Type)
	assert.Equal(t, "What is in this image?", converted[0].ContentParts[0].Text)
	assert.Equal(t, "image_url", converted[0].ContentParts[1].Type)
	assert.Equal(t, "data:image/png;base64,aW1nZGF0YQ==", converted[0].ContentParts[1].ImageURL.URL)
}

func TestConvertMessagesImageURL(t *testing.T) {
	converted, err := convertMessages([]*llm.Message{
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.ImageContent{
					Source: &llm.ContentSource{
						Type: llm.ContentSourceTypeURL,
						URL:  "https://example.com/photo.jpg",
					},
				},
			},
		},
	})
	assert.NoError(t, err)
	assert.Len(t, converted, 1)
	assert.Len(t, converted[0].ContentParts, 1)
	assert.Equal(t, "https://example.com/photo.jpg", converted[0].ContentParts[0].ImageURL.URL)
}

func TestConvertMessagesDocumentBase64(t *testing.T) {
	converted, err := convertMessages([]*llm.Message{
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.DocumentContent{
					Title: "report.pdf",
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeBase64,
						MediaType: "application/pdf",
						Data:      "cGRmZGF0YQ==",
					},
				},
			},
		},
	})
	assert.NoError(t, err)
	assert.Len(t, converted, 1)
	part := converted[0].ContentParts[0]
	assert.Equal(t, "file", part.Type)
	assert.Equal(t, "report.pdf", part.File.Filename)
	assert.Equal(t, "data:application/pdf;base64,cGRmZGF0YQ==", part.File.FileData)
}

func TestConvertMessagesDocumentFileID(t *testing.T) {
	converted, err := convertMessages([]*llm.Message{
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.DocumentContent{
					Source: &llm.ContentSource{
						Type:   llm.ContentSourceTypeFile,
						FileID: "file_abc123",
					},
				},
			},
		},
	})
	assert.NoError(t, err)
	part := converted[0].ContentParts[0]
	assert.Equal(t, "file", part.Type)
	assert.Equal(t, "file_abc123", part.File.FileID)
}

func TestConvertMessagesDocumentTextSource(t *testing.T) {
	converted, err := convertMessages([]*llm.Message{
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.DocumentContent{
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeText,
						MediaType: "text/plain",
						Data:      "The grass is green.",
					},
				},
			},
		},
	})
	assert.NoError(t, err)
	part := converted[0].ContentParts[0]
	assert.Equal(t, "text", part.Type)
	assert.Equal(t, "The grass is green.", part.Text)
}

func TestConvertMessagesDocumentURLErrors(t *testing.T) {
	_, err := convertMessages([]*llm.Message{
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.DocumentContent{
					Source: &llm.ContentSource{
						Type: llm.ContentSourceTypeURL,
						URL:  "https://example.com/doc.pdf",
					},
				},
			},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "url-based document content is not supported")
}

func TestConvertMessagesAssistantImageErrors(t *testing.T) {
	_, err := convertMessages([]*llm.Message{
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ImageContent{
					Source: &llm.ContentSource{
						Type: llm.ContentSourceTypeURL,
						URL:  "https://example.com/generated.png",
					},
				},
			},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported in assistant messages")
}

// TestConvertMessagesAuxImageWithToolResults verifies media content alongside
// tool results (e.g. a screenshot injected by a PostToolUse hook) is emitted
// as a trailing multimodal user message after the tool outputs.
func TestConvertMessagesAuxImageWithToolResults(t *testing.T) {
	converted, err := convertMessages([]*llm.Message{
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.ToolResultContent{ToolUseID: "call_1", Content: "done"},
				&llm.ImageContent{
					Source: &llm.ContentSource{
						Type: llm.ContentSourceTypeURL,
						URL:  "https://example.com/screenshot.png",
					},
				},
			},
		},
	})
	assert.NoError(t, err)
	assert.Len(t, converted, 2)
	assert.Equal(t, "tool", converted[0].Role)
	assert.Equal(t, "call_1", converted[0].ToolCallID)
	assert.Equal(t, "user", converted[1].Role)
	assert.Len(t, converted[1].ContentParts, 1)
	assert.Equal(t, "image_url", converted[1].ContentParts[0].Type)
}

// TestConvertMessagesJoinsToolCallText verifies that all text blocks in an
// assistant tool-call message are kept (the previous encoding kept only the
// last one).
func TestConvertMessagesJoinsToolCallText(t *testing.T) {
	converted, err := convertMessages([]*llm.Message{
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.TextContent{Text: "First thought."},
				&llm.TextContent{Text: "Second thought."},
				&llm.ToolUseContent{ID: "call_1", Name: "calc", Input: []byte(`{}`)},
			},
		},
	})
	assert.NoError(t, err)
	assert.Len(t, converted, 1)
	assert.Equal(t, "First thought.\n\nSecond thought.", converted[0].Content)
	assert.Len(t, converted[0].ToolCalls, 1)
}

// TestToolResultImageBlockPlaceholder verifies non-text tool result blocks
// are represented with a placeholder rather than dropped silently.
func TestToolResultImageBlockPlaceholder(t *testing.T) {
	converted, err := convertMessages([]*llm.Message{
		llm.NewToolResultMessage(&llm.ToolResultContent{
			ToolUseID: "call_1",
			Content: []*dive.ToolResultContent{
				{Type: dive.ToolResultContentTypeText, Text: "captured screenshot"},
				{Type: dive.ToolResultContentTypeImage, Data: "aW1n", MimeType: "image/png"},
			},
		}),
	})
	assert.NoError(t, err)
	assert.Len(t, converted, 1)
	assert.Equal(t, "captured screenshot\n[image content omitted]", converted[0].Content)
}

// TestRequestMarshalMultimodal verifies the full request wire shape for a
// multimodal message.
func TestRequestMarshalMultimodal(t *testing.T) {
	msgs, err := convertMessages([]*llm.Message{
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.TextContent{Text: "Describe:"},
				&llm.ImageContent{
					Source: &llm.ContentSource{
						Type: llm.ContentSourceTypeURL,
						URL:  "https://example.com/a.png",
					},
				},
			},
		},
	})
	assert.NoError(t, err)
	data, err := json.Marshal(Request{Model: "gpt-test", Messages: msgs})
	assert.NoError(t, err)
	assert.Equal(t, `{"model":"gpt-test","messages":[{"role":"user","content":[{"type":"text","text":"Describe:"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`, string(data))
}
