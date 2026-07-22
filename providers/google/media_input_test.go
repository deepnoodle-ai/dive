package google

import (
	"encoding/base64"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func userMessage(content ...llm.Content) *llm.Message {
	return &llm.Message{Role: llm.User, Content: content}
}

func TestMessagesToContentsDocumentBase64(t *testing.T) {
	pdfData := []byte("%PDF-1.4 fake")
	contents, err := messagesToContents([]*llm.Message{
		userMessage(
			&llm.TextContent{Text: "Summarize this."},
			&llm.DocumentContent{
				Source: &llm.ContentSource{
					Type:      llm.ContentSourceTypeBase64,
					MediaType: "application/pdf",
					Data:      base64.StdEncoding.EncodeToString(pdfData),
				},
			},
		),
	})
	assert.NoError(t, err)
	assert.Len(t, contents, 1)
	assert.Len(t, contents[0].Parts, 2)
	assert.Equal(t, "Summarize this.", contents[0].Parts[0].Text)
	blob := contents[0].Parts[1].InlineData
	assert.NotNil(t, blob)
	assert.Equal(t, "application/pdf", blob.MIMEType)
	assert.Equal(t, pdfData, blob.Data)
}

func TestMessagesToContentsDocumentURL(t *testing.T) {
	contents, err := messagesToContents([]*llm.Message{
		userMessage(&llm.DocumentContent{
			Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeURL,
				MediaType: "application/pdf",
				URL:       "https://example.com/report.pdf",
			},
		}),
	})
	assert.NoError(t, err)
	fileData := contents[0].Parts[0].FileData
	assert.NotNil(t, fileData)
	assert.Equal(t, "https://example.com/report.pdf", fileData.FileURI)
	assert.Equal(t, "application/pdf", fileData.MIMEType)
}

func TestMessagesToContentsDocumentTextSource(t *testing.T) {
	contents, err := messagesToContents([]*llm.Message{
		userMessage(&llm.DocumentContent{
			Source: &llm.ContentSource{
				Type:      llm.ContentSourceTypeText,
				MediaType: "text/plain",
				Data:      "The grass is green.",
			},
		}),
	})
	assert.NoError(t, err)
	assert.Equal(t, "The grass is green.", contents[0].Parts[0].Text)
}

func TestMessagesToContentsDocumentErrors(t *testing.T) {
	tests := []struct {
		name    string
		content llm.Content
		wantErr string
	}{
		{
			name:    "nil source",
			content: &llm.DocumentContent{},
			wantErr: "document content has nil source",
		},
		{
			name: "base64 without media type",
			content: &llm.DocumentContent{
				Source: &llm.ContentSource{
					Type: llm.ContentSourceTypeBase64,
					Data: "cGRm",
				},
			},
			wantErr: "media type is required for base64 document content",
		},
		{
			name: "URL source without URL",
			content: &llm.DocumentContent{
				Source: &llm.ContentSource{Type: llm.ContentSourceTypeURL},
			},
			wantErr: "URL is required for URL-based document content",
		},
		{
			name: "text source without data",
			content: &llm.DocumentContent{
				Source: &llm.ContentSource{Type: llm.ContentSourceTypeText},
			},
			wantErr: "data is required for text document content",
		},
		{
			name: "file source is unsupported",
			content: &llm.DocumentContent{
				Source: &llm.ContentSource{
					Type:   llm.ContentSourceTypeFile,
					FileID: "file_abc",
				},
			},
			wantErr: "unsupported document source type: file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := messagesToContents([]*llm.Message{userMessage(tt.content)})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMessagesToContentsImageErrors(t *testing.T) {
	tests := []struct {
		name    string
		content llm.Content
		wantErr string
	}{
		{
			name: "base64 without media type",
			content: &llm.ImageContent{
				Source: &llm.ContentSource{
					Type: llm.ContentSourceTypeBase64,
					Data: "aW1n",
				},
			},
			wantErr: "media type is required for base64 image content",
		},
		{
			name: "URL source without URL",
			content: &llm.ImageContent{
				Source: &llm.ContentSource{Type: llm.ContentSourceTypeURL},
			},
			wantErr: "URL is required for URL-based image content",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := messagesToContents([]*llm.Message{userMessage(tt.content)})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestMessagesToContentsSkipsThinking verifies that thinking blocks (e.g. from
// a session started on another provider) are skipped rather than erroring or
// being sent to Gemini.
func TestMessagesToContentsSkipsThinking(t *testing.T) {
	contents, err := messagesToContents([]*llm.Message{
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ThinkingContent{Thinking: "hmm..."},
				&llm.RedactedThinkingContent{Data: "opaque"},
				&llm.TextContent{Text: "The answer is 4."},
			},
		},
	})
	assert.NoError(t, err)
	assert.Len(t, contents, 1)
	assert.Len(t, contents[0].Parts, 1)
	assert.Equal(t, "The answer is 4.", contents[0].Parts[0].Text)
}

// TestMessagesToContentsUnknownContentErrors verifies the switch has no silent
// fall-through: content the provider cannot encode is a visible error, not a
// dropped block.
func TestMessagesToContentsUnknownContentErrors(t *testing.T) {
	_, err := messagesToContents([]*llm.Message{
		userMessage(&llm.ServerToolUseContent{ID: "srv_1", Name: "web_search"}),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported content type for google provider: server_tool_use")
}

// TestJoinToolResultTextPlaceholders verifies non-text tool result blocks are
// represented with a placeholder rather than dropped silently.
func TestJoinToolResultTextPlaceholders(t *testing.T) {
	joined := joinToolResultText([]*dive.ToolResultContent{
		{Type: dive.ToolResultContentTypeText, Text: "captured screenshot"},
		{Type: dive.ToolResultContentTypeImage, Data: "aW1n", MimeType: "image/png"},
	})
	assert.Equal(t, "captured screenshot\n\n[image content omitted]", joined)
}

// TestJoinToolResultTextEmpty verifies a result with no renderable text says
// so explicitly instead of producing an empty function response.
func TestJoinToolResultTextEmpty(t *testing.T) {
	assert.Equal(t, "(no output)", joinToolResultText(nil))
	assert.Equal(t, "(no output)", joinToolResultText([]*dive.ToolResultContent{
		{Type: dive.ToolResultContentTypeText, Text: ""},
	}))
}
