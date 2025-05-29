package openairesponses

import (
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPTool_Interfaces(t *testing.T) {
	tool := NewMCPTool(MCPToolOptions{
		ServerLabel: "test",
		ServerURL:   "https://example.com/mcp",
	})

	// Verify it implements the required interfaces
	var _ llm.Tool = tool
	var _ llm.ToolConfiguration = tool
}

func TestMCPTool_Basic(t *testing.T) {
	tool := NewMCPTool(MCPToolOptions{
		ServerLabel: "deepwiki",
		ServerURL:   "https://mcp.deepwiki.com/mcp",
	})

	assert.Equal(t, "mcp_deepwiki", tool.Name())
	assert.Contains(t, tool.Description(), "Remote MCP")
	assert.Equal(t, schema.Schema{}, tool.Schema()) // Empty for server-side tools
}

func TestMCPTool_ToolConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		opts     MCPToolOptions
		expected map[string]any
	}{
		{
			name: "minimal configuration",
			opts: MCPToolOptions{
				ServerLabel: "test",
				ServerURL:   "https://example.com/mcp",
			},
			expected: map[string]any{
				"type":         "mcp",
				"server_label": "test",
				"server_url":   "https://example.com/mcp",
			},
		},
		{
			name: "never require approval",
			opts: MCPToolOptions{
				ServerLabel:     "deepwiki",
				ServerURL:       "https://mcp.deepwiki.com/mcp",
				RequireApproval: "never",
				AllowedTools:    []string{"ask_question"},
			},
			expected: map[string]any{
				"type":             "mcp",
				"server_label":     "deepwiki",
				"server_url":       "https://mcp.deepwiki.com/mcp",
				"require_approval": "never",
				"allowed_tools":    []string{"ask_question"},
			},
		},
		{
			name: "complex approval with authentication",
			opts: MCPToolOptions{
				ServerLabel: "stripe",
				ServerURL:   "https://mcp.stripe.com",
				RequireApproval: map[string]interface{}{
					"never": map[string]interface{}{
						"tool_names": []string{"list_products", "get_balance"},
					},
				},
				Headers: map[string]string{
					"Authorization": "Bearer sk_test_example",
				},
			},
			expected: map[string]any{
				"type":         "mcp",
				"server_label": "stripe",
				"server_url":   "https://mcp.stripe.com",
				"require_approval": map[string]interface{}{
					"never": map[string]interface{}{
						"tool_names": []string{"list_products", "get_balance"},
					},
				},
				"headers": map[string]string{
					"Authorization": "Bearer sk_test_example",
				},
			},
		},
		{
			name: "filtered tools",
			opts: MCPToolOptions{
				ServerLabel:  "hubspot",
				ServerURL:    "https://mcp.hubspot.com",
				AllowedTools: []string{"get_contacts", "create_contact"},
			},
			expected: map[string]any{
				"type":          "mcp",
				"server_label":  "hubspot",
				"server_url":    "https://mcp.hubspot.com",
				"allowed_tools": []string{"get_contacts", "create_contact"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewMCPTool(tt.opts)
			config := tool.ToolConfiguration("openai-responses")

			for key, expectedValue := range tt.expected {
				actualValue, exists := config[key]
				require.True(t, exists, "Expected key %s to exist in config", key)
				assert.Equal(t, expectedValue, actualValue, "Mismatch for key %s", key)
			}

			// Verify no unexpected keys
			assert.Len(t, config, len(tt.expected), "Config should only contain expected keys")
		})
	}
}
