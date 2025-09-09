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
	"github.com/deepnoodle-ai/dive/slogger"
)

func buildAgent(
	ctx context.Context,
	baseDir string,
	agentDef Agent,
	config Config,
	tools map[string]dive.Tool,
	logger slogger.Logger,
	confirmer dive.Confirmer,
	basePath string,
) (dive.Agent, error) {
	providerName := agentDef.Provider
	if providerName == "" {
		providerName = config.DefaultProvider
		if providerName == "" {
			providerName = "anthropic"
		}
	}

	modelName := agentDef.Model
	if modelName == "" {
		modelName = config.DefaultModel
	}

	providerConfigByName := make(map[string]*Provider)
	for _, p := range config.Providers {
		providerConfigByName[p.Name] = &p
	}
	providerConfig := providerConfigByName[providerName]

	model, err := GetModel(providerName, modelName)
	if err != nil {
		return nil, fmt.Errorf("error getting model: %w", err)
	}

	var agentTools []dive.Tool
	for _, toolName := range agentDef.Tools {
		tool, ok := tools[toolName]
		if !ok {
			return nil, fmt.Errorf("agent references unknown tool %q", toolName)
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
		modelSettings = &agent.ModelSettings{
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

		if providerConfig != nil {
			modelSettings.Caching = providerConfig.Caching
		} else if agentDef.ModelSettings.Caching != nil {
			modelSettings.Caching = agentDef.ModelSettings.Caching
		}

		// Combine enabled features from provider and agent
		featuresByName := make(map[string]bool)
		if providerConfig != nil {
			for _, feature := range providerConfig.Features {
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
		modelSettings.Features = features

		// Combine request headers from provider and agent
		requestHeaders := make(http.Header)
		if providerConfig != nil {
			for key, value := range providerConfig.RequestHeaders {
				requestHeaders.Add(key, value)
			}
		}
		for key, value := range agentDef.ModelSettings.RequestHeaders {
			requestHeaders.Add(key, value)
		}
		modelSettings.RequestHeaders = requestHeaders

		// Note: MCP servers are configured at the environment level, not agent level
	}

	// Build static context messages if provided
	var contextContent []llm.Content
	if len(agentDef.Context) > 0 {
		var err error
		contextContent, err = buildContextContent(ctx, baseDir, basePath, agentDef.Context)
		if err != nil {
			return nil, fmt.Errorf("error building agent context: %w", err)
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
		ResponseTimeout:      responseTimeout,
		ToolIterationLimit:   agentDef.ToolIterationLimit,
		DateAwareness:        agentDef.DateAwareness,
		SystemPromptTemplate: agentDef.SystemPrompt,
		ModelSettings:        modelSettings,
		Logger:               logger,
		Confirmer:            confirmer,
		Context:              contextContent,
	})
}
