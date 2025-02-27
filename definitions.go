package dive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/providers/anthropic"
	"github.com/getstingrai/dive/providers/groq"
	"github.com/getstingrai/dive/providers/openai"
	"github.com/getstingrai/dive/slogger"
	"gopkg.in/yaml.v3"
)

// TeamDefinition is a serializable representation of a Team
type TeamDefinition struct {
	Name        string            `yaml:"name,omitempty" json:"name,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Agents      []AgentDefinition `yaml:"agents,omitempty" json:"agents,omitempty"`
	Tasks       []TaskDefinition  `yaml:"tasks,omitempty" json:"tasks,omitempty"`
	Tools       []ToolDefinition  `yaml:"tools,omitempty" json:"tools,omitempty"`
	Config      ConfigDefinition  `yaml:"config,omitempty" json:"config,omitempty"`
}

// ConfigDefinition is a serializable representation of global configuration
type ConfigDefinition struct {
	DefaultProvider string            `yaml:"default_provider,omitempty" json:"default_provider,omitempty"`
	DefaultModel    string            `yaml:"default_model,omitempty" json:"default_model,omitempty"`
	LogLevel        string            `yaml:"log_level,omitempty" json:"log_level,omitempty"`
	CacheControl    string            `yaml:"cache_control,omitempty" json:"cache_control,omitempty"`
	EnabledTools    []string          `yaml:"enabled_tools,omitempty" json:"enabled_tools,omitempty"`
	ProviderConfigs map[string]string `yaml:"provider_configs,omitempty" json:"provider_configs,omitempty"`
}

// AgentDefinition is a serializable representation of an Agent
type AgentDefinition struct {
	Name           string            `yaml:"name,omitempty" json:"name,omitempty"`
	Role           RoleDefinition    `yaml:"role,omitempty" json:"role,omitempty"`
	Provider       string            `yaml:"provider,omitempty" json:"provider,omitempty"`
	Model          string            `yaml:"model,omitempty" json:"model,omitempty"`
	Tools          []string          `yaml:"tools,omitempty" json:"tools,omitempty"`
	CacheControl   string            `yaml:"cache_control,omitempty" json:"cache_control,omitempty"`
	MaxActiveTasks int               `yaml:"max_active_tasks,omitempty" json:"max_active_tasks,omitempty"`
	TaskTimeout    string            `yaml:"task_timeout,omitempty" json:"task_timeout,omitempty"`
	ChatTimeout    string            `yaml:"chat_timeout,omitempty" json:"chat_timeout,omitempty"`
	Config         map[string]string `yaml:"config,omitempty" json:"config,omitempty"`
}

// RoleDefinition is a serializable representation of a Role
type RoleDefinition struct {
	Description    string   `yaml:"description,omitempty" json:"description,omitempty"`
	IsSupervisor   bool     `yaml:"is_supervisor,omitempty" json:"is_supervisor,omitempty"`
	Subordinates   []string `yaml:"subordinates,omitempty" json:"subordinates,omitempty"`
	AcceptedEvents []string `yaml:"accepted_events,omitempty" json:"accepted_events,omitempty"`
}

// TaskDefinition is a serializable representation of a Task
type TaskDefinition struct {
	Name           string   `yaml:"name,omitempty" json:"name,omitempty"`
	Description    string   `yaml:"description,omitempty" json:"description,omitempty"`
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

// ToolDefinition used for serializing tool configurations
type ToolDefinition map[string]interface{}

// LoadDefinition loads a TeamDefinition from a file. If the file has a .json
// extension, it is parsed as JSON. Otherwise, it is parsed as YAML.
func LoadDefinition(filePath string) (*TeamDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	var def TeamDefinition
	if strings.HasSuffix(filePath, ".json") {
		if err := json.Unmarshal(data, &def); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &def); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
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
			Description:    agentDef.Role.Description,
			IsSupervisor:   agentDef.Role.IsSupervisor,
			Subordinates:   agentDef.Role.Subordinates,
			AcceptedEvents: agentDef.Role.AcceptedEvents,
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
