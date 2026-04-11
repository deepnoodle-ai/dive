package llm

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
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
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestTextContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &TextContent{Text: "Hello, world!"}
		got := c.Type()
		assert.Equal(t, ContentTypeText, got)
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
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestImageContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &ImageContent{}
		got := c.Type()
		assert.Equal(t, ContentTypeImage, got)
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
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestDocumentContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &DocumentContent{}
		got := c.Type()
		assert.Equal(t, ContentTypeDocument, got)
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
			{
				name: "document with file ID",
				content: &DocumentContent{
					Source: &ContentSource{
						Type:   ContentSourceTypeFile,
						FileID: "file-abc123",
					},
					Title: "PDF Document",
				},
				expected: `{"type":"document","source":{"type":"file","file_id":"file-abc123"},"title":"PDF Document"}`,
			},
			{
				name: "document with URL",
				content: &DocumentContent{
					Source: &ContentSource{
						Type: ContentSourceTypeURL,
						URL:  "https://example.com/document.pdf",
					},
					Title: "Remote PDF",
				},
				expected: `{"type":"document","source":{"type":"url","url":"https://example.com/document.pdf"},"title":"Remote PDF"}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestToolUseContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &ToolUseContent{}
		got := c.Type()
		assert.Equal(t, ContentTypeToolUse, got)
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
			{
				name: "nil input marshals as empty object",
				content: &ToolUseContent{
					ID:    "tool_456",
					Name:  "no_params_tool",
					Input: nil,
				},
				expected: `{"type":"tool_use","id":"tool_456","name":"no_params_tool","input":{}}`,
			},
			{
				name: "empty byte slice input marshals as empty object",
				content: &ToolUseContent{
					ID:    "tool_789",
					Name:  "another_tool",
					Input: json.RawMessage{},
				},
				expected: `{"type":"tool_use","id":"tool_789","name":"another_tool","input":{}}`,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := json.Marshal(tt.content)
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestToolResultContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &ToolResultContent{}
		got := c.Type()
		assert.Equal(t, ContentTypeToolResult, got)
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
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(got))
			})
		}
	})
}

// TestToolResultContent_DecodeContent pins the typed-decode helper that
// callers use to recover the original shape of a tool result after it has
// been round-tripped through JSON — which happens transparently during
// Message.Copy, session persistence, and cross-process suspend/resume.
//
// The underlying problem: ToolResultContent.Content is `any`, so once the
// struct is unmarshaled from JSON, reading Content directly yields the
// generic decoded shape (map[string]any / []any / float64), discarding the
// caller's original Go types. DecodeContent replays the preserved raw JSON
// bytes into a typed destination instead.
func TestToolResultContent_DecodeContent(t *testing.T) {
	type nestedItem struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		Count int    `json:"count"`
	}

	t.Run("typed decode after json round trip", func(t *testing.T) {
		// Round-trip through JSON the way Message.Copy does.
		orig := &ToolResultContent{
			ToolUseID: "tool_123",
			Content: []*nestedItem{
				{Type: "text", Text: "hello", Count: 7},
				{Type: "text", Text: "world", Count: 42},
			},
		}
		data, err := json.Marshal(orig)
		assert.NoError(t, err)

		var decoded ToolResultContent
		assert.NoError(t, json.Unmarshal(data, &decoded))

		// Reading Content directly still gives the generic shape — this is
		// the backwards-compatible behavior that existing callers depend on.
		generic, ok := decoded.Content.([]any)
		assert.True(t, ok, "Content should decode as []any for backwards compat")
		assert.Equal(t, len(generic), 2)

		// DecodeContent replays the raw bytes into the original typed shape.
		var items []*nestedItem
		assert.NoError(t, decoded.DecodeContent(&items))
		assert.Equal(t, len(items), 2)
		assert.Equal(t, items[0].Type, "text")
		assert.Equal(t, items[0].Text, "hello")
		assert.Equal(t, items[0].Count, 7)
		assert.Equal(t, items[1].Count, 42)
	})

	t.Run("integers stay integers after round trip", func(t *testing.T) {
		// The classic JSON round-trip pitfall: numbers come back as
		// float64. DecodeContent avoids it by unmarshaling into a typed
		// destination rather than a map[string]any.
		raw := `{"tool_use_id":"t","content":{"count":7,"ratio":1.5}}`
		var c ToolResultContent
		assert.NoError(t, json.Unmarshal([]byte(raw), &c))

		// Reading through the generic map: int becomes float64.
		m, ok := c.Content.(map[string]any)
		assert.True(t, ok)
		_, isFloat := m["count"].(float64)
		assert.True(t, isFloat, "Content map coerces ints to float64")

		// DecodeContent into a typed struct: int stays int.
		type counts struct {
			Count int     `json:"count"`
			Ratio float64 `json:"ratio"`
		}
		var out counts
		assert.NoError(t, c.DecodeContent(&out))
		assert.Equal(t, out.Count, 7)
		assert.Equal(t, out.Ratio, 1.5)
	})

	t.Run("works on in-memory struct without round trip", func(t *testing.T) {
		// No json.Unmarshal ever happened, so rawContent is empty. The
		// fallback path marshals Content then decodes into dst.
		c := &ToolResultContent{
			ToolUseID: "t",
			Content: []*nestedItem{
				{Type: "text", Text: "fresh", Count: 3},
			},
		}
		var items []*nestedItem
		assert.NoError(t, c.DecodeContent(&items))
		assert.Equal(t, len(items), 1)
		assert.Equal(t, items[0].Text, "fresh")
		assert.Equal(t, items[0].Count, 3)
	})

	t.Run("string content", func(t *testing.T) {
		raw := `{"tool_use_id":"t","content":"15 degrees"}`
		var c ToolResultContent
		assert.NoError(t, json.Unmarshal([]byte(raw), &c))

		var s string
		assert.NoError(t, c.DecodeContent(&s))
		assert.Equal(t, s, "15 degrees")
	})

	t.Run("empty content is a no-op", func(t *testing.T) {
		c := &ToolResultContent{ToolUseID: "t"}
		var out []any
		assert.NoError(t, c.DecodeContent(&out))
		assert.Equal(t, len(out), 0)
	})

	t.Run("nil receiver returns nil", func(t *testing.T) {
		var c *ToolResultContent
		var out []any
		assert.NoError(t, c.DecodeContent(&out))
	})

	t.Run("generic helper", func(t *testing.T) {
		orig := &ToolResultContent{
			ToolUseID: "t",
			Content: []*nestedItem{
				{Type: "text", Text: "g", Count: 9},
			},
		}
		data, err := json.Marshal(orig)
		assert.NoError(t, err)
		var decoded ToolResultContent
		assert.NoError(t, json.Unmarshal(data, &decoded))

		items, err := DecodeToolResultContent[[]*nestedItem](&decoded)
		assert.NoError(t, err)
		assert.Equal(t, len(items), 1)
		assert.Equal(t, items[0].Count, 9)
	})

	t.Run("survives Message.Copy", func(t *testing.T) {
		// Message.Copy is the concrete real-world path that triggers this:
		// it marshals and unmarshals the whole message to produce an
		// independent deep copy. Any typed Content a caller stashed in a
		// ToolResultContent must still be recoverable via DecodeContent.
		msg := &Message{
			Role: User,
			Content: []Content{
				&ToolResultContent{
					ToolUseID: "tool_123",
					Content: []*nestedItem{
						{Type: "text", Text: "a", Count: 1},
						{Type: "text", Text: "b", Count: 2},
					},
				},
			},
		}
		cp := msg.Copy()
		trc, ok := cp.Content[0].(*ToolResultContent)
		assert.True(t, ok)

		var items []*nestedItem
		assert.NoError(t, trc.DecodeContent(&items))
		assert.Equal(t, len(items), 2)
		assert.Equal(t, items[0].Text, "a")
		assert.Equal(t, items[1].Count, 2)
	})

	t.Run("round trip marshal re-emits content", func(t *testing.T) {
		// Make sure that after our custom UnmarshalJSON populates both
		// rawContent and Content, a subsequent Marshal still emits the
		// content field (from the generic Content value, not the raw
		// bytes — simple and correct).
		orig := `{"tool_use_id":"t","content":{"a":1},"is_error":true}`
		var c ToolResultContent
		assert.NoError(t, json.Unmarshal([]byte(orig), &c))

		out, err := json.Marshal(&c)
		assert.NoError(t, err)

		// Re-decoding proves round-trip parity for the exported fields.
		var again ToolResultContent
		assert.NoError(t, json.Unmarshal(out, &again))
		assert.Equal(t, again.ToolUseID, "t")
		assert.Equal(t, again.IsError, true)
		var body struct {
			A int `json:"a"`
		}
		assert.NoError(t, again.DecodeContent(&body))
		assert.Equal(t, body.A, 1)
	})
}

func TestServerToolUseContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &ServerToolUseContent{}
		got := c.Type()
		assert.Equal(t, ContentTypeServerToolUse, got)
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
				assert.NoError(t, err)

				// We need to unmarshal and compare objects since map ordering is non-deterministic
				var gotObj, expectedObj map[string]interface{}
				err = json.Unmarshal(got, &gotObj)
				assert.NoError(t, err)

				err = json.Unmarshal([]byte(tt.expected), &expectedObj)
				assert.NoError(t, err)

				assert.Equal(t, expectedObj, gotObj)
			})
		}
	})
}

func TestWebSearchToolResultContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &WebSearchToolResultContent{}
		got := c.Type()
		assert.Equal(t, ContentTypeWebSearchToolResult, got)
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
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestThinkingContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &ThinkingContent{}
		got := c.Type()
		assert.Equal(t, ContentTypeThinking, got)
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
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestRedactedThinkingContent(t *testing.T) {
	t.Run("Type", func(t *testing.T) {
		c := &RedactedThinkingContent{}
		got := c.Type()
		assert.Equal(t, ContentTypeRedactedThinking, got)
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
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, string(got))
			})
		}
	})
}

func TestCloneContent(t *testing.T) {
	cc := &CacheControl{Type: CacheControlTypeEphemeral}

	t.Run("TextContent", func(t *testing.T) {
		orig := &TextContent{Text: "hello", CacheControl: cc, Citations: []Citation{&MockCitation{Text: "cite"}}}
		clone := orig.CloneContent()
		cloned := clone.(*TextContent)
		assert.True(t, orig != cloned)
		assert.Equal(t, "hello", cloned.Text)
		assert.Nil(t, cloned.CacheControl)
		assert.Len(t, cloned.Citations, 1)
	})

	t.Run("RefusalContent", func(t *testing.T) {
		orig := &RefusalContent{Text: "refused", CacheControl: cc}
		clone := orig.CloneContent()
		cloned := clone.(*RefusalContent)
		assert.True(t, orig != cloned)
		assert.Equal(t, "refused", cloned.Text)
		assert.Nil(t, cloned.CacheControl)
	})

	t.Run("ImageContent", func(t *testing.T) {
		src := &ContentSource{Type: ContentSourceTypeURL, URL: "https://example.com/img.jpg"}
		orig := &ImageContent{Source: src, CacheControl: cc}
		clone := orig.CloneContent()
		cloned := clone.(*ImageContent)
		assert.True(t, orig != cloned)
		assert.Equal(t, src, cloned.Source)
		assert.Nil(t, cloned.CacheControl)
	})

	t.Run("DocumentContent", func(t *testing.T) {
		src := &ContentSource{Type: ContentSourceTypeBase64, Data: "abc"}
		orig := &DocumentContent{Source: src, Title: "doc", CacheControl: cc}
		clone := orig.CloneContent()
		cloned := clone.(*DocumentContent)
		assert.True(t, orig != cloned)
		assert.Equal(t, "doc", cloned.Title)
		assert.Equal(t, src, cloned.Source)
		assert.Nil(t, cloned.CacheControl)
	})

	t.Run("ToolResultContent", func(t *testing.T) {
		orig := &ToolResultContent{ToolUseID: "id1", Content: "result", IsError: true, CacheControl: cc}
		clone := orig.CloneContent()
		cloned := clone.(*ToolResultContent)
		assert.True(t, orig != cloned)
		assert.Equal(t, "id1", cloned.ToolUseID)
		assert.Equal(t, "result", cloned.Content)
		assert.True(t, cloned.IsError)
		assert.Nil(t, cloned.CacheControl)
	})

	t.Run("SummaryContent", func(t *testing.T) {
		orig := &SummaryContent{Summary: "summary text", CacheControl: cc}
		clone := orig.CloneContent()
		cloned := clone.(*SummaryContent)
		assert.True(t, orig != cloned)
		assert.Equal(t, "summary text", cloned.Summary)
		assert.Nil(t, cloned.CacheControl)
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

func TestMCPContentTypes(t *testing.T) {
	t.Run("MCPToolsListContent", func(t *testing.T) {
		t.Run("Type", func(t *testing.T) {
			c := &MCPListToolsContent{}
			got := c.Type()
			assert.Equal(t, ContentTypeMCPListTools, got)
		})

		t.Run("MarshalJSON", func(t *testing.T) {
			tests := []struct {
				name     string
				content  *MCPListToolsContent
				expected string
			}{
				{
					name: "basic tools list",
					content: &MCPListToolsContent{
						ServerLabel: "deepwiki",
						Tools: []*MCPToolDefinition{
							{
								Name:        "ask_question",
								Description: "Ask a question about a GitHub repository",
							},
						},
					},
					expected: `{"type":"mcp_list_tools","server_label":"deepwiki","tools":[{"name":"ask_question","description":"Ask a question about a GitHub repository"}]}`,
				},
				{
					name: "multiple tools",
					content: &MCPListToolsContent{
						ServerLabel: "deepwiki",
						Tools: []*MCPToolDefinition{
							{
								Name:        "ask_question",
								Description: "Ask a question about a GitHub repository",
							},
							{
								Name:        "search_repos",
								Description: "Search for GitHub repositories",
							},
						},
					},
					expected: `{"type":"mcp_list_tools","server_label":"deepwiki","tools":[{"name":"ask_question","description":"Ask a question about a GitHub repository"},{"name":"search_repos","description":"Search for GitHub repositories"}]}`,
				},
				{
					name: "tool without description",
					content: &MCPListToolsContent{
						ServerLabel: "simple-server",
						Tools: []*MCPToolDefinition{
							{
								Name: "simple_tool",
							},
						},
					},
					expected: `{"type":"mcp_list_tools","server_label":"simple-server","tools":[{"name":"simple_tool"}]}`,
				},
				{
					name: "empty tools list",
					content: &MCPListToolsContent{
						ServerLabel: "empty-server",
						Tools:       []*MCPToolDefinition{},
					},
					expected: `{"type":"mcp_list_tools","server_label":"empty-server","tools":[]}`,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := json.Marshal(tt.content)
					assert.NoError(t, err)
					assert.Equal(t, tt.expected, string(got))
				})
			}
		})

		t.Run("UnmarshalContent", func(t *testing.T) {
			data := []byte(`{"type":"mcp_list_tools","server_label":"deepwiki","tools":[{"name":"ask_question","description":"Ask a question about a GitHub repository"}]}`)
			content, err := UnmarshalContent(data)
			assert.NoError(t, err)

			toolsList, ok := content.(*MCPListToolsContent)
			assert.True(t, ok)
			assert.Equal(t, "deepwiki", toolsList.ServerLabel)
			assert.Len(t, toolsList.Tools, 1)
			assert.Equal(t, "ask_question", toolsList.Tools[0].Name)
			assert.Equal(t, "Ask a question about a GitHub repository", toolsList.Tools[0].Description)
		})
	})

	t.Run("MCPApprovalRequestContent", func(t *testing.T) {
		t.Run("Type", func(t *testing.T) {
			c := &MCPApprovalRequestContent{}
			got := c.Type()
			assert.Equal(t, ContentTypeMCPApprovalRequest, got)
		})

		t.Run("MarshalJSON", func(t *testing.T) {
			tests := []struct {
				name     string
				content  *MCPApprovalRequestContent
				expected string
			}{
				{
					name: "basic approval request",
					content: &MCPApprovalRequestContent{
						ID:          "ID",
						Arguments:   "ARG",
						Name:        "ask_question",
						ServerLabel: "deepwiki",
					},
					expected: `{"type":"mcp_approval_request","id":"ID","arguments":"ARG","name":"ask_question","server_label":"deepwiki"}`,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := json.Marshal(tt.content)
					assert.NoError(t, err)
					assert.Equal(t, tt.expected, string(got))
				})
			}
		})

		t.Run("UnmarshalContent", func(t *testing.T) {
			data := []byte(`{"type":"mcp_approval_request","name":"ask_question","server_label":"deepwiki","id":"mcpr_123456"}`)
			content, err := UnmarshalContent(data)
			assert.NoError(t, err)

			approvalRequest, ok := content.(*MCPApprovalRequestContent)
			assert.True(t, ok)
			assert.Equal(t, "ask_question", approvalRequest.Name)
			assert.Equal(t, "deepwiki", approvalRequest.ServerLabel)
			assert.Equal(t, "mcpr_123456", approvalRequest.ID)
		})
	})

	t.Run("MCPToolDefinition with InputSchema", func(t *testing.T) {
		t.Run("MarshalJSON with schema", func(t *testing.T) {
			tool := &MCPToolDefinition{
				Name:        "test_tool",
				Description: "A test tool",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"param1": map[string]interface{}{
							"type":        "string",
							"description": "First parameter",
						},
					},
					"required": []string{"param1"},
				},
			}

			data, err := json.Marshal(tool)
			assert.NoError(t, err)

			// Unmarshal back to verify structure
			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			assert.NoError(t, err)

			assert.Equal(t, "test_tool", result["name"])
			assert.Equal(t, "A test tool", result["description"])
			assert.NotNil(t, result["input_schema"])

			schema, ok := result["input_schema"].(map[string]interface{})
			assert.True(t, ok)
			assert.Equal(t, "object", schema["type"])
		})

		t.Run("MCPToolsListContent with enhanced tools", func(t *testing.T) {
			toolsList := &MCPListToolsContent{
				ServerLabel: "enhanced-server",
				Tools: []*MCPToolDefinition{
					{
						Name:        "enhanced_tool",
						Description: "An enhanced tool with schema",
						InputSchema: map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"query": map[string]interface{}{
									"type":        "string",
									"description": "Search query",
								},
							},
						},
					},
				},
			}

			data, err := json.Marshal(toolsList)
			assert.NoError(t, err)

			// Verify it can be unmarshaled
			content, err := UnmarshalContent(data)
			assert.NoError(t, err)

			parsedList, ok := content.(*MCPListToolsContent)
			assert.True(t, ok)
			assert.Equal(t, "enhanced-server", parsedList.ServerLabel)
			assert.Len(t, parsedList.Tools, 1)
			assert.Equal(t, "enhanced_tool", parsedList.Tools[0].Name)
			assert.NotNil(t, parsedList.Tools[0].InputSchema)
		})
	})
}
