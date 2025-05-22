package config

import (
	"fmt"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/slogger"
)

func buildAgent(agentDef Agent, config Config, tools map[string]dive.Tool, logger slogger.Logger, confirmer dive.Confirmer) (dive.Agent, error) {
	providerName := agentDef.Provider
	if providerName == "" {
		providerName = config.LLM.DefaultProvider
		if providerName == "" {
			providerName = "anthropic"
		}
	}

	modelName := agentDef.Model
	if modelName == "" {
		modelName = config.LLM.DefaultModel
	}

	model, err := GetModel(providerName, modelName)
	if err != nil {
		return nil, fmt.Errorf("error getting model: %w", err)
	}

	var agentTools []dive.Tool
	for _, toolName := range agentDef.Tools {
		tool, ok := tools[toolName]
		if !ok {
			return nil, fmt.Errorf("tool %q not found or not enabled", toolName)
		}
		agentTools = append(agentTools, tool)
	}

	var responseTimeout time.Duration
	if agentDef.ResponseTimeout != nil {
		var err error
		switch v := agentDef.ResponseTimeout.(type) {
		case string:
			responseTimeout, err = time.ParseDuration(v)
			if err != nil {
				return nil, fmt.Errorf("invalid response timeout: %w", err)
			}
		case int:
			responseTimeout = time.Duration(v) * time.Second
		case float64:
			responseTimeout = time.Duration(int64(v)) * time.Second
		default:
			return nil, fmt.Errorf("invalid response timeout: %v", v)
		}
	}

	var modelSettings *agent.ModelSettings
	if agentDef.ModelSettings != nil {
		var toolChoice llm.ToolChoice
		if agentDef.ModelSettings.ToolChoice != "" {
			toolChoice = llm.ToolChoice(agentDef.ModelSettings.ToolChoice)
			if !toolChoice.IsValid() {
				return nil, fmt.Errorf("invalid tool choice: %q", agentDef.ModelSettings.ToolChoice)
			}
		}
		modelSettings = &agent.ModelSettings{
			Temperature:       agentDef.ModelSettings.Temperature,
			PresencePenalty:   agentDef.ModelSettings.PresencePenalty,
			FrequencyPenalty:  agentDef.ModelSettings.FrequencyPenalty,
			ReasoningBudget:   agentDef.ModelSettings.ReasoningBudget,
			ReasoningEffort:   agentDef.ModelSettings.ReasoningEffort,
			MaxTokens:         agentDef.ModelSettings.MaxTokens,
			ParallelToolCalls: agentDef.ModelSettings.ParallelToolCalls,
			ToolChoice:        toolChoice,
		}
	}

	return agent.New(agent.Options{
		Name:                 agentDef.Name,
		Goal:                 agentDef.Goal,
		Instructions:         agentDef.Instructions,
		IsSupervisor:         agentDef.IsSupervisor,
		Subordinates:         agentDef.Subordinates,
		Model:                model,
		Tools:                agentTools,
		ToolChoice:           llm.ToolChoice(agentDef.ToolChoice),
		ResponseTimeout:      responseTimeout,
		ToolIterationLimit:   agentDef.ToolIterationLimit,
		DateAwareness:        agentDef.DateAwareness,
		SystemPromptTemplate: agentDef.SystemPrompt,
		ModelSettings:        modelSettings,
		Logger:               logger,
		Confirmer:            confirmer,
	})
}
