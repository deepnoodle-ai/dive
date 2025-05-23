package llm

type MCPToolConfiguration struct {
	Enabled      bool     `json:"enabled"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
}

// MCPServerConfig is used to configure an MCP server.
// Corresponds to this Anthropic feature:
// https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector#using-the-mcp-connector-in-the-messages-api
type MCPServerConfig struct {
	Type               string                `json:"type"`
	URL                string                `json:"url"`
	Name               string                `json:"name,omitempty"`
	AuthorizationToken string                `json:"authorization_token,omitempty"`
	ToolConfiguration  *MCPToolConfiguration `json:"tool_configuration,omitempty"`
}
