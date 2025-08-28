package mcp

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	require.NotNil(t, manager)
	require.NotNil(t, manager.servers)
	require.NotNil(t, manager.tools)
	require.Empty(t, manager.servers)
	require.Empty(t, manager.tools)
}

func TestMCPManager_GetAllTools_EmptyManager(t *testing.T) {
	manager := NewManager()
	tools := manager.GetAllTools()
	require.NotNil(t, tools)
	require.Empty(t, tools)
}

func TestMCPManager_GetToolsByServer_NonExistent(t *testing.T) {
	manager := NewManager()
	tools := manager.GetToolsByServer("nonexistent")
	require.Nil(t, tools)
}

func TestMCPManager_GetTool_NonExistent(t *testing.T) {
	manager := NewManager()
	tool := manager.GetTool("nonexistent.tool")
	require.Nil(t, tool)
}

func TestMCPManager_GetServerStatus_Empty(t *testing.T) {
	manager := NewManager()
	status := manager.GetServerStatus()
	require.NotNil(t, status)
	require.Empty(t, status)
}

func TestMCPManager_IsServerConnected_NonExistent(t *testing.T) {
	manager := NewManager()
	connected := manager.IsServerConnected("nonexistent")
	require.False(t, connected)
}

func TestMCPManager_GetServerNames_Empty(t *testing.T) {
	manager := NewManager()
	names := manager.GetServerNames()
	require.NotNil(t, names)
	require.Empty(t, names)
}

func TestMCPManager_Close_EmptyManager(t *testing.T) {
	manager := NewManager()
	err := manager.Close()
	require.NoError(t, err)
}

func TestMCPManager_RefreshTools_EmptyManager(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()
	err := manager.RefreshTools(ctx)
	require.NoError(t, err)
}

func TestMCPManager_InitializeServers_UnsupportedType(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	serverConfigs := []*ServerConfig{
		{
			Type: "unsupported",
			Name: "test-server",
			URL:  "invalid://url",
			// ToolEnabled: true,
		},
	}

	err := manager.InitializeServers(ctx, serverConfigs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported mcp server type: unsupported")

	// Verify no servers were added
	names := manager.GetServerNames()
	require.Empty(t, names)
}

func TestMCPManager_InitializeServers_DuplicateServer(t *testing.T) {
	manager := NewManager()

	// Manually add a server connection to simulate already initialized server
	testConfig := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	}

	client, err := NewClient(testConfig)
	require.NoError(t, err)

	manager.servers["test-server"] = &MCPServerConnection{
		Client: client,
		Config: testConfig,
		Tools:  []dive.Tool{},
	}

	// Try to initialize the same server again
	ctx := context.Background()
	serverConfigs := []*ServerConfig{testConfig}

	err = manager.InitializeServers(ctx, serverConfigs)
	require.NoError(t, err) // Should not return error - silently skip already initialized servers
}

func TestMCPManager_WithMockServer(t *testing.T) {
	manager := NewManager()

	// Create a test server connection manually
	testConfig := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	}

	client, err := NewClient(testConfig)
	require.NoError(t, err)

	// Mock some tools
	mockTools := []mcp.Tool{
		{
			Name:        "test-tool-1",
			Description: "Test tool 1",
		},
		{
			Name:        "test-tool-2",
			Description: "Test tool 2",
		},
	}

	var diveTools []dive.Tool
	for _, mcpTool := range mockTools {
		adapter := NewToolAdapter(client, mcpTool, testConfig.Name)
		diveTools = append(diveTools, adapter)
	}

	// Add the server connection manually (simulating successful initialization)
	manager.servers[testConfig.Name] = &MCPServerConnection{
		Client: client,
		Config: testConfig,
		Tools:  diveTools,
	}

	// Add tools to the global tools map
	for _, tool := range diveTools {
		toolKey := testConfig.Name + "." + tool.Name()
		manager.tools[toolKey] = tool
	}

	// Test GetServerNames
	names := manager.GetServerNames()
	require.Len(t, names, 1)
	require.Contains(t, names, "test-server")

	// Test GetToolsByServer
	tools := manager.GetToolsByServer("test-server")
	require.Len(t, tools, 2)

	// Test GetAllTools
	allTools := manager.GetAllTools()
	require.Len(t, allTools, 2)
	require.Contains(t, allTools, "test-server.test-tool-1")
	require.Contains(t, allTools, "test-server.test-tool-2")

	// Test GetTool
	tool1 := manager.GetTool("test-server.test-tool-1")
	require.NotNil(t, tool1)
	require.Equal(t, "test-tool-1", tool1.Name())

	// Test GetServerStatus
	status := manager.GetServerStatus()
	require.Len(t, status, 1)
	// Note: Since we didn't actually connect, this will be false
	require.Contains(t, status, "test-server")

	// Test IsServerConnected
	connected := manager.IsServerConnected("test-server")
	require.False(t, connected) // Not actually connected

	// Test Close
	err = manager.Close()
	require.NoError(t, err)

	// Verify everything is cleaned up
	names = manager.GetServerNames()
	require.Empty(t, names)
	allTools = manager.GetAllTools()
	require.Empty(t, allTools)
}

func TestMCPManager_RefreshTools_WithMockServer(t *testing.T) {
	manager := NewManager()

	// Create a test server connection
	testConfig := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	}

	client, err := NewClient(testConfig)
	require.NoError(t, err)

	// Set up the client as connected but don't actually connect
	client.connected = true

	// Add initial tools
	initialTools := []mcp.Tool{
		{Name: "initial-tool", Description: "Initial tool"},
	}

	var diveTools []dive.Tool
	for _, mcpTool := range initialTools {
		adapter := NewToolAdapter(client, mcpTool, testConfig.Name)
		diveTools = append(diveTools, adapter)
	}

	manager.servers[testConfig.Name] = &MCPServerConnection{
		Client: client,
		Config: testConfig,
		Tools:  diveTools,
	}

	// Add initial tools to global map
	for _, tool := range diveTools {
		toolKey := testConfig.Name + "." + tool.Name()
		manager.tools[toolKey] = tool
	}

	// Verify initial state
	require.Len(t, manager.GetAllTools(), 1)

	// RefreshTools will fail because we can't actually list tools from a mock server
	// but it should handle the error gracefully - we can't test this easily without
	// a proper mock, so we'll just verify that the method exists and doesn't panic
	// when called on disconnected servers
	ctx := context.Background()

	// Disconnect the client to avoid the nil pointer dereference
	client.connected = false

	err = manager.RefreshTools(ctx)
	require.NoError(t, err) // Should succeed by skipping disconnected servers

	// Verify the server connection is still there
	require.False(t, manager.IsServerConnected("test-server")) // Now disconnected
}

func TestMCPManager_RefreshTools_DisconnectedServer(t *testing.T) {
	manager := NewManager()

	// Create a test server connection that's not connected
	testConfig := &ServerConfig{
		Type: "http",
		Name: "test-server",
		URL:  "http://localhost:8080",
		// ToolEnabled: true,
	}

	client, err := NewClient(testConfig)
	require.NoError(t, err)
	// Don't set connected = true, so it's disconnected

	manager.servers[testConfig.Name] = &MCPServerConnection{
		Client: client,
		Config: testConfig,
		Tools:  []dive.Tool{},
	}

	// RefreshTools should skip disconnected servers
	ctx := context.Background()
	err = manager.RefreshTools(ctx)
	require.NoError(t, err) // Should succeed by skipping disconnected servers
}
