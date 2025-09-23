package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/enhanced"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/log"
	"github.com/deepnoodle-ai/dive/subagents"
)

const (
	defaultResponseTimeout    = 5 * time.Minute
	defaultToolIterationLimit = 20
)

// AgentFactory provides methods to create agents with enhanced features
type AgentFactory struct {
	unifiedConfig *UnifiedConfig
	environment   *enhanced.Environment
	logger        log.Logger
}

// NewAgentFactory creates a new agent factory
func NewAgentFactory(ctx context.Context, configPath string) (*AgentFactory, error) {
	// Load unified configuration
	uc, err := LoadUnifiedConfig(ctx, configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load unified config: %w", err)
	}

	// Set up logger
	logger := log.New(log.GetDefaultLevel())
	if uc.Config != nil && uc.Config.Config.LogLevel != "" {
		level := log.LevelFromString(uc.Config.Config.LogLevel)
		logger = log.New(level)
	}

	// Create environment options
	opts := EnvironmentOpts{
		Config:    uc.Config,
		Logger:    logger,
		Directory: filepath.Dir(configPath),
	}

	// Set up confirmation mode
	confirmationMode := dive.ConfirmIfNotReadOnly
	if uc.Config != nil && uc.Config.Config.ConfirmationMode != "" {
		confirmationMode = dive.ConfirmationMode(uc.Config.Config.ConfirmationMode)
	}
	opts.Confirmer = dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: confirmationMode,
	})

	// Create enhanced environment
	env, err := uc.CreateEnhancedEnvironment(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create enhanced environment: %w", err)
	}

	return &AgentFactory{
		unifiedConfig: uc,
		environment:   env,
		logger:        logger,
	}, nil
}

// CreateAgent creates an enhanced agent by name
func (af *AgentFactory) CreateAgent(name string) (dive.Agent, error) {
	// Find the agent in the environment
	for _, agent := range af.environment.Agents {
		if agent.Name() == name {
			// Agent is already enhanced by the environment
			return agent, nil
		}
	}

	// If not found, try to create from subagent definition
	if af.environment.SubagentManager != nil {
		subagentDef, err := af.environment.SubagentManager.GetSubagent(name)
		if err == nil {
			// Create agent from subagent definition
			baseAgent, err := af.createBaseAgentFromSubagent(subagentDef)
			if err != nil {
				return nil, err
			}
			// Enhance it
			return af.environment.EnhanceAgent(baseAgent), nil
		}
	}

	return nil, fmt.Errorf("agent '%s' not found", name)
}

// CreateDefaultAgent creates the default agent with all enhancements
func (af *AgentFactory) CreateDefaultAgent() (dive.Agent, error) {
	if len(af.environment.Agents) == 0 {
		// Create a basic agent if none configured
		agent, err := af.createBasicAgent()
		if err != nil {
			return nil, err
		}
		return af.environment.EnhanceAgent(agent), nil
	}

	// Return the first configured agent
	return af.environment.Agents[0], nil
}

// CreateAgentWithOptions creates a custom agent with specified options
func (af *AgentFactory) CreateAgentWithOptions(opts dive.AgentOptions) (dive.Agent, error) {
	// Apply defaults from configuration
	if opts.Logger == nil {
		opts.Logger = af.logger
	}

	if opts.ResponseTimeout == 0 && af.unifiedConfig.Config != nil {
		// Could add a ResponseTimeout to config if needed
		opts.ResponseTimeout = defaultResponseTimeout
	}

	if opts.ToolIterationLimit == 0 {
		opts.ToolIterationLimit = defaultToolIterationLimit
	}

	// Apply model settings from config if not specified
	if opts.ModelSettings == nil && af.unifiedConfig.Config != nil {
		opts.ModelSettings = af.createModelSettings()
	}

	// Add MCP servers if configured
	if af.unifiedConfig.MCPConfig != nil && len(af.unifiedConfig.MCPConfig.ServerConfigs) > 0 {
		var mcpServers []llm.MCPServerConfig
		for _, server := range af.unifiedConfig.MCPConfig.ServerConfigs {
			if server.AutoStart {
				mcpServers = append(mcpServers, *server.MCPServer.ToLLMConfig())
			}
		}
		if opts.ModelSettings == nil {
			opts.ModelSettings = &dive.ModelSettings{}
		}
		opts.ModelSettings.MCPServers = mcpServers
	}

	// Add tools from environment
	if len(opts.Tools) == 0 && af.environment.Tools != nil {
		for _, tool := range af.environment.Tools {
			opts.Tools = append(opts.Tools, tool)
		}
	}

	// Create the base agent
	agent, err := dive.NewAgent(opts)
	if err != nil {
		return nil, err
	}

	// Enhance with our features
	return af.environment.EnhanceAgent(agent), nil
}

// GetEnvironment returns the enhanced environment
func (af *AgentFactory) GetEnvironment() *enhanced.Environment {
	return af.environment
}

// GetUnifiedConfig returns the unified configuration
func (af *AgentFactory) GetUnifiedConfig() *UnifiedConfig {
	return af.unifiedConfig
}

// createBaseAgentFromSubagent creates a base agent from a subagent definition
func (af *AgentFactory) createBaseAgentFromSubagent(def *subagents.SubagentDefinition) (dive.Agent, error) {
	// Get model
	model, err := GetModel("", def.Model)
	if err != nil {
		// Use default model if specified one not found
		model, err = GetModel(af.unifiedConfig.Config.Config.DefaultProvider, af.unifiedConfig.Config.Config.DefaultModel)
		if err != nil {
			return nil, fmt.Errorf("failed to get model: %w", err)
		}
	}

	// Get tools
	var tools []dive.Tool
	for _, toolName := range def.Tools {
		if tool, ok := af.environment.Tools[toolName]; ok {
			tools = append(tools, tool)
		}
	}

	opts := dive.AgentOptions{
		Name:         def.Name,
		Instructions: def.SystemPrompt,
		Model:        model,
		Tools:        tools,
		Logger:       af.logger,
	}

	return dive.NewAgent(opts)
}

// createBasicAgent creates a basic agent with defaults
func (af *AgentFactory) createBasicAgent() (dive.Agent, error) {
	// Get default model
	model, err := GetModel(
		af.unifiedConfig.Config.Config.DefaultProvider,
		af.unifiedConfig.Config.Config.DefaultModel,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get default model: %w", err)
	}

	// Get all available tools
	var tools []dive.Tool
	for _, tool := range af.environment.Tools {
		tools = append(tools, tool)
	}

	opts := dive.AgentOptions{
		Name:          "Assistant",
		Instructions:  "You are a helpful AI assistant.",
		Model:         model,
		Tools:         tools,
		Logger:        af.logger,
		ModelSettings: af.createModelSettings(),
	}

	return dive.NewAgent(opts)
}

// createModelSettings creates model settings from configuration
func (af *AgentFactory) createModelSettings() *dive.ModelSettings {
	settings := &dive.ModelSettings{}

	// Apply settings from config
	if af.unifiedConfig.Config != nil {
		// Could extract temperature, max tokens, etc. from config
		// For now, using defaults
	}

	// Add MCP servers
	if af.unifiedConfig.MCPConfig != nil && len(af.unifiedConfig.MCPConfig.ServerConfigs) > 0 {
		var mcpServers []llm.MCPServerConfig
		for _, server := range af.unifiedConfig.MCPConfig.ServerConfigs {
			if server.AutoStart {
				mcpServers = append(mcpServers, *server.MCPServer.ToLLMConfig())
			}
		}
		settings.MCPServers = mcpServers
	}

	return settings
}

// LoadOrCreateConfig loads an existing config or creates a new one
func LoadOrCreateConfig(ctx context.Context, path string) (*UnifiedConfig, error) {
	// Try to load existing config
	if _, err := os.Stat(path); err == nil {
		return LoadUnifiedConfig(ctx, path)
	}

	// Create new config with defaults
	uc := &UnifiedConfig{
		Config: &Config{
			Name:    "Dive Configuration",
			Version: "1.0",
			Config: GlobalConfig{
				LogLevel: "info",
			},
		},
		Memory: &MemoryConfig{
			Enabled:  true,
			AutoLoad: true,
		},
		Permissions: &PermissionConfig{
			DefaultMode: "normal",
		},
		MCPConfig: &MCPConfiguration{
			AutoDiscovery: true,
		},
	}

	// Initialize runtime
	uc.runtime = &RuntimeState{
		Context: ctx,
		Logger:  log.New(log.GetDefaultLevel()),
	}

	return uc, nil
}

// QuickStart provides a simple way to get started with enhanced agents
func QuickStart(ctx context.Context) (*AgentFactory, error) {
	// Look for config in common locations
	configPaths := []string{
		"dive.yaml",
		"dive.yml",
		".dive/config.yaml",
		".dive/config.yml",
	}

	var configPath string
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			configPath = path
			break
		}
	}

	// Create factory with found config or empty config
	return NewAgentFactory(ctx, configPath)
}