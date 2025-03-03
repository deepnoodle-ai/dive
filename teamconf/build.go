package teamconf

import (
	"context"
	"fmt"
	"time"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/providers/anthropic"
	"github.com/getstingrai/dive/providers/groq"
	"github.com/getstingrai/dive/providers/openai"
	"github.com/getstingrai/dive/slogger"
)

// Build builds a dive.Team from a team configuration
func Build(ctx context.Context, def *Team) (*dive.DiveTeam, []*dive.Task, error) {
	logLevel := "info"
	if def.Config.LogLevel != "" {
		logLevel = def.Config.LogLevel
	}
	logger := slogger.New(slogger.LevelFromString(logLevel))

	var enabledTools []string
	var toolConfigs map[string]map[string]interface{}

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

	toolsMap, err := initializeTools(enabledTools, toolConfigs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize tools: %w", err)
	}

	agents := make([]dive.Agent, 0, len(def.Agents))
	for _, agentDef := range def.Agents {
		agent, err := buildAgent(agentDef, def.Config, toolsMap, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build agent %s: %w", agentDef.Name, err)
		}
		agents = append(agents, agent)
	}

	tasks := make([]*dive.Task, 0, len(def.Tasks))
	for _, taskDef := range def.Tasks {
		task, err := buildTask(taskDef, agents)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build task %s: %w", taskDef.Name, err)
		}
		tasks = append(tasks, task)
	}

	team, err := dive.NewTeam(dive.TeamOptions{
		Name:        def.Name,
		Description: def.Description,
		Agents:      agents,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create team: %w", err)
	}
	return team, tasks, nil
}

func buildAgent(agentDef Agent, globalConfig Config, toolsMap map[string]llm.Tool, logger slogger.Logger) (dive.Agent, error) {
	provider := agentDef.Provider
	if provider == "" {
		provider = globalConfig.DefaultProvider
		if provider == "" {
			provider = "anthropic"
		}
	}

	model := agentDef.Model
	if model == "" {
		model = globalConfig.DefaultModel
	}

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
		return nil, fmt.Errorf("unsupported provider: %q", provider)
	}

	var agentTools []llm.Tool
	for _, toolName := range agentDef.Tools {
		tool, ok := toolsMap[toolName]
		if !ok {
			return nil, fmt.Errorf("tool %q not found or not enabled", toolName)
		}
		agentTools = append(agentTools, tool)
	}

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

	cacheControl := agentDef.CacheControl
	if cacheControl == "" {
		cacheControl = globalConfig.CacheControl
	}

	agent := dive.NewAgent(dive.AgentOptions{
		Name:           agentDef.Name,
		Description:    agentDef.Description,
		Instructions:   agentDef.Instructions,
		IsSupervisor:   agentDef.IsSupervisor,
		Subordinates:   agentDef.Subordinates,
		AcceptedEvents: agentDef.AcceptedEvents,
		LLM:            llmProvider,
		Tools:          agentTools,
		TaskTimeout:    taskTimeout,
		ChatTimeout:    chatTimeout,
		CacheControl:   cacheControl,
		LogLevel:       globalConfig.LogLevel,
		Logger:         logger,
	})
	return agent, nil
}

func buildTask(taskDef Task, agents []dive.Agent) (*dive.Task, error) {
	var timeout time.Duration
	if taskDef.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(taskDef.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
	}

	// Find assigned agent if specified
	var assignedAgent dive.Agent
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

	return dive.NewTask(dive.TaskOptions{
		Name:           taskDef.Name,
		Description:    taskDef.Description,
		ExpectedOutput: taskDef.ExpectedOutput,
		Dependencies:   taskDef.Dependencies,
		OutputFormat:   dive.OutputFormat(taskDef.OutputFormat),
		AssignedAgent:  assignedAgent,
		OutputFile:     taskDef.OutputFile,
		Timeout:        timeout,
		Context:        taskDef.Context,
	}), nil
}
