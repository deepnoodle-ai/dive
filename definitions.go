package dive

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/providers/anthropic"
	"github.com/getstingrai/dive/providers/groq"
	"github.com/getstingrai/dive/providers/openai"
	"github.com/getstingrai/dive/slogger"
	"gopkg.in/yaml.v3"
)

// TeamDefinition represents the top-level YAML structure
type TeamDefinition struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Agents      []AgentDefinition `yaml:"agents" json:"agents"`
	Tasks       []TaskDefinition  `yaml:"tasks" json:"tasks"`
	Tools       []ToolDefinition  `yaml:"tools" json:"tools"`
	Config      ConfigDefinition  `yaml:"config" json:"config"`
}

// ConfigDefinition contains global configuration settings
type ConfigDefinition struct {
	DefaultProvider string            `yaml:"default_provider" json:"default_provider"`
	DefaultModel    string            `yaml:"default_model" json:"default_model"`
	LogLevel        string            `yaml:"log_level" json:"log_level"`
	CacheControl    string            `yaml:"cache_control" json:"cache_control"`
	EnabledTools    []string          `yaml:"enabled_tools" json:"enabled_tools"`
	ProviderConfigs map[string]string `yaml:"provider_configs" json:"provider_configs"`
}

// AgentDefinition represents an agent definition in YAML
type AgentDefinition struct {
	Name           string            `yaml:"name" json:"name"`
	Role           RoleDefinition    `yaml:"role" json:"role"`
	Provider       string            `yaml:"provider,omitempty" json:"provider,omitempty"`
	Model          string            `yaml:"model,omitempty" json:"model,omitempty"`
	Tools          []string          `yaml:"tools,omitempty" json:"tools,omitempty"`
	CacheControl   string            `yaml:"cache_control,omitempty" json:"cache_control,omitempty"`
	MaxActiveTasks int               `yaml:"max_active_tasks,omitempty" json:"max_active_tasks,omitempty"`
	TaskTimeout    string            `yaml:"task_timeout,omitempty" json:"task_timeout,omitempty"`
	ChatTimeout    string            `yaml:"chat_timeout,omitempty" json:"chat_timeout,omitempty"`
	Config         map[string]string `yaml:"config,omitempty" json:"config,omitempty"`
}

// RoleDefinition represents a role definition in YAML
type RoleDefinition struct {
	Description   string   `yaml:"description" json:"description"`
	IsSupervisor  bool     `yaml:"is_supervisor,omitempty" json:"is_supervisor,omitempty"`
	Subordinates  []string `yaml:"subordinates,omitempty" json:"subordinates,omitempty"`
	AcceptsChats  bool     `yaml:"accepts_chats,omitempty" json:"accepts_chats,omitempty"`
	AcceptsEvents []string `yaml:"accepts_events,omitempty" json:"accepts_events,omitempty"`
	AcceptsWork   []string `yaml:"accepts_work,omitempty" json:"accepts_work,omitempty"`
}

// TaskDefinition represents a task definition in YAML
type TaskDefinition struct {
	Name           string   `yaml:"name" json:"name"`
	Description    string   `yaml:"description" json:"description"`
	ExpectedOutput string   `yaml:"expected_output,omitempty" json:"expected_output,omitempty"`
	OutputFormat   string   `yaml:"output_format,omitempty" json:"output_format,omitempty"`
	AssignedAgent  string   `yaml:"assigned_agent,omitempty" json:"assigned_agent,omitempty"`
	Dependencies   []string `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	MaxIterations  *int     `yaml:"max_iterations,omitempty" json:"max_iterations,omitempty"`
	OutputFile     string   `yaml:"output_file,omitempty" json:"output_file,omitempty"`
	Timeout        string   `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Context        string   `yaml:"context,omitempty" json:"context,omitempty"`
	Kind           string   `yaml:"kind,omitempty" json:"kind,omitempty"`
}

// ToolDefinition represents a tool definition
type ToolDefinition map[string]interface{}

// LoadDefinitionYAML loads a YAML team definition from a file
func LoadDefinitionYAML(filePath string) (*TeamDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	var def TeamDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	return &def, nil
}

// BuildTeam builds a Team from a TeamDefinition
func BuildTeam(ctx context.Context, def *TeamDefinition) (*DiveTeam, []*Task, error) {
	// Set up default configuration
	logLevel := "info"
	if def.Config.LogLevel != "" {
		logLevel = def.Config.LogLevel
	}

	// Create logger
	logger := slogger.New(slogger.LevelFromString(logLevel))

	var enabledTools []string
	var toolConfigs map[string]map[string]interface{}

	if def.Config.EnabledTools != nil {
		enabledTools = def.Config.EnabledTools
	}

	if def.Tools != nil {
		toolConfigs = make(map[string]map[string]interface{}, len(def.Tools))
		for _, toolDef := range def.Tools {
			name, ok := toolDef["name"].(string)
			if !ok {
				return nil, nil, fmt.Errorf("tool name is missing")
			}
			toolConfigs[name] = toolDef
		}
	}

	// Initialize tools
	toolsMap, err := initializeToolsWithConfig(enabledTools, toolConfigs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize tools: %w", err)
	}

	// Create agents
	agents := make([]Agent, 0, len(def.Agents))
	for _, agentDef := range def.Agents {
		agent, err := buildAgent(agentDef, def.Config, toolsMap, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build agent %s: %w", agentDef.Name, err)
		}
		agents = append(agents, agent)
	}

	// Create tasks
	tasks := make([]*Task, 0, len(def.Tasks))
	for _, taskDef := range def.Tasks {
		task, err := buildTask(taskDef, agents)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build task %s: %w", taskDef.Name, err)
		}
		tasks = append(tasks, task)
	}

	// Create team
	team, err := NewTeam(TeamOptions{
		Name:        def.Name,
		Description: def.Description,
		Agents:      agents,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create team: %w", err)
	}
	return team, tasks, nil
}

// Helper functions

func buildAgent(agentDef AgentDefinition, globalConfig ConfigDefinition, toolsMap map[string]llm.Tool, logger slogger.Logger) (Agent, error) {
	// Determine provider and model
	provider := agentDef.Provider
	if provider == "" {
		provider = globalConfig.DefaultProvider
		if provider == "" {
			provider = "anthropic" // Default provider
		}
	}

	model := agentDef.Model
	if model == "" {
		model = globalConfig.DefaultModel
	}

	// Create LLM provider
	var llmProvider llm.LLM
	switch provider {
	case "anthropic":
		opts := []anthropic.Option{}
		if model != "" {
			opts = append(opts, anthropic.WithModel(model))
		}
		llmProvider = anthropic.New(opts...)
	case "openai":
		opts := []openai.Option{}
		if model != "" {
			opts = append(opts, openai.WithModel(model))
		}
		llmProvider = openai.New(opts...)
	case "groq":
		opts := []groq.Option{}
		if model != "" {
			opts = append(opts, groq.WithModel(model))
		}
		llmProvider = groq.New(opts...)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	// Collect tools for this agent
	var agentTools []llm.Tool
	for _, toolName := range agentDef.Tools {
		tool, ok := toolsMap[toolName]
		if !ok {
			return nil, fmt.Errorf("tool %s not found or not enabled", toolName)
		}
		agentTools = append(agentTools, tool)
	}

	// Parse timeouts
	var taskTimeout, chatTimeout time.Duration
	if agentDef.TaskTimeout != "" {
		var err error
		taskTimeout, err = time.ParseDuration(agentDef.TaskTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid task timeout: %w", err)
		}
	}

	if agentDef.ChatTimeout != "" {
		var err error
		chatTimeout, err = time.ParseDuration(agentDef.ChatTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid chat timeout: %w", err)
		}
	}

	// Set cache control
	cacheControl := agentDef.CacheControl
	if cacheControl == "" {
		cacheControl = globalConfig.CacheControl
		if cacheControl == "" {
			cacheControl = "ephemeral" // Default cache control
		}
	}

	// Create agent
	agent := NewAgent(AgentOptions{
		Name: agentDef.Name,
		Role: Role{
			Description:   agentDef.Role.Description,
			IsSupervisor:  agentDef.Role.IsSupervisor,
			Subordinates:  agentDef.Role.Subordinates,
			AcceptsChats:  agentDef.Role.AcceptsChats,
			AcceptsEvents: agentDef.Role.AcceptsEvents,
			AcceptsWork:   agentDef.Role.AcceptsWork,
		},
		LLM:            llmProvider,
		Tools:          agentTools,
		MaxActiveTasks: agentDef.MaxActiveTasks,
		TaskTimeout:    taskTimeout,
		ChatTimeout:    chatTimeout,
		CacheControl:   cacheControl,
		LogLevel:       globalConfig.LogLevel,
		Logger:         logger,
	})

	return agent, nil
}

func buildTask(taskDef TaskDefinition, agents []Agent) (*Task, error) {
	// Parse timeout
	var timeout time.Duration
	if taskDef.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(taskDef.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
	}

	// Find assigned agent if specified
	var assignedAgent Agent
	if taskDef.AssignedAgent != "" {
		for _, agent := range agents {
			if agent.Name() == taskDef.AssignedAgent {
				assignedAgent = agent
				break
			}
		}
		if assignedAgent == nil {
			return nil, fmt.Errorf("assigned agent %s not found", taskDef.AssignedAgent)
		}
	}

	// Parse output format
	var outputFormat OutputFormat
	if taskDef.OutputFormat != "" {
		outputFormat = OutputFormat(taskDef.OutputFormat)
	}

	// Create task
	task := NewTask(TaskOptions{
		Name:           taskDef.Name,
		Description:    taskDef.Description,
		ExpectedOutput: taskDef.ExpectedOutput,
		OutputFormat:   outputFormat,
		AssignedAgent:  assignedAgent,
		Dependencies:   taskDef.Dependencies,
		MaxIterations:  taskDef.MaxIterations,
		OutputFile:     taskDef.OutputFile,
		Timeout:        timeout,
		Context:        taskDef.Context,
		Kind:           taskDef.Kind,
	})

	return task, nil
}
