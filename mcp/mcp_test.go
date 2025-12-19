package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestMCPErrorTypes(t *testing.T) {
	// Test custom error types
	err := NewMCPError("test_operation", "test_server", ErrNotConnected)
	require.Contains(t, err.Error(), "test_operation")
	require.Contains(t, err.Error(), "test_server")
	require.True(t, IsNotConnectedError(err))

	// Test error unwrapping
	require.ErrorIs(t, err, ErrNotConnected)
}

func TestMCPClient_NewClient(t *testing.T) {
	config := &ServerConfig{
		Type: "stdio",
		Name: "test-server",
		URL:  "/bin/echo",
		// ToolEnabled: true,
	}

	client, err := NewClient(config)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "test-server", client.config.Name)
	require.False(t, client.IsConnected())
}

func TestMCPClient_ErrorHandling(t *testing.T) {
	config := &ServerConfig{
		Type: "stdio",
		Name: "test-server",
		URL:  "/bin/echo",
		// ToolEnabled: true,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	// Test operations on disconnected client
	ctx := context.Background()

	_, err = client.ListTools(ctx)
	require.Error(t, err)
	require.True(t, IsNotConnectedError(err))

	_, err = client.ListResources(ctx)
	require.Error(t, err)
	require.True(t, IsNotConnectedError(err))

	_, err = client.ReadResource(ctx, "test://resource")
	require.Error(t, err)
	require.True(t, IsNotConnectedError(err))

	_, err = client.CallTool(ctx, "test", map[string]interface{}{})
	require.Error(t, err)
	require.True(t, IsNotConnectedError(err))
}

func TestMCPManager_Basic(t *testing.T) {
	manager := NewManager()
	require.NotNil(t, manager)

	// Test getting server names from empty manager
	serverNames := manager.GetServerNames()
	require.Empty(t, serverNames)
}

func TestToolFilterConfiguration(t *testing.T) {
	// Test with tools disabled
	config := &ServerConfig{
		Type: "stdio",
		Name: "test-server",
		URL:  "/bin/echo",
		ToolConfiguration: &ToolConfiguration{
			Enabled: false,
		},
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	tools := []mcp.Tool{
		{Name: "tool1"},
		{Name: "tool2"},
	}

	filtered := client.filterTools(tools)
	require.Empty(t, filtered)

	// Test with allowed tools filter
	config.ToolConfiguration = &ToolConfiguration{
		Enabled:      true,
		AllowedTools: []string{"tool1"},
	}

	filtered = client.filterTools(tools)
	require.Len(t, filtered, 1)
	require.Equal(t, "tool1", filtered[0].Name)

	// Test with no filter (all tools allowed)
	config.ToolConfiguration = &ToolConfiguration{
		Enabled: true,
	}
	filtered = client.filterTools(tools)
	require.Len(t, filtered, 2)
}
