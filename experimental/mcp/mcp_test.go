package mcp

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPErrorTypes(t *testing.T) {
	// Test custom error types
	err := NewMCPError("test_operation", "test_server", ErrNotConnected)
	assert.Contains(t, err.Error(), "test_operation")
	assert.Contains(t, err.Error(), "test_server")
	assert.True(t, IsNotConnectedError(err))

	// Test error unwrapping
	assert.ErrorIs(t, err, ErrNotConnected)
}

func TestMCPClient_NewClient(t *testing.T) {
	config := &ServerConfig{
		Type: "stdio",
		Name: "test-server",
		URL:  "/bin/echo",
		// ToolEnabled: true,
	}

	client, err := NewClient(config)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "test-server", client.config.Name)
	assert.False(t, client.IsConnected())
}

func TestMCPClient_ErrorHandling(t *testing.T) {
	config := &ServerConfig{
		Type: "stdio",
		Name: "test-server",
		URL:  "/bin/echo",
		// ToolEnabled: true,
	}

	client, err := NewClient(config)
	assert.NoError(t, err)

	// Test operations on disconnected client
	ctx := context.Background()

	_, err = client.ListTools(ctx)
	assert.Error(t, err)
	assert.True(t, IsNotConnectedError(err))

	_, err = client.ListResources(ctx)
	assert.Error(t, err)
	assert.True(t, IsNotConnectedError(err))

	_, err = client.ReadResource(ctx, "test://resource")
	assert.Error(t, err)
	assert.True(t, IsNotConnectedError(err))

	_, err = client.CallTool(ctx, "test", map[string]interface{}{})
	assert.Error(t, err)
	assert.True(t, IsNotConnectedError(err))
}

func TestMCPManager_Basic(t *testing.T) {
	manager := NewManager()
	assert.NotNil(t, manager)

	// Test getting server names from empty manager
	serverNames := manager.GetServerNames()
	assert.Empty(t, serverNames)
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
	assert.NoError(t, err)

	tools := []mcp.Tool{
		{Name: "tool1"},
		{Name: "tool2"},
	}

	filtered := client.filterTools(tools)
	assert.Empty(t, filtered)

	// Test with allowed tools filter
	config.ToolConfiguration = &ToolConfiguration{
		Enabled:      true,
		AllowedTools: []string{"tool1"},
	}

	filtered = client.filterTools(tools)
	assert.Len(t, filtered, 1)
	assert.Equal(t, "tool1", filtered[0].Name)

	// Test with no filter (all tools allowed)
	config.ToolConfiguration = &ToolConfiguration{
		Enabled: true,
	}
	filtered = client.filterTools(tools)
	assert.Len(t, filtered, 2)
}
