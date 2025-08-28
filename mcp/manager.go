package mcp

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/slogger"
)

// MCPServerConnection represents a connection to an MCP server
type MCPServerConnection struct {
	Client             *Client
	Config             *ServerConfig
	Tools              []dive.Tool
	ResourceRepository dive.DocumentRepository
}

// Manager manages multiple MCP server connections and tool discovery
type Manager struct {
	servers map[string]*MCPServerConnection
	tools   map[string]dive.Tool
	logger  slogger.Logger
	mutex   sync.RWMutex
}

// ManagerOptions configures a new MCP manager
type ManagerOptions struct {
	Logger slogger.Logger
}

// NewManager creates a new MCP manager
func NewManager(opts ...ManagerOptions) *Manager {
	m := &Manager{
		servers: make(map[string]*MCPServerConnection),
		tools:   make(map[string]dive.Tool),
	}
	if len(opts) > 0 {
		m.logger = opts[0].Logger
	}
	return m
}

// InitializeServers connects to and initializes all configured MCP servers
func (m *Manager) InitializeServers(ctx context.Context, serverConfigs []*ServerConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var errors []error
	for _, serverConfig := range serverConfigs {
		if err := m.initializeServer(ctx, serverConfig); err != nil {
			errors = append(errors, fmt.Errorf("failed to initialize mcp server %s: %w", serverConfig.Name, err))
			continue
		}
	}
	if len(errors) > 0 {
		// Return the first error, but log all errors
		return errors[0]
	}
	return nil
}

// initializeServer initializes a single MCP server connection
func (m *Manager) initializeServer(ctx context.Context, serverConfig *ServerConfig) error {
	// Check if server is already initialized - if so, skip silently
	if _, exists := m.servers[serverConfig.Name]; exists {
		return nil
	}

	if m.logger != nil {
		m.logger.Info("mcp server starting",
			"server", serverConfig.Name,
			"type", serverConfig.Type,
		)
	}

	// Create mcp client, connect, and discover tools
	client, err := NewClient(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to create mcp client: %w", err)
	}
	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to mcp server: %w", err)
	}
	mcpTools, err := client.ListTools(ctx)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to list mcp tools: %w", err)
	}

	// Create tool adapters
	var tools []dive.Tool
	for _, mcpTool := range mcpTools {
		adapter := NewToolAdapter(client, mcpTool, serverConfig.Name)
		tools = append(tools, adapter)
		// Note we do NOT currently prefix MCP tool names with the server name.
		// This means that if two MCP servers have tools with the same name,
		// they will conflict. For now, let's raise an error if this happens.
		//
		// Add to global tools map with server prefix to avoid name conflicts
		// toolKey := fmt.Sprintf("%s.%s", serverConfig.Name, mcpTool.Name)
		if _, exists := m.tools[mcpTool.Name]; exists {
			return fmt.Errorf("mcp server %s has duplicate tool name %q",
				serverConfig.Name, mcpTool.Name)
		}
		m.tools[mcpTool.Name] = adapter
	}

	// Create resource repository if server supports resources
	var resourceRepo dive.DocumentRepository
	if client.GetServerCapabilities() != nil && client.GetServerCapabilities().Resources != nil {
		resourceRepo = NewResourceRepository(client, serverConfig.Name)
	}

	if m.logger != nil {
		var toolNames []string
		for _, tool := range tools {
			toolNames = append(toolNames, tool.Name())
		}
		sort.Strings(toolNames)
		m.logger.Info("mcp server is ready",
			"server", serverConfig.Name,
			"type", serverConfig.Type,
			"tool_count", len(tools),
			"tool_names", toolNames,
			"has_resources", resourceRepo != nil,
		)
	}

	// Store the server connection
	m.servers[serverConfig.Name] = &MCPServerConnection{
		Client:             client,
		Config:             serverConfig,
		Tools:              tools,
		ResourceRepository: resourceRepo,
	}
	return nil
}

// GetAllTools returns all tools from all connected MCP servers
func (m *Manager) GetAllTools() map[string]dive.Tool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy of the tools map
	result := make(map[string]dive.Tool, len(m.tools))
	for k, v := range m.tools {
		result[k] = v
	}
	return result
}

// GetToolsByServer returns tools from a specific MCP server
func (m *Manager) GetToolsByServer(serverName string) []dive.Tool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if server, exists := m.servers[serverName]; exists {
		return server.Tools
	}
	return nil
}

// GetTool returns a specific tool by name (with server prefix)
func (m *Manager) GetTool(toolKey string) dive.Tool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return m.tools[toolKey]
}

// GetServerStatus returns the connection status of all servers
func (m *Manager) GetServerStatus() map[string]bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	status := make(map[string]bool)
	for name, server := range m.servers {
		status[name] = server.Client.IsConnected()
	}
	return status
}

// RefreshTools refreshes the tool list for all servers
func (m *Manager) RefreshTools(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var errors []error

	for serverName, server := range m.servers {
		if !server.Client.IsConnected() {
			continue
		}

		// Re-discover tools
		mcpTools, err := server.Client.ListTools(ctx)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to refresh tools for mcp server %s: %w", serverName, err))
			continue
		}

		// Remove old tools for this server
		for toolKey := range m.tools {
			if len(toolKey) > len(serverName)+1 && toolKey[:len(serverName)+1] == serverName+"." {
				delete(m.tools, toolKey)
			}
		}

		// Create new tool adapters
		var tools []dive.Tool
		for _, mcpTool := range mcpTools {
			adapter := NewToolAdapter(server.Client, mcpTool, serverName)
			tools = append(tools, adapter)
			m.tools[mcpTool.Name] = adapter
		}

		// Update server tools
		server.Tools = tools
	}

	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

// Close closes all MCP server connections
func (m *Manager) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var errors []error

	for serverName, server := range m.servers {
		if err := server.Client.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close mcp server %s: %w", serverName, err))
		}
	}

	// Clear all data
	m.servers = make(map[string]*MCPServerConnection)
	m.tools = make(map[string]dive.Tool)

	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

// IsServerConnected checks if a specific server is connected
func (m *Manager) IsServerConnected(serverName string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if server, exists := m.servers[serverName]; exists {
		return server.Client.IsConnected()
	}
	return false
}

// GetServerNames returns a list of all configured server names
func (m *Manager) GetServerNames() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	names := make([]string, 0, len(m.servers))
	for name := range m.servers {
		names = append(names, name)
	}
	return names
}

// GetResourceRepository returns the resource repository for a specific server
func (m *Manager) GetResourceRepository(serverName string) dive.DocumentRepository {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if server, exists := m.servers[serverName]; exists {
		return server.ResourceRepository
	}
	return nil
}

// GetAllResourceRepositories returns a map of all resource repositories by server name
func (m *Manager) GetAllResourceRepositories() map[string]dive.DocumentRepository {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	repositories := make(map[string]dive.DocumentRepository)
	for serverName, server := range m.servers {
		if server.ResourceRepository != nil {
			repositories[serverName] = server.ResourceRepository
		}
	}
	return repositories
}
