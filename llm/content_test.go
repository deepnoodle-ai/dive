package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContentTypeStringMethods(t *testing.T) {
	tests := []struct {
		name     string
		source   ContentSourceType
		expected string
	}{
		{
			name:     "ContentSourceTypeBase64",
			source:   ContentSourceTypeBase64,
			expected: "base64",
		},
		{
			name:     "ContentSourceTypeURL",
			source:   ContentSourceTypeURL,
			expected: "url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.source.String()
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestTextContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &TextContent{Text: "Hello, world!"}
		got := c.Type()
		require.Equal(t, ContentTypeText, got)
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		tests := []struct {
			name     string
			content  *TextContent
			expected string
		}{
			{
				name:     "basic text",
				content:  &TextContent{Text: "Hello, world!"},
				expected: `{"type":"text","text":"Hello, world!"}`,
			},
			{
				name: "with cache control",
				content: &TextContent{
					Text:         "Hello, world!",
					CacheControl: &CacheControl{Type: "test-cache"},
				},
				expected: `{"type":"text","text":"Hello, world!","cache_control":{"type":"test-cache"}}`,
			},
			{
				name: "with citations",
				content: &TextContent{
					Text: "Hello, world!",
					Citations: []Citation{
						&MockCitation{
							Text: "Test citation",
						},
					},
				},
				expected: `{"type":"text","text":"Hello, world!","citations":[{"cited_text":"Test citation"}]}`,
			},

			{
				name: "with citations",
				content: &TextContent{
					Text: "Hello, world!",
					Citations: []Citation{
						&MockCitation{
							Text: "Test citation",
						},
					},
				},
				expected: `{"type":"text","text":"Hello, world!","citations":[{"cited_text":"Test citation"}]}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				require.NoError(t, err)
				require.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestImageContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &ImageContent{}
		got := c.Type()
		require.Equal(t, ContentTypeImage, got)
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		tests := []struct {
			name     string
			content  *ImageContent
			expected string
		}{
			{
				name: "base64 source",
				content: &ImageContent{
					Source: &ContentSource{
						Type:      ContentSourceTypeBase64,
						MediaType: "image/jpeg",
						Data:      "base64data",
					},
				},
				expected: `{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"base64data"}}`,
			},
			{
				name: "url source",
				content: &ImageContent{
					Source: &ContentSource{
						Type: ContentSourceTypeURL,
						URL:  "https://example.com/image.jpg",
					},
				},
				expected: `{"type":"image","source":{"type":"url","url":"https://example.com/image.jpg"}}`,
			},
			{
				name: "with cache control",
				content: &ImageContent{
					Source: &ContentSource{
						Type: ContentSourceTypeURL,
						URL:  "https://example.com/image.jpg",
					},
					CacheControl: &CacheControl{Type: "test-cache"},
				},
				expected: `{"type":"image","source":{"type":"url","url":"https://example.com/image.jpg"},"cache_control":{"type":"test-cache"}}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				require.NoError(t, err)
				require.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestDocumentContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &DocumentContent{}
		got := c.Type()
		require.Equal(t, ContentTypeDocument, got)
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		tests := []struct {
			name     string
			content  *DocumentContent
			expected string
		}{
			{
				name: "basic document",
				content: &DocumentContent{
					Source: &ContentSource{
						Type:      ContentSourceTypeBase64,
						MediaType: "text/plain",
						Data:      "base64data",
					},
					Title: "Document Title",
				},
				expected: `{"type":"document","source":{"type":"base64","media_type":"text/plain","data":"base64data"},"title":"Document Title"}`,
			},
			{
				name: "document with context",
				content: &DocumentContent{
					Source: &ContentSource{
						Type:      ContentSourceTypeBase64,
						MediaType: "text/plain",
						Data:      "base64data",
					},
					Title:   "Document Title",
					Context: "Document Context",
				},
				expected: `{"type":"document","source":{"type":"base64","media_type":"text/plain","data":"base64data"},"title":"Document Title","context":"Document Context"}`,
			},
			{
				name: "document with citations",
				content: &DocumentContent{
					Source: &ContentSource{
						Type:      ContentSourceTypeBase64,
						MediaType: "text/plain",
						Data:      "base64data",
					},
					Title:     "Document Title",
					Context:   "Document Context",
					Citations: &CitationSettings{Enabled: true},
				},
				expected: `{"type":"document","source":{"type":"base64","media_type":"text/plain","data":"base64data"},"title":"Document Title","context":"Document Context","citations":{"enabled":true}}`,
			},
			{
				name: "document with content chunks",
				content: &DocumentContent{
					Source: &ContentSource{
						Type: "content",
						Content: []*ContentChunk{
							{Type: "text", Text: "Chunk 1"},
							{Type: "text", Text: "Chunk 2"},
						},
					},
					Title: "Document Title",
				},
				expected: `{"type":"document","source":{"type":"content","content":[{"type":"text","text":"Chunk 1"},{"type":"text","text":"Chunk 2"}]},"title":"Document Title"}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				require.NoError(t, err)
				require.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestToolUseContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &ToolUseContent{}
		got := c.Type()
		require.Equal(t, ContentTypeToolUse, got)
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		tests := []struct {
			name     string
			content  *ToolUseContent
			expected string
		}{
			{
				name: "basic tool use",
				content: &ToolUseContent{
					ID:    "tool_123",
					Name:  "get_weather",
					Input: json.RawMessage(`{"location":"San Francisco"}`),
				},
				expected: `{"type":"tool_use","id":"tool_123","name":"get_weather","input":{"location":"San Francisco"}}`,
			},
			{
				name: "empty input",
				content: &ToolUseContent{
					ID:    "tool_123",
					Name:  "get_weather",
					Input: json.RawMessage(`{}`),
				},
				expected: `{"type":"tool_use","id":"tool_123","name":"get_weather","input":{}}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				require.NoError(t, err)
				require.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestToolResultContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &ToolResultContent{}
		got := c.Type()
		require.Equal(t, ContentTypeToolResult, got)
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		tests := []struct {
			name     string
			content  *ToolResultContent
			expected string
		}{
			{
				name: "string result",
				content: &ToolResultContent{
					ToolUseID: "tool_123",
					Content:   "15 degrees",
				},
				expected: `{"type":"tool_result","tool_use_id":"tool_123","content":"15 degrees"}`,
			},
			{
				name: "complex result",
				content: &ToolResultContent{
					ToolUseID: "tool_123",
					Content: map[string]interface{}{
						"temperature": 15,
						"unit":        "celsius",
					},
				},
				expected: `{"type":"tool_result","tool_use_id":"tool_123","content":{"temperature":15,"unit":"celsius"}}`,
			},
			{
				name: "error result",
				content: &ToolResultContent{
					ToolUseID: "tool_123",
					Content:   "Error: Missing required parameter",
					IsError:   true,
				},
				expected: `{"type":"tool_result","tool_use_id":"tool_123","content":"Error: Missing required parameter","is_error":true}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				require.NoError(t, err)
				require.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestServerToolUseContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &ServerToolUseContent{}
		got := c.Type()
		require.Equal(t, ContentTypeServerToolUse, got)
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		tests := []struct {
			name     string
			content  *ServerToolUseContent
			expected string
		}{
			{
				name: "basic server tool use",
				content: &ServerToolUseContent{
					ID:   "srvtool_123",
					Name: "web_search",
					Input: map[string]interface{}{
						"query": "claude shannon birth date",
					},
				},
				expected: `{"type":"server_tool_use","id":"srvtool_123","name":"web_search","input":{"query":"claude shannon birth date"}}`,
			},
			{
				name: "multiple inputs",
				content: &ServerToolUseContent{
					ID:   "srvtool_123",
					Name: "web_search",
					Input: map[string]interface{}{
						"query":  "claude shannon birth date",
						"limit":  5,
						"filter": true,
					},
				},
				expected: `{"type":"server_tool_use","id":"srvtool_123","name":"web_search","input":{"filter":true,"limit":5,"query":"claude shannon birth date"}}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				require.NoError(t, err)

				// We need to unmarshal and compare objects since map ordering is non-deterministic
				var gotObj, expectedObj map[string]interface{}
				err = json.Unmarshal(got, &gotObj)
				require.NoError(t, err)

				err = json.Unmarshal([]byte(tt.expected), &expectedObj)
				require.NoError(t, err)

				require.Equal(t, expectedObj, gotObj)
			})
		}
	})
}

func TestWebSearchToolResultContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &WebSearchToolResultContent{}
		got := c.Type()
		require.Equal(t, ContentTypeWebSearchToolResult, got)
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		tests := []struct {
			name     string
			content  *WebSearchToolResultContent
			expected string
		}{
			{
				name: "basic search results",
				content: &WebSearchToolResultContent{
					ToolUseID: "srvtool_123",
					Content: []*WebSearchResult{
						{
							Type:             "web_search_result",
							URL:              "https://example.com/page1",
							Title:            "Example Page 1",
							EncryptedContent: "encrypted123",
							PageAge:          "April 30, 2025",
						},
					},
				},
				expected: `{"type":"web_search_tool_result","tool_use_id":"srvtool_123","content":[{"type":"web_search_result","url":"https://example.com/page1","title":"Example Page 1","encrypted_content":"encrypted123","page_age":"April 30, 2025"}]}`,
			},
			{
				name: "multiple search results",
				content: &WebSearchToolResultContent{
					ToolUseID: "srvtool_123",
					Content: []*WebSearchResult{
						{
							Type:             "web_search_result",
							URL:              "https://example.com/page1",
							Title:            "Example Page 1",
							EncryptedContent: "encrypted123",
							PageAge:          "April 30, 2025",
						},
						{
							Type:             "web_search_result",
							URL:              "https://example.com/page2",
							Title:            "Example Page 2",
							EncryptedContent: "encrypted456",
							PageAge:          "May 1, 2025",
						},
					},
				},
				expected: `{"type":"web_search_tool_result","tool_use_id":"srvtool_123","content":[{"type":"web_search_result","url":"https://example.com/page1","title":"Example Page 1","encrypted_content":"encrypted123","page_age":"April 30, 2025"},{"type":"web_search_result","url":"https://example.com/page2","title":"Example Page 2","encrypted_content":"encrypted456","page_age":"May 1, 2025"}]}`,
			},
			{
				name: "with error code",
				content: &WebSearchToolResultContent{
					ToolUseID: "srvtool_123",
					Content:   []*WebSearchResult{},
					ErrorCode: "max_uses_exceeded",
				},
				expected: `{"type":"web_search_tool_result","tool_use_id":"srvtool_123","content":[],"error_code":"max_uses_exceeded"}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				require.NoError(t, err)
				require.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestThinkingContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &ThinkingContent{}
		got := c.Type()
		require.Equal(t, ContentTypeThinking, got)
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		tests := []struct {
			name     string
			content  *ThinkingContent
			expected string
		}{
			{
				name: "basic thinking",
				content: &ThinkingContent{
					Thinking:  "Let me analyze this step by step...",
					Signature: "signature123",
				},
				expected: `{"type":"thinking","thinking":"Let me analyze this step by step...","signature":"signature123"}`,
			},
			{
				name: "empty thinking",
				content: &ThinkingContent{
					Thinking:  "",
					Signature: "signature123",
				},
				expected: `{"type":"thinking","thinking":"","signature":"signature123"}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				require.NoError(t, err)
				require.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestRedactedThinkingContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &RedactedThinkingContent{}
		got := c.Type()
		require.Equal(t, ContentTypeRedactedThinking, got)
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		tests := []struct {
			name     string
			content  *RedactedThinkingContent
			expected string
		}{
			{
				name: "basic redacted thinking",
				content: &RedactedThinkingContent{
					Data: "encrypted123",
				},
				expected: `{"type":"redacted_thinking","data":"encrypted123"}`,
			},
			{
				name: "empty data",
				content: &RedactedThinkingContent{
					Data: "",
				},
				expected: `{"type":"redacted_thinking","data":""}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				require.NoError(t, err)
				require.Equal(t, tt.expected, string(got))
			})
		}
	})
}

// MockCitation implements the Citation interface for testing
type MockCitation struct {
	Text string `json:"cited_text"`
}

func (c *MockCitation) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{
		"cited_text": c.Text,
	})
}

func (c *MockCitation) IsCitation() bool {
	return true
}
