package config

import (
	"context"
	"fmt"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/mcp"
	"github.com/deepnoodle-ai/dive/slogger"
)

type BuildOptions struct {
	Logger     slogger.Logger
	Confirmer  dive.Confirmer
	ThreadRepo dive.ThreadRepository
	BasePath   string
}

// BuildAgents creates agents from the given configuration
func BuildAgents(config *Config, opts BuildOptions) ([]dive.Agent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var logger slogger.Logger = slogger.DefaultLogger
	if opts.Logger != nil {
		logger = opts.Logger
	}

	confirmationMode := dive.ConfirmIfNotReadOnly
	if config.Config.ConfirmationMode != "" {
		confirmationMode = dive.ConfirmationMode(config.Config.ConfirmationMode)
		if !confirmationMode.IsValid() {
			return nil, fmt.Errorf("invalid confirmation mode: %s", config.Config.ConfirmationMode)
		}
	}

	var confirmer dive.Confirmer
	if opts.Confirmer != nil {
		confirmer = opts.Confirmer
	} else {
		confirmer = dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
			Mode: confirmationMode,
		})
	}

	// Initialize MCP manager to discover tools
	var mcpServers []*mcp.ServerConfig
	for _, mcpServer := range config.MCPServers {
		mcpServers = append(mcpServers, mcpServer.ToMCPConfig())
	}
	mcpManager := mcp.NewManager(mcp.ManagerOptions{Logger: logger})
	var mcpTools map[string]dive.Tool
	if len(mcpServers) > 0 {
		if err := mcpManager.InitializeServers(ctx, mcpServers); err != nil {
			return nil, fmt.Errorf("failed to initialize mcp servers: %w", err)
		}
		mcpTools = mcpManager.GetAllTools()
	} else {
		mcpTools = map[string]dive.Tool{}
	}

	toolDefsByName := make(map[string]Tool)
	for _, toolDef := range config.Tools {
		toolDefsByName[toolDef.Name] = toolDef
	}

	// Auto-add any tool definitions mentioned in agents by name that were
	// otherwise unconfigured. These will use the tool's default configuration.
	for _, agentDef := range config.Agents {
		for _, toolName := range agentDef.Tools {
			if _, ok := toolDefsByName[toolName]; !ok {
				toolDefsByName[toolName] = Tool{Name: toolName}
			}
		}
	}

	// Start building a map of tools that will be passed to agents
	toolsMap := map[string]dive.Tool{}
	for _, mcpTool := range mcpTools {
		toolsMap[mcpTool.Name()] = mcpTool
	}

	// Identify which tools were NOT provided by MCP servers, initialize them,
	// and then add them to the tools map
	regularToolDefs := make([]Tool, 0, len(toolDefsByName))
	for _, toolDef := range toolDefsByName {
		if _, isMCPTool := mcpTools[toolDef.Name]; !isMCPTool {
			regularToolDefs = append(regularToolDefs, toolDef)
		}
	}
	regularTools, err := initializeTools(regularToolDefs)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tools: %w", err)
	}
	for toolName, tool := range regularTools {
		toolsMap[toolName] = tool
	}

	agents := make([]dive.Agent, 0, len(config.Agents))
	for _, agentDef := range config.Agents {
		agent, err := buildAgent(ctx, opts.BasePath, agentDef, config.Config, toolsMap, logger, confirmer, opts.BasePath, opts.ThreadRepo)
		if err != nil {
			return nil, fmt.Errorf("failed to build agent %s: %w", agentDef.Name, err)
		}
		agents = append(agents, agent)
	}
	return agents, nil
}
