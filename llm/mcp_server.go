package llm

// MCPToolApprovalFilter is used to configure the approval filter for MCP tools.
// The Always and Never fields should contain the names of tools whose calls
// should have customized approvals.
type MCPToolApprovalFilter struct {
	Always []string `json:"always,omitempty"`
	Never  []string `json:"never,omitempty"`
}

// MCPToolConfiguration represents the configuration for MCP tools.
// Generally corresponds to Anthropic's tool_configuration field:
// https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector#mcp-server-configuration
// OpenAI Remote MCP uses approval mode/filter:
// https://platform.openai.com/docs/guides/tools-remote-mcp
type MCPToolConfiguration struct {
	Enabled      bool     `json:"enabled"`
	AllowedTools []string `json:"allowed_tools,omitempty"`

	// OpenAI MCP server only, for the Responses API
	ApprovalMode   string                 `json:"approval_mode,omitempty"`
	ApprovalFilter *MCPToolApprovalFilter `json:"approval_filter,omitempty"`
}

// MCPOAuthConfig represents OAuth 2.0 configuration for MCP servers
type MCPOAuthConfig struct {
	ClientID     string            `json:"client_id"`
	ClientSecret string            `json:"client_secret,omitempty"`
	RedirectURI  string            `json:"redirect_uri"`
	Scopes       []string          `json:"scopes,omitempty"`
	PKCEEnabled  bool              `json:"pkce_enabled,omitempty"`
	TokenStore   *MCPTokenStore    `json:"token_store,omitempty"`
	ExtraParams  map[string]string `json:"extra_params,omitempty"`
}

// MCPTokenStore represents token storage configuration
type MCPTokenStore struct {
	Type string `json:"type"`           // "memory", "file", "keychain"
	Path string `json:"path,omitempty"` // For file storage
}

// MCPServerConfig is used to configure an MCP server.
// Corresponds to this Anthropic feature:
// https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector#using-the-mcp-connector-in-the-messages-api
// And OpenAI's Remote MCP feature:
// https://platform.openai.com/docs/guides/tools-remote-mcp#page-top
type MCPServerConfig struct {
	Type               string                `json:"type"`
	Command            string                `json:"command,omitempty"`
	URL                string                `json:"url,omitempty"`
	Name               string                `json:"name,omitempty"`
	AuthorizationToken string                `json:"authorization_token,omitempty"`
	OAuth              *MCPOAuthConfig       `json:"oauth,omitempty"`
	ToolConfiguration  *MCPToolConfiguration `json:"tool_configuration,omitempty"`
	Headers            map[string]string     `json:"headers,omitempty"`
}
