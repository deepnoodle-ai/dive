package llm

type MCPToolConfiguration struct {
	Enabled      bool     `json:"enabled"`
	AllowedTools []string `json:"allowed_tools,omitempty"`
}

// MCPApprovalRequirement represents the approval requirements for MCP tools
// type MCPApprovalRequirement struct {
// 	Never *MCPNeverApproval `json:"never,omitempty"`
// }

// // MCPNeverApproval specifies tools that never require approval
// type MCPNeverApproval struct {
// 	ToolNames []string `json:"tool_names"`
// }

// MCPServerConfig is used to configure an MCP server.
// Corresponds to this Anthropic feature:
// https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector#using-the-mcp-connector-in-the-messages-api
// And OpenAI's Remote MCP feature:
// https://platform.openai.com/docs/guides/tools-remote-mcp#page-top
type MCPServerConfig struct {
	Type               string                 `json:"type"`
	URL                string                 `json:"url"`
	Name               string                 `json:"name,omitempty"`
	AuthorizationToken string                 `json:"authorization_token,omitempty"`
	ToolConfiguration  *MCPToolConfiguration  `json:"tool_configuration,omitempty"`
	Headers            map[string]string      `json:"headers,omitempty"`
	ToolApproval       string                 `json:"tool_approval,omitempty"`
	ToolApprovalFilter *MCPToolApprovalFilter `json:"tool_approval_filter,omitempty"`
}

// MCPToolApprovalFilter is used to configure the approval filter for MCP tools.
// The Always and Never fields should contain the names of tools whose calls
// should have customized approvals.
type MCPToolApprovalFilter struct {
	Always []string `json:"always,omitempty"`
	Never  []string `json:"never,omitempty"`
}
