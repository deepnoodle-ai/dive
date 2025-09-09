package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/mcp"
	"github.com/deepnoodle-ai/dive/slogger"
	"github.com/goccy/go-yaml"
)

// Save writes a DiveConfig to a file. The file extension is used to
// determine the configuration format:
// - .json -> JSON
// - .yml or .yaml -> YAML
func (config *DiveConfig) Save(path string) error {
	// Determine format from extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return config.SaveJSON(path)
	case ".yml", ".yaml":
		return config.SaveYAML(path)
	default:
		return fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// SaveYAML writes a DiveConfig to a YAML file
func (config *DiveConfig) SaveYAML(path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveJSON writes a DiveConfig to a JSON file
func (config *DiveConfig) SaveJSON(path string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Write a DiveConfig to a writer in YAML format
func (config *DiveConfig) Write(w io.Writer) error {
	return yaml.NewEncoder(w).Encode(config)
}

// BuildAgents creates agents from the configuration
func (config *DiveConfig) BuildAgents(opts ...BuildOption) ([]dive.Agent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	buildOpts := &BuildOptions{}
	for _, opt := range opts {
		opt(buildOpts)
	}

	var logger slogger.Logger = slogger.DefaultLogger
	if buildOpts.Logger != nil {
		logger = buildOpts.Logger
	} else if config.Config.LogLevel != "" {
		levelStr := config.Config.LogLevel
		if !isValidLogLevel(levelStr) {
			return nil, fmt.Errorf("invalid log level: %s", levelStr)
		}
		level := slogger.LevelFromString(levelStr)
		logger = slogger.New(level)
	}

	confirmationMode := dive.ConfirmIfNotReadOnly
	if config.Config.ConfirmationMode != "" {
		confirmationMode = dive.ConfirmationMode(config.Config.ConfirmationMode)
		if !confirmationMode.IsValid() {
			return nil, fmt.Errorf("invalid confirmation mode: %s", config.Config.ConfirmationMode)
		}
	}

	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: confirmationMode,
	})

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

	// Documents directory
	baseDir := buildOpts.DocumentsDir
	if baseDir == "" {
		baseDir = "."
	}

	// This path will be used to resolve relative paths as needed
	basePath := "."
	if buildOpts.BasePath != "" {
		basePath = buildOpts.BasePath
	} else {
		if wd, err := os.Getwd(); err == nil {
			basePath = wd
		}
	}

	var threadRepo dive.ThreadRepository
	if buildOpts.ThreadRepo != nil {
		threadRepo = buildOpts.ThreadRepo
	} else {
		threadRepo = agent.NewMemoryThreadRepository()
	}

	agents := make([]dive.Agent, 0, len(config.Agents))
	for _, agentDef := range config.Agents {
		agent, err := buildAgent(ctx, baseDir, agentDef, config.Config, toolsMap, logger, confirmer, basePath, threadRepo)
		if err != nil {
			return nil, fmt.Errorf("failed to build agent %s: %w", agentDef.Name, err)
		}
		agents = append(agents, agent)
	}

	return agents, nil
}

// GetMCPServers returns the MCP server configurations
func (config *DiveConfig) GetMCPServers() []MCPServer {
	return config.MCPServers
}

func isValidLogLevel(level string) bool {
	return level == "debug" || level == "info" || level == "warn" || level == "error"
}

func boolPtr(b bool) *bool {
	return &b
}
