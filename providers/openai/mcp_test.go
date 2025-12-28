package openai

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go"
)

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
		assert.NoError(t, err)
		assert.Len(t, encoded, 3) // Text + Combined MCP + Text

		// First item should be text
		assert.NotNil(t, encoded[0].OfOutputMessage)

		// Second item should be the combined MCP call with both use and result
		assert.NotNil(t, encoded[1].OfMcpCall)
		mcpCall := encoded[1].OfMcpCall
		assert.Equal(t, "mcp_123", mcpCall.ID)
		assert.Equal(t, "query_database", mcpCall.Name)
		assert.Equal(t, "db-server", mcpCall.ServerLabel)
		assert.Equal(t, `{"query": "SELECT * FROM users"}`, mcpCall.Arguments)
		assert.Equal(t, openai.String("Found 5 users"), mcpCall.Output)

		// Third item should be text
		assert.NotNil(t, encoded[2].OfOutputMessage)
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
		assert.NoError(t, err)
		assert.Len(t, encoded, 1) // Only the combined MCP call

		assert.NotNil(t, encoded[0].OfMcpCall)
		mcpCall := encoded[0].OfMcpCall
		assert.Equal(t, "mcp_456", mcpCall.ID)
		assert.Equal(t, "risky_operation", mcpCall.Name)
		assert.Equal(t, "unsafe-server", mcpCall.ServerLabel)
		assert.Equal(t, openai.String("Operation failed: Permission denied"), mcpCall.Error)
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
		assert.NoError(t, err)
		assert.Len(t, encoded, 1)

		assert.NotNil(t, encoded[0].OfMcpCall)
		mcpCall := encoded[0].OfMcpCall
		assert.Equal(t, "mcp_789", mcpCall.ID)
		assert.Equal(t, "async_task", mcpCall.Name)
		assert.Equal(t, "worker-server", mcpCall.ServerLabel)
		assert.Equal(t, `{"task": "background_job"}`, mcpCall.Arguments)
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
		assert.NoError(t, err)
		assert.Len(t, encoded, 1)

		assert.NotNil(t, encoded[0].OfMcpCall)
		mcpCall := encoded[0].OfMcpCall
		expectedOutput := "First part of result\nSecond part of result\nFinal conclusion"
		assert.Equal(t, openai.String(expectedOutput), mcpCall.Output)
	})
}
