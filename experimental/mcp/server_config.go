package mcp

// ToolApprovalFilter is used to configure the approval filter for MCP tools.
// The Always and Never fields should contain the names of tools whose calls
// should have customized approvals.
type ToolApprovalFilter struct {
	Always []string `json:"always,omitempty"`
	Never  []string `json:"never,omitempty"`
}

// ToolConfiguration customizes tool behavior for MCP servers.
type ToolConfiguration struct {
	Enabled        bool                `json:"enabled"`
	AllowedTools   []string            `json:"allowed_tools,omitempty"`
	ApprovalMode   string              `json:"approval_mode,omitempty"`
	ApprovalFilter *ToolApprovalFilter `json:"approval_filter,omitempty"`
}

// OAuthConfig represents OAuth 2.0 configuration for MCP servers
type OAuthConfig struct {
	ClientID     string            `json:"client_id"`
	ClientSecret string            `json:"client_secret,omitempty"`
	RedirectURI  string            `json:"redirect_uri"`
	Scopes       []string          `json:"scopes,omitempty"`
	PKCEEnabled  bool              `json:"pkce_enabled,omitempty"`
	TokenStore   *TokenStore       `json:"token_store,omitempty"`
	ExtraParams  map[string]string `json:"extra_params,omitempty"`
}

// TokenStore represents token storage configuration
type TokenStore struct {
	Type string `json:"type"`           // "memory", "file", "keychain"
	Path string `json:"path,omitempty"` // For file storage
}

// ServerConfig is used to configure an MCP server.
// Corresponds to this Anthropic feature:
// https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector#using-the-mcp-connector-in-the-messages-api
// And OpenAI's Remote MCP feature:
// https://platform.openai.com/docs/guides/tools-remote-mcp#page-top
type ServerConfig struct {
	Type               string             `json:"type"`
	Command            string             `json:"command,omitempty"`
	URL                string             `json:"url,omitempty"`
	Name               string             `json:"name,omitempty"`
	Env                map[string]string  `json:"env,omitempty"`
	Args               []string           `json:"args,omitempty"`
	AuthorizationToken string             `json:"authorization_token,omitempty"`
	OAuth              *OAuthConfig       `json:"oauth,omitempty"`
	ToolConfiguration  *ToolConfiguration `json:"tool_configuration,omitempty"`
	Headers            map[string]string  `json:"headers,omitempty"`
}

// IsOAuthEnabled returns true if OAuth is configured for this server
func (s *ServerConfig) IsOAuthEnabled() bool {
	return s.OAuth != nil
}

// IsToolEnabled returns true if tools are enabled for this server
func (s *ServerConfig) IsToolEnabled() bool {
	if s.ToolConfiguration == nil {
		return true // default to enabled
	}
	return s.ToolConfiguration.Enabled
}

// GetAllowedTools returns the list of allowed tools, or nil if all tools are allowed
func (s *ServerConfig) GetAllowedTools() []string {
	if s.ToolConfiguration == nil {
		return nil // all tools allowed
	}
	return s.ToolConfiguration.AllowedTools
}
