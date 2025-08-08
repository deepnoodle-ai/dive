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

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/environment"
	"github.com/diveagents/dive/mcp"
	"github.com/diveagents/dive/slogger"
	"github.com/goccy/go-yaml"
)

// Environment is a serializable representation of an AI agent environment
type Environment struct {
	Name        string      `yaml:"Name,omitempty" json:"Name,omitempty"`
	Description string      `yaml:"Description,omitempty" json:"Description,omitempty"`
	Version     string      `yaml:"Version,omitempty" json:"Version,omitempty"`
	Config      Config      `yaml:"Config,omitempty" json:"Config,omitempty"`
	Tools       []Tool      `yaml:"Tools,omitempty" json:"Tools,omitempty"`
	Documents   []Document  `yaml:"Documents,omitempty" json:"Documents,omitempty"`
	Agents      []Agent     `yaml:"Agents,omitempty" json:"Agents,omitempty"`
	Workflows   []Workflow  `yaml:"Workflows,omitempty" json:"Workflows,omitempty"`
	Triggers    []Trigger   `yaml:"Triggers,omitempty" json:"Triggers,omitempty"`
	Schedules   []Schedule  `yaml:"Schedules,omitempty" json:"Schedules,omitempty"`
	MCPServers  []MCPServer `yaml:"MCPServers,omitempty" json:"MCPServers,omitempty"`
}

// Save writes an Environment configuration to a file. The file extension is used to
// determine the configuration format:
// - .json -> JSON
// - .yml or .yaml -> YAML
func (env *Environment) Save(path string) error {
	// Determine format from extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return env.SaveJSON(path)
	case ".yml", ".yaml":
		return env.SaveYAML(path)
	default:
		return fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// SaveYAML writes an Environment configuration to a YAML file
func (env *Environment) SaveYAML(path string) error {
	data, err := yaml.Marshal(env)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SaveJSON writes an Environment configuration to a JSON file
func (env *Environment) SaveJSON(path string) error {
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Write an Environment configuration to a writer in YAML format
func (env *Environment) Write(w io.Writer) error {
	return yaml.NewEncoder(w).Encode(env)
}

// Build creates a new Environment from the configuration
func (env *Environment) Build(opts ...BuildOption) (*environment.Environment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	buildOpts := &BuildOptions{}
	for _, opt := range opts {
		opt(buildOpts)
	}

	var logger slogger.Logger = slogger.DefaultLogger
	if buildOpts.Logger != nil {
		logger = buildOpts.Logger
	} else if env.Config.LogLevel != "" {
		levelStr := env.Config.LogLevel
		if !isValidLogLevel(levelStr) {
			return nil, fmt.Errorf("invalid log level: %s", levelStr)
		}
		level := slogger.LevelFromString(levelStr)
		logger = slogger.New(level)
	}

	confirmationMode := dive.ConfirmIfNotReadOnly
	if env.Config.ConfirmationMode != "" {
		confirmationMode = dive.ConfirmationMode(env.Config.ConfirmationMode)
		if !confirmationMode.IsValid() {
			return nil, fmt.Errorf("invalid confirmation mode: %s", env.Config.ConfirmationMode)
		}
	}

	confirmer := dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{
		Mode: confirmationMode,
	})

	// Initialize MCP manager to discover tools
	var mcpServers []*mcp.ServerConfig
	for _, mcpServer := range env.MCPServers {
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
	for _, toolDef := range env.Tools {
		toolDefsByName[toolDef.Name] = toolDef
	}

	// Auto-add any tool definitions mentioned in agents by name that were
	// otherwise unconfigured. These will use the tool's default configuration.
	for _, agentDef := range env.Agents {
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

	// Documents
	if buildOpts.DocumentsDir != "" && buildOpts.DocumentsRepo != nil {
		return nil, fmt.Errorf("documents dir and repo cannot both be set")
	}
	var docRepo dive.DocumentRepository
	if buildOpts.DocumentsRepo != nil {
		docRepo = buildOpts.DocumentsRepo
	} else {
		dir := buildOpts.DocumentsDir
		if dir == "" {
			dir = "."
		}
		docRepo, err = agent.NewFileDocumentRepository(dir)
		if err != nil {
			return nil, fmt.Errorf("failed to create document repository: %w", err)
		}
	}
	if docRepo != nil {
		namedDocuments := make(map[string]*dive.DocumentMetadata, len(env.Documents))
		for _, doc := range env.Documents {
			namedDocuments[doc.Name] = &dive.DocumentMetadata{
				Name: doc.Name,
				Path: doc.Path,
			}
		}
		for _, doc := range env.Documents {
			if err := docRepo.RegisterDocument(ctx, doc.Name, doc.Path); err != nil {
				return nil, fmt.Errorf("failed to register document %s: %w", doc.Name, err)
			}
		}
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

	// Agents
	agents := make([]dive.Agent, 0, len(env.Agents))
	for _, agentDef := range env.Agents {
		agent, err := buildAgent(ctx, docRepo, agentDef, env.Config, toolsMap, logger, confirmer, basePath)
		if err != nil {
			return nil, fmt.Errorf("failed to build agent %s: %w", agentDef.Name, err)
		}
		agents = append(agents, agent)
	}

	var threadRepo dive.ThreadRepository
	if buildOpts.ThreadRepo != nil {
		threadRepo = buildOpts.ThreadRepo
	} else {
		threadRepo = agent.NewMemoryThreadRepository()
	}

	// Environment
	result, err := environment.New(environment.Options{
		Name:               env.Name,
		Description:        env.Description,
		Agents:             agents,
		Logger:             logger,
		DocumentRepository: docRepo,
		ThreadRepository:   threadRepo,
		Confirmer:          confirmer,
		MCPServers:         mcpServers,
		MCPManager:         mcpManager,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}
	return result, nil
}
