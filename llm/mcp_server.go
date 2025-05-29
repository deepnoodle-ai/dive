package llm

type MCPToolConfiguration struct {
	Enabled      bool     `json:"enabled"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
}

// MCPApprovalRequirement represents the approval requirements for MCP tools
type MCPApprovalRequirement struct {
	Never *MCPNeverApproval `json:"never,omitempty"`
}

// MCPNeverApproval specifies tools that never require approval
type MCPNeverApproval struct {
	ToolNames []string `json:"tool_names"`
}

// MCPServerConfig is used to configure an MCP server.
// Corresponds to this Anthropic feature:
// https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector#using-the-mcp-connector-in-the-messages-api
// And OpenAI's Remote MCP feature:
// https://platform.openai.com/docs/guides/remote-mcp
type MCPServerConfig struct {
	Type               string                `json:"type"`
	URL                string                `json:"url"`
	Name               string                `json:"name,omitempty"`
	AuthorizationToken string                `json:"authorization_token,omitempty"`
	ToolConfiguration  *MCPToolConfiguration `json:"tool_configuration,omitempty"`
	Headers            map[string]string     `json:"headers,omitempty"`

	// OpenAI-specific approval requirement options
	// Can be "always", "never", or a complex object specifying tool-specific approvals
	ApprovalRequirement interface{} `json:"approval_requirement,omitempty"`
}
