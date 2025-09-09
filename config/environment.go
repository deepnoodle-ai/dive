package config

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/mcp"
	"github.com/deepnoodle-ai/dive/slogger"
)

type EnvironmentOpts struct {
	Config     *Config
	Logger     slogger.Logger
	Confirmer  dive.Confirmer
	Threads    dive.ThreadRepository
	Directory  string
	MCPManager *mcp.Manager
}

type Environment struct {
	Config     *Config
	MCPManager *mcp.Manager
	Threads    dive.ThreadRepository
	Confirmer  dive.Confirmer
	Logger     slogger.Logger
	Directory  string
	Agents     []dive.Agent
	Tools      map[string]dive.Tool
}

// NewEnvironment creates Dive resources corresponding to the given configuration.
// This includes initializing MCP servers, tools, and agents.
func NewEnvironment(ctx context.Context, opts EnvironmentOpts) (*Environment, error) {
	cfg := opts.Config

	if opts.Logger == nil {
		opts.Logger = slogger.DefaultLogger
	}

	confirmationMode := dive.ConfirmIfNotReadOnly
	if cfg.Config.ConfirmationMode != "" {
		confirmationMode = dive.ConfirmationMode(cfg.Config.ConfirmationMode)
		if !confirmationMode.IsValid() {
			return nil, fmt.Errorf("invalid confirmation mode: %s", cfg.Config.ConfirmationMode)
		}
	}
	if opts.Confirmer == nil {
		opts.Confirmer = dive.NewTerminalConfirmer(dive.TerminalConfirmerOptions{Mode: confirmationMode})
	}

	if opts.MCPManager == nil {
		opts.MCPManager = mcp.NewManager(mcp.ManagerOptions{Logger: opts.Logger})
	}

	// Initialize MCP manager to discover tools
	var mcpServers []*mcp.ServerConfig
	for _, mcpServer := range cfg.MCPServers {
		mcpServers = append(mcpServers, mcpServer.ToMCPConfig())
	}
	var mcpTools map[string]dive.Tool
	if len(mcpServers) > 0 {
		if err := opts.MCPManager.InitializeServers(ctx, mcpServers); err != nil {
			return nil, fmt.Errorf("failed to initialize mcp servers: %w", err)
		}
		mcpTools = opts.MCPManager.GetAllTools()
	} else {
		mcpTools = map[string]dive.Tool{}
	}

	toolDefsByName := make(map[string]Tool)
	for _, toolDef := range cfg.Tools {
		toolDefsByName[toolDef.Name] = toolDef
	}

	// Auto-add any tool definitions mentioned in agents by name that were
	// otherwise unconfigured. These will use the tool's default configuration.
	for _, agentDef := range cfg.Agents {
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

	env := &Environment{
		Config:     cfg,
		MCPManager: opts.MCPManager,
		Threads:    opts.Threads,
		Confirmer:  opts.Confirmer,
		Logger:     opts.Logger,
		Directory:  opts.Directory,
		Tools:      toolsMap,
	}

	// Create agents
	for _, agentDef := range cfg.Agents {
		agent, err := env.buildAgent(ctx, agentDef)
		if err != nil {
			return nil, fmt.Errorf("failed to build agent %s: %w", agentDef.Name, err)
		}
		env.Agents = append(env.Agents, agent)
	}
	return env, nil
}

func (e *Environment) buildAgent(ctx context.Context, definition Agent) (dive.Agent, error) {
	providerName := definition.Provider
	if providerName == "" {
		providerName = e.Config.Config.DefaultProvider
		if providerName == "" {
			providerName = "anthropic"
		}
	}

	modelName := definition.Model
	if modelName == "" {
		modelName = e.Config.Config.DefaultModel
	}

	providerConfigByName := make(map[string]*Provider)
	for _, p := range e.Config.Config.Providers {
		providerConfigByName[p.Name] = &p
	}
	providerConfig := providerConfigByName[providerName]

	model, err := GetModel(providerName, modelName)
	if err != nil {
		return nil, fmt.Errorf("error getting model: %w", err)
	}

	var agentTools []dive.Tool
	for _, toolName := range definition.Tools {
		tool, ok := e.Tools[toolName]
		if !ok {
			return nil, fmt.Errorf("agent references unknown tool %q", toolName)
		}
		agentTools = append(agentTools, tool)
	}

	var responseTimeout time.Duration
	if definition.ResponseTimeout != nil {
		var err error
		responseTimeout, err = parseTimeout(definition.ResponseTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid response timeout: %w", err)
		}
	}

	var modelSettings *agent.ModelSettings
	if definition.ModelSettings != nil {
		modelSettings = buildModelSettings(providerConfig, definition)
	}

	// Build static context messages if provided
	var contextContent []llm.Content
	if len(definition.Context) > 0 {
		var err error
		contextContent, err = buildContextContent(e.Directory, definition.Context)
		if err != nil {
			return nil, fmt.Errorf("error building agent context: %w", err)
		}
	}

	return agent.New(agent.Options{
		ID:                 definition.ID,
		Name:               definition.Name,
		Goal:               definition.Goal,
		Instructions:       definition.Instructions,
		IsSupervisor:       definition.IsSupervisor,
		Subordinates:       definition.Subordinates,
		Model:              model,
		Tools:              agentTools,
		ResponseTimeout:    responseTimeout,
		ToolIterationLimit: definition.ToolIterationLimit,
		DateAwareness:      definition.DateAwareness,
		SystemPrompt:       definition.SystemPrompt,
		ModelSettings:      modelSettings,
		Logger:             e.Logger,
		Confirmer:          e.Confirmer,
		ThreadRepository:   e.Threads,
		Context:            contextContent,
	})
}

func buildModelSettings(p *Provider, agentDef Agent) *agent.ModelSettings {
	settings := &agent.ModelSettings{
		Temperature:       agentDef.ModelSettings.Temperature,
		PresencePenalty:   agentDef.ModelSettings.PresencePenalty,
		FrequencyPenalty:  agentDef.ModelSettings.FrequencyPenalty,
		ReasoningBudget:   agentDef.ModelSettings.ReasoningBudget,
		ReasoningEffort:   agentDef.ModelSettings.ReasoningEffort,
		MaxTokens:         agentDef.ModelSettings.MaxTokens,
		ParallelToolCalls: agentDef.ModelSettings.ParallelToolCalls,
		ToolChoice:        agentDef.ModelSettings.ToolChoice,
		RequestHeaders:    make(http.Header),
	}

	if p != nil {
		settings.Caching = p.Caching
	} else if agentDef.ModelSettings.Caching != nil {
		settings.Caching = agentDef.ModelSettings.Caching
	}

	// Combine enabled features from provider and agent
	featuresByName := make(map[string]bool)
	if p != nil {
		for _, feature := range p.Features {
			featuresByName[feature] = true
		}
	}
	for _, feature := range agentDef.ModelSettings.Features {
		featuresByName[feature] = true
	}
	features := make([]string, 0, len(featuresByName))
	for feature := range featuresByName {
		features = append(features, feature)
	}
	sort.Strings(features)
	settings.Features = features

	// Combine request headers from provider and agent
	requestHeaders := make(http.Header)
	if p != nil {
		for key, value := range p.RequestHeaders {
			requestHeaders.Add(key, value)
		}
	}
	for key, value := range agentDef.ModelSettings.RequestHeaders {
		requestHeaders.Add(key, value)
	}
	settings.RequestHeaders = requestHeaders

	return settings
}

func parseTimeout(timeout any) (time.Duration, error) {
	var result time.Duration
	switch v := timeout.(type) {
	case string:
		var err error
		result, err = time.ParseDuration(v)
		if err != nil {
			return 0, fmt.Errorf("invalid response timeout: %w", err)
		}
	case int:
		result = time.Duration(v) * time.Second
	case float64:
		result = time.Duration(int64(v)) * time.Second
	default:
		return 0, fmt.Errorf("invalid response timeout: %v", v)
	}
	return result, nil
}
