package providers

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestIsServerToolContent(t *testing.T) {
	serverSide := []llm.Content{
		&llm.ServerToolUseContent{ID: "srvtoolu_1", Name: "web_search"},
		&llm.ServerToolResultContent{BlockType: llm.ContentTypeWebFetchToolResult, ToolUseID: "srvtoolu_1"},
		&llm.WebSearchToolResultContent{ToolUseID: "srvtoolu_1"},
		&llm.CodeExecutionToolResultContent{ToolUseID: "srvtoolu_1"},
		&llm.BashCodeExecutionToolResultContent{ToolUseID: "srvtoolu_1"},
		&llm.TextEditorCodeExecutionToolResultContent{ToolUseID: "srvtoolu_1"},
		&llm.MCPToolUseContent{ID: "mcptoolu_1"},
		&llm.MCPToolResultContent{ToolUseID: "mcptoolu_1"},
	}
	for _, c := range serverSide {
		assert.True(t, IsServerToolContent(c), "%T", c)
	}
	clientSide := []llm.Content{
		&llm.TextContent{Text: "hi"},
		&llm.ToolUseContent{ID: "toolu_1"},
		&llm.ToolResultContent{ToolUseID: "toolu_1"},
		&llm.ThinkingContent{Thinking: "hmm"},
	}
	for _, c := range clientSide {
		assert.False(t, IsServerToolContent(c), "%T", c)
	}
}
