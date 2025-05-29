package openairesponses

import (
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
)

var (
	_ llm.Tool              = &MCPTool{}
	_ llm.ToolConfiguration = &MCPTool{}
)

/* A tool definition must be added in the request that looks like this:
   "tools": [{
       "type": "mcp",
       "server_label": "deepwiki",
       "server_url": "https://mcp.deepwiki.com/mcp",
       "require_approval": "never",
       "allowed_tools": ["ask_question"],
       "headers": {
           "Authorization": "Bearer sk_example"
       }
   }]
*/

// MCPToolOptions are the options used to configure an MCPTool.
type MCPToolOptions struct {
	ServerLabel     string            `json:"server_label"`
	ServerURL       string            `json:"server_url"`
	AllowedTools    []string          `json:"allowed_tools,omitempty"`
	RequireApproval interface{}       `json:"require_approval,omitempty"` // "always", "never", or complex object
	Headers         map[string]string `json:"headers,omitempty"`
}

// NewMCPTool creates a new MCPTool with the given options.
func NewMCPTool(opts MCPToolOptions) *MCPTool {
	return &MCPTool{
		serverLabel:     opts.ServerLabel,
		serverURL:       opts.ServerURL,
		allowedTools:    opts.AllowedTools,
		requireApproval: opts.RequireApproval,
		headers:         opts.Headers,
	}
}

// MCPTool is a tool that allows models to connect to remote MCP servers. This is
// provided by OpenAI as a server-side tool in the Responses API. Learn more:
// https://platform.openai.com/docs/guides/remote-mcp
type MCPTool struct {
	serverLabel     string
	serverURL       string
	allowedTools    []string
	requireApproval interface{}
	headers         map[string]string
}

func (t *MCPTool) Name() string {
	return "mcp_" + t.serverLabel
}

func (t *MCPTool) Description() string {
	return "Uses OpenAI's Remote MCP feature to connect to MCP servers for extended functionality."
}

func (t *MCPTool) Schema() schema.Schema {
	return schema.Schema{} // Empty for server-side tools
}

func (t *MCPTool) ToolConfiguration(providerName string) map[string]any {
	config := map[string]any{
		"type":         "mcp",
		"server_label": t.serverLabel,
		"server_url":   t.serverURL,
	}

	if len(t.allowedTools) > 0 {
		config["allowed_tools"] = t.allowedTools
	}
	if t.requireApproval != nil {
		config["require_approval"] = t.requireApproval
	}
	if len(t.headers) > 0 {
		config["headers"] = t.headers
	}

	return config
}
