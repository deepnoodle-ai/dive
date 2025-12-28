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
