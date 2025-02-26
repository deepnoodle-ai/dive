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
	"github.com/getstingrai/dive/tools"
	"github.com/getstingrai/dive/tools/google"
	"github.com/mendableai/firecrawl-go"
	"gopkg.in/yaml.v3"
)

// YAMLDefinition represents the top-level YAML structure
type YAMLDefinition struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description"`
	Agents      []YAMLAgent      `yaml:"agents"`
	Tasks       []YAMLTask       `yaml:"tasks"`
	Config      YAMLGlobalConfig `yaml:"config"`
}

// YAMLGlobalConfig contains global configuration settings
type YAMLGlobalConfig struct {
	DefaultProvider string            `yaml:"default_provider"`
	DefaultModel    string            `yaml:"default_model"`
	LogLevel        string            `yaml:"log_level"`
	CacheControl    string            `yaml:"cache_control"`
	EnabledTools    []string          `yaml:"enabled_tools"`
	ProviderConfigs map[string]string `yaml:"provider_configs"`
}

// YAMLAgent represents an agent definition in YAML
type YAMLAgent struct {
	Name           string            `yaml:"name"`
	Role           YAMLRole          `yaml:"role"`
	Provider       string            `yaml:"provider,omitempty"`
	Model          string            `yaml:"model,omitempty"`
	Tools          []string          `yaml:"tools,omitempty"`
	CacheControl   string            `yaml:"cache_control,omitempty"`
	MaxActiveTasks int               `yaml:"max_active_tasks,omitempty"`
	TaskTimeout    string            `yaml:"task_timeout,omitempty"`
	ChatTimeout    string            `yaml:"chat_timeout,omitempty"`
	Config         map[string]string `yaml:"config,omitempty"`
}

// YAMLRole represents a role definition in YAML
type YAMLRole struct {
	Description   string   `yaml:"description"`
	IsSupervisor  bool     `yaml:"is_supervisor,omitempty"`
	Subordinates  []string `yaml:"subordinates,omitempty"`
	AcceptsChats  bool     `yaml:"accepts_chats,omitempty"`
	AcceptsEvents []string `yaml:"accepts_events,omitempty"`
	AcceptsWork   []string `yaml:"accepts_work,omitempty"`
}

// YAMLTask represents a task definition in YAML
type YAMLTask struct {
	Name           string   `yaml:"name"`
	Description    string   `yaml:"description"`
	ExpectedOutput string   `yaml:"expected_output,omitempty"`
	OutputFormat   string   `yaml:"output_format,omitempty"`
	AssignedAgent  string   `yaml:"assigned_agent,omitempty"`
	Dependencies   []string `yaml:"dependencies,omitempty"`
	MaxIterations  *int     `yaml:"max_iterations,omitempty"`
	OutputFile     string   `yaml:"output_file,omitempty"`
	Timeout        string   `yaml:"timeout,omitempty"`
	Context        string   `yaml:"context,omitempty"`
	Kind           string   `yaml:"kind,omitempty"`
}

// LoadDefinition loads a YAML definition from a file
func LoadDefinition(filePath string) (*YAMLDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var def YAMLDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &def, nil
}

// BuildTeam builds a Team from a YAML definition
func BuildTeam(ctx context.Context, def *YAMLDefinition) (*DiveTeam, []*Task, error) {
	// Set up default configuration
	logLevel := "info"
	if def.Config.LogLevel != "" {
		logLevel = def.Config.LogLevel
	}

	// Create logger
	logger := slogger.New(slogger.LevelFromString(logLevel))

	// Initialize tools
	toolsMap, err := initializeTools(def.Config.EnabledTools, logger)
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

// LoadAndRunTeam loads a YAML definition and runs the team
func LoadAndRunTeam(ctx context.Context, filePath string) ([]*TaskResult, error) {
	def, err := LoadDefinition(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load definition: %w", err)
	}

	team, tasks, err := BuildTeam(ctx, def)
	if err != nil {
		return nil, fmt.Errorf("failed to build team: %w", err)
	}

	if err := team.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start team: %w", err)
	}
	defer team.Stop(ctx)

	results, err := team.Work(ctx, tasks...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute work: %w", err)
	}
	return results, nil
}

// Helper functions

func buildAgent(agentDef YAMLAgent, globalConfig YAMLGlobalConfig, toolsMap map[string]llm.Tool, logger slogger.Logger) (Agent, error) {
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

func buildTask(taskDef YAMLTask, agents []Agent) (*Task, error) {
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

func initializeTools(enabledTools []string, logger slogger.Logger) (map[string]llm.Tool, error) {
	toolsMap := make(map[string]llm.Tool)

	// Create a set of enabled tools for quick lookup
	enabledToolsSet := make(map[string]bool)
	for _, tool := range enabledTools {
		enabledToolsSet[tool] = true
	}

	// Initialize Google Search if enabled
	if enabledToolsSet["google_search"] {
		if key := os.Getenv("GOOGLE_SEARCH_CX"); key != "" {
			googleClient, err := google.New()
			if err != nil {
				return nil, fmt.Errorf("failed to initialize Google Search: %w", err)
			}
			toolsMap["google_search"] = tools.NewGoogleSearch(googleClient)
			logger.Info("google search enabled")
		} else {
			logger.Warn("google search requested but GOOGLE_SEARCH_CX not set")
		}
	}

	// Initialize Firecrawl if enabled
	if enabledToolsSet["firecrawl"] {
		if key := os.Getenv("FIRECRAWL_API_KEY"); key != "" {
			app, err := firecrawl.NewFirecrawlApp(key, "")
			if err != nil {
				return nil, fmt.Errorf("failed to initialize Firecrawl: %w", err)
			}
			toolsMap["firecrawl"] = tools.NewFirecrawlScraper(app, 30000)
			logger.Info("firecrawl enabled")
		} else {
			logger.Warn("firecrawl requested but FIRECRAWL_API_KEY not set")
		}
	}

	if enabledToolsSet["file_read"] {
		toolsMap["file_read"] = tools.NewFileReadTool(tools.FileReadToolOptions{
			DefaultFilePath: "",
			MaxSize:         1024 * 200,
			RootDirectory:   "",
		})
		logger.Info("file_read enabled")
	}

	if enabledToolsSet["file_write"] {
		toolsMap["file_write"] = tools.NewFileWriteTool(tools.FileWriteToolOptions{
			DefaultFilePath: "",
			AllowList:       []string{},
			DenyList:        []string{},
			RootDirectory:   "",
		})
		logger.Info("file_write enabled")
	}

	if enabledToolsSet["directory_list"] {
		toolsMap["directory_list"] = tools.NewDirectoryListTool(tools.DirectoryListToolOptions{
			DefaultPath:   "",
			MaxEntries:    200,
			RootDirectory: "",
		})
	}

	// Add more tools here as needed

	return toolsMap, nil
}
