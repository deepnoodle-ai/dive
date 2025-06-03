package openai

import (
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/openai/openai-go"
	"github.com/stretchr/testify/require"
)

// func TestMCPIntegration(t *testing.T) {
// 	provider := New(WithModel("gpt-4.1"))

// 	t.Run("MCP server configuration from llm.Config", func(t *testing.T) {
// 		config := &llm.Config{
// 			Messages: []*llm.Message{
// 				llm.NewUserTextMessage("Test message"),
// 			},
// 			MCPServers: []llm.MCPServerConfig{
// 				{
// 					Type: "url",
// 					Name: "test-server",
// 					URL:  "https://example.com/mcp",
// 					ToolConfiguration: &llm.MCPToolConfiguration{
// 						Enabled:      true,
// 						AllowedTools: []string{"query", "search"},
// 					},
// 					AuthorizationToken: "test-token",
// 				},
// 			},
// 		}

// 		request, err := provider.buildRequest(config)
// 		require.NoError(t, err)

// 		// Check that MCP server was added to tools
// 		var mcpTool map[string]any
// 		for _, tool := range request.Tools {
// 			t := tool.(map[string]any)
// 			if t["type"] == "mcp" && t["server_label"] == "test-server" {
// 				mcpTool = t
// 				break
// 			}
// 		}

// 		require.NotNil(t, mcpTool, "MCP tool should be present")
// 		assert.Equal(t, "test-server", mcpTool["server_label"])
// 		assert.Equal(t, "https://example.com/mcp", mcpTool["server_url"])
// 		assert.Equal(t, []string{"query", "search"}, mcpTool["allowed_tools"])
// 		assert.Equal(t, "always", mcpTool["require_approval"]) // Default value
// 		assert.Equal(t, "Bearer test-token", mcpTool["headers"].(map[string]string)["Authorization"])
// 	})

// 	t.Run("MCP approval response handling", func(t *testing.T) {
// 		messages := []*llm.Message{
// 			llm.NewUserTextMessage("Regular message"),
// 			{
// 				Role: llm.User,
// 				Content: []llm.Content{
// 					&llm.TextContent{
// 						Text: "MCP_APPROVAL_RESPONSE:mcpr_123:true",
// 					},
// 				},
// 			},
// 		}

// 		inputMessages, err := provider.convertMessagesToInput(messages)
// 		require.NoError(t, err)
// 		require.Len(t, inputMessages, 2)
// 		require.Len(t, inputMessages[1].Content, 1)

// 		content := inputMessages[1].Content[0]
// 		assert.Equal(t, "mcp_approval_response", content.Type)
// 		assert.Equal(t, "mcpr_123", content.ApprovalRequestID)
// 		assert.NotNil(t, content.Approve)
// 		assert.True(t, *content.Approve)
// 	})

// 	t.Run("MCP response conversion", func(t *testing.T) {
// 		response := &Response{
// 			ID:    "resp_123",
// 			Model: "gpt-4.1",
// 			Output: []OutputItem{
// 				{
// 					Type:        "mcp_list_tools",
// 					ServerLabel: "test-server",
// 					Tools: []MCPToolDefinition{
// 						{Name: "query"},
// 						{Name: "search"},
// 					},
// 				},
// 				{
// 					Type:        "mcp_call",
// 					ID:          "mcp_456",
// 					Name:        "query",
// 					ServerLabel: "test-server",
// 					Output:      "Query result: success",
// 				},
// 				{
// 					Type:        "mcp_approval_request",
// 					Name:        "dangerous_tool",
// 					ServerLabel: "test-server",
// 				},
// 			},
// 		}

// 		llmResponse, err := provider.convertResponse(response)
// 		require.NoError(t, err)

// 		require.Len(t, llmResponse.Content, 3)

// 		// Check MCP tool list content
// 		textContent1, ok := llmResponse.Content[0].(*llm.TextContent)
// 		require.True(t, ok)
// 		assert.Contains(t, textContent1.Text, "MCP server 'test-server' tools:")
// 		assert.Contains(t, textContent1.Text, "- query")
// 		assert.Contains(t, textContent1.Text, "- search")

// 		// Check MCP call result content
// 		textContent2, ok := llmResponse.Content[1].(*llm.TextContent)
// 		require.True(t, ok)
// 		assert.Contains(t, textContent2.Text, "MCP tool result: Query result: success")

// 		// Check MCP approval request content
// 		textContent3, ok := llmResponse.Content[2].(*llm.TextContent)
// 		require.True(t, ok)
// 		assert.Contains(t, textContent3.Text, "MCP approval required for tool 'dangerous_tool' on server 'test-server'")
// 	})
// }

// func TestMCPStreamingEvents(t *testing.T) {
// 	t.Run("MCP call streaming event", func(t *testing.T) {
// 		iterator := &StreamIterator{
// 			nextContentIndex: 0,
// 			eventCount:       1, // Skip message_start event
// 		}

// 		streamEvent := &StreamEvent{
// 			Response: &Response{
// 				ID:    "resp_123",
// 				Model: "gpt-4.1",
// 				Output: []OutputItem{
// 					{
// 						Type: "mcp_call",
// 						ID:   "mcp_456",
// 						Name: "query",
// 					},
// 				},
// 			},
// 		}

// 		events := iterator.convertStreamEvent(streamEvent)
// 		require.NotNil(t, events)
// 		require.Len(t, events, 1)
// 		assert.Equal(t, llm.EventTypeContentBlockStart, events[0].Type)
// 		assert.NotNil(t, events[0].Index)
// 		assert.Equal(t, 0, *events[0].Index)
// 		assert.Equal(t, llm.ContentTypeToolUse, events[0].ContentBlock.Type)
// 		assert.Equal(t, "mcp_456", events[0].ContentBlock.ID)
// 		assert.Equal(t, "query", events[0].ContentBlock.Name)
// 	})

// 	t.Run("MCP list tools streaming event", func(t *testing.T) {
// 		iterator := &StreamIterator{
// 			nextContentIndex: 0,
// 			eventCount:       1, // Skip message_start event
// 		}

// 		streamEvent := &StreamEvent{
// 			Response: &Response{
// 				ID:    "resp_123",
// 				Model: "gpt-4.1",
// 				Output: []OutputItem{
// 					{
// 						Type:        "mcp_list_tools",
// 						ServerLabel: "test-server",
// 						Tools: []MCPToolDefinition{
// 							{Name: "query"},
// 							{Name: "search"},
// 						},
// 					},
// 				},
// 			},
// 		}

// 		events := iterator.convertStreamEvent(streamEvent)
// 		require.NotNil(t, events)
// 		require.Len(t, events, 1)
// 		assert.Equal(t, llm.EventTypeContentBlockStart, events[0].Type)
// 		assert.Equal(t, 0, *events[0].Index)
// 		assert.Equal(t, llm.ContentTypeText, events[0].ContentBlock.Type)
// 		assert.Contains(t, events[0].ContentBlock.Text, "MCP server 'test-server' tools:")
// 		assert.Contains(t, events[0].ContentBlock.Text, "- query")
// 		assert.Contains(t, events[0].ContentBlock.Text, "- search")
// 	})

// 	t.Run("MCP approval request streaming event", func(t *testing.T) {
// 		iterator := &StreamIterator{
// 			nextContentIndex: 0,
// 			eventCount:       1, // Skip message_start event
// 		}

// 		streamEvent := &StreamEvent{
// 			Response: &Response{
// 				ID:    "resp_123",
// 				Model: "gpt-4.1",
// 				Output: []OutputItem{
// 					{
// 						Type:        "mcp_approval_request",
// 						Name:        "dangerous_tool",
// 						ServerLabel: "test-server",
// 					},
// 				},
// 			},
// 		}

// 		events := iterator.convertStreamEvent(streamEvent)
// 		require.NotNil(t, events)
// 		require.Len(t, events, 1)
// 		assert.Equal(t, llm.EventTypeContentBlockStart, events[0].Type)
// 		assert.NotNil(t, events[0].Index)
// 		assert.Equal(t, 0, *events[0].Index)
// 		assert.Equal(t, llm.ContentTypeText, events[0].ContentBlock.Type)
// 		assert.Contains(t, events[0].ContentBlock.Text, "MCP approval required for tool 'dangerous_tool' on server 'test-server'")
// 	})
// }

func TestMCPContentPairing(t *testing.T) {
	t.Run("pairs MCPToolUseContent with MCPToolResultContent", func(t *testing.T) {
		message := &llm.Message{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.TextContent{Text: "I'll call the MCP tool"},
				&llm.MCPToolUseContent{
					ID:         "mcp_123",
					Name:       "query_database",
					ServerName: "db-server",
					Input:      []byte(`{"query": "SELECT * FROM users"}`),
				},
				&llm.MCPToolResultContent{
					ToolUseID: "mcp_123",
					IsError:   false,
					Content: []*llm.ContentChunk{
						{Type: "text", Text: "Found 5 users"},
					},
				},
				&llm.TextContent{Text: "The query completed successfully"},
			},
		}

		encoded, err := encodeAssistantMessage(message)
		require.NoError(t, err)
		require.Len(t, encoded, 3) // Text + Combined MCP + Text

		// First item should be text
		require.NotNil(t, encoded[0].OfOutputMessage)

		// Second item should be the combined MCP call with both use and result
		require.NotNil(t, encoded[1].OfMcpCall)
		mcpCall := encoded[1].OfMcpCall
		require.Equal(t, "mcp_123", mcpCall.ID)
		require.Equal(t, "query_database", mcpCall.Name)
		require.Equal(t, "db-server", mcpCall.ServerLabel)
		require.Equal(t, `{"query": "SELECT * FROM users"}`, mcpCall.Arguments)
		require.Equal(t, openai.String("Found 5 users"), mcpCall.Output)

		// Third item should be text
		require.NotNil(t, encoded[2].OfOutputMessage)
	})

	t.Run("pairs MCPToolUseContent with error MCPToolResultContent", func(t *testing.T) {
		message := &llm.Message{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.MCPToolUseContent{
					ID:         "mcp_456",
					Name:       "risky_operation",
					ServerName: "unsafe-server",
					Input:      []byte(`{"action": "delete_all"}`),
				},
				&llm.MCPToolResultContent{
					ToolUseID: "mcp_456",
					IsError:   true,
					Content: []*llm.ContentChunk{
						{Type: "text", Text: "Operation failed: Permission denied"},
					},
				},
			},
		}

		encoded, err := encodeAssistantMessage(message)
		require.NoError(t, err)
		require.Len(t, encoded, 1) // Only the combined MCP call

		require.NotNil(t, encoded[0].OfMcpCall)
		mcpCall := encoded[0].OfMcpCall
		require.Equal(t, "mcp_456", mcpCall.ID)
		require.Equal(t, "risky_operation", mcpCall.Name)
		require.Equal(t, "unsafe-server", mcpCall.ServerLabel)
		require.Equal(t, openai.String("Operation failed: Permission denied"), mcpCall.Error)
	})

	t.Run("handles MCPToolUseContent without corresponding result", func(t *testing.T) {
		message := &llm.Message{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.MCPToolUseContent{
					ID:         "mcp_789",
					Name:       "async_task",
					ServerName: "worker-server",
					Input:      []byte(`{"task": "background_job"}`),
				},
			},
		}

		encoded, err := encodeAssistantMessage(message)
		require.NoError(t, err)
		require.Len(t, encoded, 1)

		require.NotNil(t, encoded[0].OfMcpCall)
		mcpCall := encoded[0].OfMcpCall
		require.Equal(t, "mcp_789", mcpCall.ID)
		require.Equal(t, "async_task", mcpCall.Name)
		require.Equal(t, "worker-server", mcpCall.ServerLabel)
		require.Equal(t, `{"task": "background_job"}`, mcpCall.Arguments)
	})

	t.Run("handles multiple chunks in MCPToolResultContent", func(t *testing.T) {
		message := &llm.Message{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.MCPToolUseContent{
					ID:         "mcp_multi",
					Name:       "multi_output",
					ServerName: "data-server",
					Input:      []byte(`{"format": "detailed"}`),
				},
				&llm.MCPToolResultContent{
					ToolUseID: "mcp_multi",
					IsError:   false,
					Content: []*llm.ContentChunk{
						{Type: "text", Text: "First part of result"},
						{Type: "text", Text: "Second part of result"},
						{Type: "text", Text: "Final conclusion"},
					},
				},
			},
		}

		encoded, err := encodeAssistantMessage(message)
		require.NoError(t, err)
		require.Len(t, encoded, 1)

		require.NotNil(t, encoded[0].OfMcpCall)
		mcpCall := encoded[0].OfMcpCall
		expectedOutput := "First part of result\nSecond part of result\nFinal conclusion"
		require.Equal(t, openai.String(expectedOutput), mcpCall.Output)
	})
}
