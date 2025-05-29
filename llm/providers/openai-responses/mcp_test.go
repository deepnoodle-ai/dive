package openairesponses

import (
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPIntegration(t *testing.T) {
	provider := New(WithModel("gpt-4.1"))

	t.Run("MCP server configuration from llm.Config", func(t *testing.T) {
		config := &llm.Config{
			Messages: []*llm.Message{
				llm.NewUserTextMessage("Test message"),
			},
			MCPServers: []llm.MCPServerConfig{
				{
					Type: "url",
					Name: "test-server",
					URL:  "https://example.com/mcp",
					ToolConfiguration: &llm.MCPToolConfiguration{
						Enabled:      true,
						AllowedTools: []string{"query", "search"},
					},
					AuthorizationToken: "test-token",
				},
			},
		}

		request, err := provider.buildRequest(config)
		require.NoError(t, err)

		// Check that MCP server was added to tools
		var mcpTool *Tool
		for _, tool := range request.Tools {
			if tool.Type == "mcp" && tool.ServerLabel == "test-server" {
				mcpTool = &tool
				break
			}
		}

		require.NotNil(t, mcpTool, "MCP tool should be present")
		assert.Equal(t, "test-server", mcpTool.ServerLabel)
		assert.Equal(t, "https://example.com/mcp", mcpTool.ServerURL)
		assert.Equal(t, []string{"query", "search"}, mcpTool.AllowedTools)
		assert.Equal(t, "always", mcpTool.RequireApproval) // Default value
		assert.Equal(t, "Bearer test-token", mcpTool.Headers["Authorization"])
	})

	t.Run("MCP approval response handling", func(t *testing.T) {
		messages := []*llm.Message{
			llm.NewUserTextMessage("Regular message"),
			{
				Role: llm.User,
				Content: []llm.Content{
					&llm.TextContent{
						Text: "MCP_APPROVAL_RESPONSE:mcpr_123:true",
					},
				},
			},
		}

		input, err := provider.convertMessagesToInput(messages)
		require.NoError(t, err)

		inputMessages, ok := input.([]InputMessage)
		require.True(t, ok, "Input should be converted to message array when multiple messages")
		require.Len(t, inputMessages, 2)
		require.Len(t, inputMessages[1].Content, 1)

		content := inputMessages[1].Content[0]
		assert.Equal(t, "mcp_approval_response", content.Type)
		assert.Equal(t, "mcpr_123", content.ApprovalRequestID)
		assert.NotNil(t, content.Approve)
		assert.True(t, *content.Approve)
	})

	t.Run("MCP response conversion", func(t *testing.T) {
		response := &Response{
			ID:    "resp_123",
			Model: "gpt-4.1",
			Output: []OutputItem{
				{
					Type:        "mcp_list_tools",
					ServerLabel: "test-server",
					Tools: []MCPToolDefinition{
						{Name: "query"},
						{Name: "search"},
					},
				},
				{
					Type:        "mcp_call",
					ID:          "mcp_456",
					Name:        "query",
					ServerLabel: "test-server",
					Output:      "Query result: success",
				},
				{
					Type:        "mcp_approval_request",
					Name:        "dangerous_tool",
					ServerLabel: "test-server",
				},
			},
		}

		llmResponse, err := provider.convertResponse(response)
		require.NoError(t, err)

		require.Len(t, llmResponse.Content, 3)

		// Check MCP tool list content
		textContent1, ok := llmResponse.Content[0].(*llm.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent1.Text, "MCP server 'test-server' tools:")
		assert.Contains(t, textContent1.Text, "- query")
		assert.Contains(t, textContent1.Text, "- search")

		// Check MCP call result content
		textContent2, ok := llmResponse.Content[1].(*llm.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent2.Text, "MCP tool result: Query result: success")

		// Check MCP approval request content
		textContent3, ok := llmResponse.Content[2].(*llm.TextContent)
		require.True(t, ok)
		assert.Contains(t, textContent3.Text, "MCP approval required for tool 'dangerous_tool' on server 'test-server'")
	})
}

func TestMCPStreamingEvents(t *testing.T) {
	t.Run("MCP call streaming event", func(t *testing.T) {
		iterator := &StreamIterator{
			nextContentIndex: 0,
			eventCount:       1, // Skip message_start event
		}

		streamEvent := &StreamEvent{
			Response: &Response{
				ID:    "resp_123",
				Model: "gpt-4.1",
				Output: []OutputItem{
					{
						Type: "mcp_call",
						ID:   "mcp_456",
						Name: "query",
					},
				},
			},
		}

		event := iterator.convertStreamEvent(streamEvent)
		require.NotNil(t, event)
		assert.Equal(t, llm.EventTypeContentBlockStart, event.Type)
		assert.NotNil(t, event.Index)
		assert.Equal(t, 0, *event.Index)
		assert.Equal(t, llm.ContentTypeToolUse, event.ContentBlock.Type)
		assert.Equal(t, "mcp_456", event.ContentBlock.ID)
		assert.Equal(t, "query", event.ContentBlock.Name)
	})

	t.Run("MCP list tools streaming event", func(t *testing.T) {
		iterator := &StreamIterator{
			nextContentIndex: 0,
			eventCount:       1, // Skip message_start event
		}

		streamEvent := &StreamEvent{
			Response: &Response{
				ID:    "resp_123",
				Model: "gpt-4.1",
				Output: []OutputItem{
					{
						Type:        "mcp_list_tools",
						ServerLabel: "test-server",
						Tools: []MCPToolDefinition{
							{Name: "query"},
							{Name: "search"},
						},
					},
				},
			},
		}

		event := iterator.convertStreamEvent(streamEvent)
		require.NotNil(t, event)
		assert.Equal(t, llm.EventTypeContentBlockStart, event.Type)
		assert.NotNil(t, event.Index)
		assert.Equal(t, 0, *event.Index)
		assert.Equal(t, llm.ContentTypeText, event.ContentBlock.Type)
		assert.Contains(t, event.ContentBlock.Text, "MCP server 'test-server' tools:")
		assert.Contains(t, event.ContentBlock.Text, "- query")
		assert.Contains(t, event.ContentBlock.Text, "- search")
	})

	t.Run("MCP approval request streaming event", func(t *testing.T) {
		iterator := &StreamIterator{
			nextContentIndex: 0,
			eventCount:       1, // Skip message_start event
		}

		streamEvent := &StreamEvent{
			Response: &Response{
				ID:    "resp_123",
				Model: "gpt-4.1",
				Output: []OutputItem{
					{
						Type:        "mcp_approval_request",
						Name:        "dangerous_tool",
						ServerLabel: "test-server",
					},
				},
			},
		}

		event := iterator.convertStreamEvent(streamEvent)
		require.NotNil(t, event)
		assert.Equal(t, llm.EventTypeContentBlockStart, event.Type)
		assert.NotNil(t, event.Index)
		assert.Equal(t, 0, *event.Index)
		assert.Equal(t, llm.ContentTypeText, event.ContentBlock.Type)
		assert.Contains(t, event.ContentBlock.Text, "MCP approval required for tool 'dangerous_tool' on server 'test-server'")
	})
}
