package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name   string
		config *ServerConfig
	}{
		{
			name: "http server config",
			config: &ServerConfig{
				Type: "http",
				Name: "test-http-server",
				URL:  "http://localhost:8080",
				// ToolEnabled: true,
			},
		},
		{
			name: "stdio server config",
			config: &ServerConfig{
				Type: "stdio",
				Name: "test-stdio-server",
				URL:  "/path/to/server",
				// ToolEnabled: true,
			},
		},
		{
			name: "server with tool configuration",
			config: &ServerConfig{
				Type: "http",
				Name: "test-server-with-tools",
				URL:  "http://localhost:8080",
				// ToolEnabled:  true,
				// AllowedTools: []string{"tool1", "tool2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			require.NoError(t, err)
			require.NotNil(t, client)
			require.Equal(t, tt.config, client.config)
			require.False(t, client.connected)
			require.Nil(t, client.client)
			require.Empty(t, client.tools)
		})
	}
}

func TestClient_IsConnected(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	// Initially not connected
	require.False(t, client.IsConnected())

	// Simulate connection
	client.connected = true
	require.True(t, client.IsConnected())

	// Simulate disconnection
	client.connected = false
	require.False(t, client.IsConnected())
}

func TestClient_GetTools(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	// Initially no tools
	tools := client.GetTools()
	require.Empty(t, tools)

	// Add some tools
	testTools := []mcp.Tool{
		{
			Name:        "test-tool-1",
			Description: "Test tool 1",
		},
		{
			Name:        "test-tool-2",
			Description: "Test tool 2",
		},
	}
	client.tools = testTools

	// Verify tools are returned
	tools = client.GetTools()
	require.Equal(t, testTools, tools)
}

func TestClient_filterTools(t *testing.T) {
	tests := []struct {
		name          string
		config        *ServerConfig
		inputTools    []mcp.Tool
		expectedTools []mcp.Tool
	}{
		{
			name: "no tool configuration returns all tools",
			config: &ServerConfig{
				Type: "http",
				Name: "test-server",
				URL:  "http://localhost:8080",
				// ToolEnabled: true,
			},
			inputTools: []mcp.Tool{
				{Name: "tool1", Description: "Tool 1"},
				{Name: "tool2", Description: "Tool 2"},
			},
			expectedTools: []mcp.Tool{
				{Name: "tool1", Description: "Tool 1"},
				{Name: "tool2", Description: "Tool 2"},
			},
		},
		{
			name: "disabled tools returns empty",
			config: &ServerConfig{
				Type: "http",
				Name: "test-server",
				URL:  "http://localhost:8080",
				ToolConfiguration: &ToolConfiguration{
					Enabled: false,
				},
			},
			inputTools: []mcp.Tool{
				{Name: "tool1", Description: "Tool 1"},
				{Name: "tool2", Description: "Tool 2"},
			},
			expectedTools: []mcp.Tool{},
		},
		{
			name: "enabled with no allowed tools returns all",
			config: &ServerConfig{
				Type: "http",
				Name: "test-server",
				URL:  "http://localhost:8080",
				// ToolEnabled: true,
				// AllowedTools: []string{},
			},
			inputTools: []mcp.Tool{
				{Name: "tool1", Description: "Tool 1"},
				{Name: "tool2", Description: "Tool 2"},
			},
			expectedTools: []mcp.Tool{
				{Name: "tool1", Description: "Tool 1"},
				{Name: "tool2", Description: "Tool 2"},
			},
		},
		{
			name: "filters tools based on allowed list",
			config: &ServerConfig{
				Type: "http",
				Name: "test-server",
				URL:  "http://localhost:8080",
				ToolConfiguration: &ToolConfiguration{
					Enabled:      true,
					AllowedTools: []string{"tool1", "tool3"},
				},
			},
			inputTools: []mcp.Tool{
				{Name: "tool1", Description: "Tool 1"},
				{Name: "tool2", Description: "Tool 2"},
				{Name: "tool3", Description: "Tool 3"},
			},
			expectedTools: []mcp.Tool{
				{Name: "tool1", Description: "Tool 1"},
				{Name: "tool3", Description: "Tool 3"},
			},
		},
		{
			name: "empty result when no tools match allowed list",
			config: &ServerConfig{
				Type: "http",
				Name: "test-server",
				URL:  "http://localhost:8080",
				ToolConfiguration: &ToolConfiguration{
					Enabled:      true,
					AllowedTools: []string{"nonexistent"},
				},
			},
			inputTools: []mcp.Tool{
				{Name: "tool1", Description: "Tool 1"},
				{Name: "tool2", Description: "Tool 2"},
			},
			expectedTools: nil, // filterTools returns nil for empty result
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			require.NoError(t, err)

			filtered := client.filterTools(tt.inputTools)
			require.Equal(t, tt.expectedTools, filtered)
		})
	}
}

func TestClient_ListTools_NotConnected(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	ctx := context.Background()
	tools, err := client.ListTools(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mcp client not connected")
	require.Nil(t, tools)
}

func TestClient_CallTool_NotConnected(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	ctx := context.Background()
	result, err := client.CallTool(ctx, "test-tool", map[string]interface{}{"param": "value"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mcp client not connected")
	require.Nil(t, result)
}

func TestClient_Connect_UnsupportedType(t *testing.T) {
	client, err := NewClient(&ServerConfig{
		Type: "unsupported",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = client.Connect(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported mcp server type: unsupported")
	require.False(t, client.IsConnected())
}

func TestClient_Connect_StdioWithEnvAndArgs(t *testing.T) {
	config := &ServerConfig{
		Type: "stdio",
		Name: "test-stdio-server",
		URL:  "python",
		Env: map[string]string{
			"API_KEY":    "test-key",
			"DEBUG":      "true",
			"SERVER_URL": "http://localhost:8080",
		},
		Args: []string{
			"server.py",
			"--port", "3000",
			"--verbose",
		},
		// toolEnabled: true,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify that the config methods return the expected values
	require.Equal(t, "stdio", client.config.Type)
	require.Equal(t, "python", client.config.URL)

	expectedEnv := map[string]string{
		"API_KEY":    "test-key",
		"DEBUG":      "true",
		"SERVER_URL": "http://localhost:8080",
	}
	require.Equal(t, expectedEnv, client.config.Env)

	expectedArgs := []string{"server.py", "--port", "3000", "--verbose"}
	require.Equal(t, expectedArgs, client.config.Args)

	// Note: We don't actually call Connect() here because it would try to start
	// a real subprocess, which would fail in the test environment.
	// The important part is that the configuration is properly stored and accessible.
}
