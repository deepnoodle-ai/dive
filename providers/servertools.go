package providers

import "github.com/deepnoodle-ai/dive/llm"

// IsServerToolContent reports whether content is a tool call or tool result
// that a provider ran on its own servers rather than one Dive executed:
// Anthropic server tools (server_tool_use, web search, web fetch, tool search
// and code execution results), MCP connector calls and results, and OpenAI's
// web search and MCP calls, which decode to the same types, plus OpenAI's MCP
// tool listing.
//
// A provider encoder uses it to leave out history it cannot replay. Such
// blocks are only valid for the provider that ran the tools; sent to another
// one they are rejected, or their foreign IDs are. The assistant text around
// them still carries what the model concluded, so skipping them loses no
// answer. A provider that can replay its own server tool blocks (Anthropic,
// OpenAI for its web search and MCP calls) must tell its own blocks apart
// before skipping the rest.
func IsServerToolContent(content llm.Content) bool {
	switch content.(type) {
	case *llm.ServerToolUseContent, *llm.ServerToolResultContent,
		*llm.WebSearchToolResultContent, *llm.CodeExecutionToolResultContent,
		*llm.BashCodeExecutionToolResultContent, *llm.TextEditorCodeExecutionToolResultContent,
		*llm.MCPToolUseContent, *llm.MCPToolResultContent,
		*llm.MCPListToolsContent:
		return true
	}
	return false
}
