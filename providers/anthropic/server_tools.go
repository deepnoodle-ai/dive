package anthropic

import (
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

// Anthropic starts the ID of every server tool call with one of these
// prefixes: "srvtoolu_" for server tools such as web search and web fetch,
// and "mcptoolu_" for MCP connector calls. Results carry the same ID as
// their tool_use_id.
const (
	serverToolIDPrefix = "srvtoolu_"
	mcpToolIDPrefix    = "mcptoolu_"
)

// isForeignServerToolContent reports whether content is a server tool call or
// result that another provider ran, such as an OpenAI web_search_call or
// mcp_call. Anthropic cannot replay those, so the encoder leaves them out.
func isForeignServerToolContent(content llm.Content) bool {
	return providers.IsServerToolContent(content) && !isAnthropicServerToolContent(content)
}

// isAnthropicServerToolContent reports whether server tool content came from
// an Anthropic response, judged by the prefix of its tool use ID.
func isAnthropicServerToolContent(content llm.Content) bool {
	id := serverToolUseID(content)
	return strings.HasPrefix(id, serverToolIDPrefix) || strings.HasPrefix(id, mcpToolIDPrefix)
}

// serverToolUseID returns the tool use ID of a server tool call, or of the
// call a server tool result answers.
func serverToolUseID(content llm.Content) string {
	switch c := content.(type) {
	case *llm.ServerToolUseContent:
		return c.ID
	case *llm.MCPToolUseContent:
		return c.ID
	case *llm.ServerToolResultContent:
		return c.ToolUseID
	case *llm.WebSearchToolResultContent:
		return c.ToolUseID
	case *llm.CodeExecutionToolResultContent:
		return c.ToolUseID
	case *llm.BashCodeExecutionToolResultContent:
		return c.ToolUseID
	case *llm.TextEditorCodeExecutionToolResultContent:
		return c.ToolUseID
	case *llm.MCPToolResultContent:
		return c.ToolUseID
	}
	return ""
}
