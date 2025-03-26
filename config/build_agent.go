package config

import (
	"fmt"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/diveagents/dive/llm/providers/groq"
	"github.com/diveagents/dive/llm/providers/openai"
	"github.com/diveagents/dive/slogger"
)

func buildAgent(
	agentDef Agent,
	config Config,
	tools map[string]llm.Tool,
	logger slogger.Logger,
) (dive.Agent, error) {
	providerName := agentDef.Provider
	if providerName == "" {
		providerName = config.LLM.DefaultProvider
		if providerName == "" {
			providerName = "anthropic"
		}
	}

	model := agentDef.Model
	if model == "" {
		model = config.LLM.DefaultModel
	}

	var llmProvider llm.LLM
	switch providerName {
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
		return nil, fmt.Errorf("unsupported provider: %q", providerName)
	}

	var agentTools []llm.Tool
	for _, toolName := range agentDef.Tools {
		tool, ok := tools[toolName]
		if !ok {
			return nil, fmt.Errorf("tool %q not found or not enabled", toolName)
		}
		agentTools = append(agentTools, tool)
	}

	var chatTimeout time.Duration
	if agentDef.ChatTimeout != "" {
		var err error
		chatTimeout, err = time.ParseDuration(agentDef.ChatTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid chat timeout: %w", err)
		}
	}

	cacheControl := agentDef.CacheControl
	if cacheControl == "" {
		cacheControl = config.LLM.CacheControl
	}

	return agent.New(agent.Options{
		Name:               agentDef.Name,
		Goal:               agentDef.Goal,
		Backstory:          agentDef.Backstory,
		IsSupervisor:       agentDef.IsSupervisor,
		Subordinates:       agentDef.Subordinates,
		AcceptedEvents:     agentDef.AcceptedEvents,
		LLM:                llmProvider,
		Tools:              agentTools,
		ChatTimeout:        chatTimeout,
		CacheControl:       cacheControl,
		Logger:             logger,
		ToolIterationLimit: agentDef.ToolIterationLimit,
	})
}
