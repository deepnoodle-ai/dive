package google

import (
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
	"google.golang.org/genai"
)

type classificationSpec struct {
	entryPrefix string
	scopes      []modelcaps.VerificationScope
}

var (
	googleGeminiAPIScopes = []modelcaps.VerificationScope{
		modelcaps.VerificationGoogleGeminiAPI,
	}
	googleVertexAIScopes = []modelcaps.VerificationScope{
		modelcaps.VerificationGoogleVertexAI,
	}
)

var googleClassificationSpecs = map[string]classificationSpec{
	"gemini-3.7-flash":                   {entryPrefix: "gemini-3.7-flash", scopes: googleVertexAIScopes},
	"gemini-3.6-flash":                   {entryPrefix: "gemini-3.6-flash", scopes: googleGeminiAPIScopes},
	"gemini-3.5-flash":                   {entryPrefix: "gemini-3.5-flash", scopes: googleGeminiAPIScopes},
	"gemini-3.5-flash-lite":              {entryPrefix: "gemini-3.5-flash-lite", scopes: googleGeminiAPIScopes},
	"gemini-3.1-pro-preview":             {entryPrefix: "gemini-3.1-pro", scopes: googleGeminiAPIScopes},
	"gemini-3.1-pro-preview-customtools": {entryPrefix: "gemini-3.1-pro", scopes: googleGeminiAPIScopes},
	"gemini-3.1-flash-lite":              {entryPrefix: "gemini-3.1-flash-lite", scopes: googleGeminiAPIScopes},
	"gemini-3.1-flash-lite-preview":      {entryPrefix: "gemini-3.1-flash-lite", scopes: googleGeminiAPIScopes},
	"gemini-3-flash-preview":             {entryPrefix: "gemini-3-flash", scopes: googleGeminiAPIScopes},
	"gemini-3-pro-preview":               {entryPrefix: "gemini-3-pro-preview"},
	"gemini-2.5-pro":                     {entryPrefix: "gemini-2.5-pro", scopes: googleGeminiAPIScopes},
	"gemini-2.5-flash":                   {entryPrefix: "gemini-2.5-flash", scopes: googleGeminiAPIScopes},
	"gemini-2.5-flash-lite":              {entryPrefix: "gemini-2.5-flash-lite", scopes: googleGeminiAPIScopes},
	"gemini-2.0-flash":                   {entryPrefix: "gemini-2.0-flash"},
	"gemini-1.5-pro":                     {entryPrefix: "gemini-1.5-pro"},
	"gemini-1.5-flash":                   {entryPrefix: "gemini-1.5-flash"},
}

var budgetEmulatedEfforts = []llm.ReasoningEffort{
	llm.ReasoningEffortMinimal,
	llm.ReasoningEffortLow,
	llm.ReasoningEffortMedium,
	llm.ReasoningEffortHigh,
	llm.ReasoningEffortXHigh,
	llm.ReasoningEffortMax,
}

type requestControlPlan struct {
	thinking    *genai.ThinkingConfig
	temperature *float64
	explanation modelcaps.Plan
}

func init() {
	modelcaps.MustRegister("google", modelcaps.Resolver{
		Classify: classifyModelControls,
		Explain:  explainModelControls,
	})
}

func classifyModelControls(model string) (modelcaps.Classification, bool) {
	canonical := normalizeClassificationModel(model)
	spec, ok := googleClassificationSpecs[canonical]
	if !ok {
		return modelcaps.Classification{}, false
	}
	entry, ok := lookupEntry(canonical)
	if !ok || entry.prefix != spec.entryPrefix {
		return modelcaps.Classification{}, false
	}

	caps := entry.caps
	reasoning := modelcaps.ReasoningClassification{
		NativeEfforts:      caps.efforts,
		AdaptiveThinking:   !caps.unverified && caps.maxBudget > 0,
		CanDisableThinking: caps.canDisableThinking,
	}
	if !caps.unverified && caps.maxBudget > 0 {
		minimum, maximum := caps.minBudget, caps.maxBudget
		reasoning.Budget = &modelcaps.ReasoningBudgetClassification{
			Minimum: &minimum,
			Maximum: &maximum,
		}
		if !caps.supportsThinkingLevel() {
			reasoning.EmulatedEfforts = budgetEmulatedEfforts
		}
	}
	scopes := spec.scopes
	if caps.unverified {
		scopes = nil
	}
	return modelcaps.Classification{
		Model:              canonical,
		Temperature:        modelAcceptsTemperature(canonical),
		Reasoning:          reasoning,
		VerificationScopes: scopes,
	}, true
}

func explainModelControls(config llm.Config) (modelcaps.Plan, bool) {
	classification, ok := classifyModelControls(config.Model)
	if !ok {
		return modelcaps.Plan{}, false
	}
	config.Model = classification.Model
	config.Logger = nil
	return planRequestControls(config.Model, &config).explanation, true
}

func planRequestControls(model string, config *llm.Config) requestControlPlan {
	thinking := buildThinkingConfig(model, config)
	var temperature *float64
	if modelAcceptsTemperature(model) {
		temperature = config.Temperature
	} else if config.Temperature != nil {
		warnf(config, "temperature is not supported by this Google model and will be ignored",
			"model", model)
	}
	return requestControlPlan{
		thinking:    thinking,
		temperature: temperature,
		explanation: projectControlPlan(model, config, thinking, temperature),
	}
}

func projectControlPlan(
	model string,
	config *llm.Config,
	thinking *genai.ThinkingConfig,
	temperature *float64,
) modelcaps.Plan {
	plan := modelcaps.Plan{
		Model:       model,
		Effort:      modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
		Budget:      modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
		Thinking:    modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
		Temperature: modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
	}

	budget, hasBudget := thinkingBudget(thinking)
	level, hasLevel := thinkingEffort(thinking)
	switch {
	case hasBudget && budget == dynamicThinkingBudget:
		plan.Effective.Thinking = llm.ThinkingTypeAdaptive
	case hasBudget && budget == 0:
		plan.Effective.Thinking = llm.ThinkingTypeDisabled
	case hasBudget:
		value := budget
		plan.Effective.ReasoningBudget = &value
		plan.Effective.Thinking = llm.ThinkingTypeEnabled
	case hasLevel:
		plan.Effective.ReasoningEffort = level
		if config.Thinking == llm.ThinkingTypeDisabled {
			plan.Effective.Thinking = llm.ThinkingTypeEnabled
		}
	case thinking != nil && thinking.IncludeThoughts && config.Thinking == llm.ThinkingTypeEnabled:
		plan.Effective.Thinking = llm.ThinkingTypeEnabled
	}
	if temperature != nil {
		value := *temperature
		plan.Effective.Temperature = &value
	}

	thinkingOff := config.Thinking == llm.ThinkingTypeDisabled ||
		config.ReasoningEffort == llm.ReasoningEffortNone
	if config.ReasoningEffort != "" {
		switch {
		case config.ReasoningEffort == llm.ReasoningEffortNone && hasBudget && budget == 0:
			plan.Effort = modelcaps.ControlDecision{Action: modelcaps.ControlEmulated}
		case config.ReasoningEffort == llm.ReasoningEffortNone && (hasBudget || hasLevel):
			plan.Effort = modelcaps.ControlDecision{Action: modelcaps.ControlEmulated, Adjusted: true}
		case config.ReasoningBudget != nil:
			plan.Effort = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "explicit reasoning budget takes precedence",
			}
		case hasLevel:
			plan.Effort = modelcaps.ControlDecision{
				Action:   modelcaps.ControlApplied,
				Adjusted: level != config.ReasoningEffort,
			}
		case hasBudget:
			mapped := effortBudget(config.ReasoningEffort)
			plan.Effort = modelcaps.ControlDecision{
				Action:   modelcaps.ControlEmulated,
				Adjusted: budget != mapped,
			}
		default:
			plan.Effort = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "reasoning effort has no effect on the final request",
			}
		}
	}

	if config.ReasoningBudget != nil {
		switch {
		case thinkingOff:
			plan.Budget = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "thinking disable takes precedence",
			}
		case hasBudget && budget == dynamicThinkingBudget:
			plan.Budget = modelcaps.ControlDecision{
				Action: modelcaps.ControlEmulated,
				Reason: "dynamic budget is represented as adaptive thinking",
			}
		case hasBudget:
			plan.Budget = modelcaps.ControlDecision{
				Action:   modelcaps.ControlApplied,
				Adjusted: budget != *config.ReasoningBudget,
			}
		default:
			plan.Budget = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "reasoning budget has no effect on the final request",
			}
		}
	}

	switch config.Thinking {
	case "":
	case llm.ThinkingTypeDisabled:
		if hasBudget && budget == 0 {
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlApplied}
		} else if hasBudget || hasLevel {
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlEmulated, Adjusted: true}
		} else {
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlOmitted}
		}
	case llm.ThinkingTypeAdaptive:
		if hasBudget && budget == dynamicThinkingBudget {
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlApplied}
		} else {
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlOmitted}
		}
	case llm.ThinkingTypeEnabled:
		if thinking != nil {
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlApplied}
		} else {
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlOmitted}
		}
	default:
		plan.Thinking = modelcaps.ControlDecision{
			Action: modelcaps.ControlOmitted,
			Reason: "unrecognized thinking control",
		}
	}

	if config.Temperature != nil {
		if temperature != nil {
			plan.Temperature = modelcaps.ControlDecision{Action: modelcaps.ControlApplied}
		} else {
			plan.Temperature = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "temperature is not meaningful for this model",
			}
		}
	}
	return plan
}

func thinkingBudget(thinking *genai.ThinkingConfig) (int, bool) {
	if thinking == nil || thinking.ThinkingBudget == nil {
		return 0, false
	}
	return int(*thinking.ThinkingBudget), true
}

func thinkingEffort(thinking *genai.ThinkingConfig) (llm.ReasoningEffort, bool) {
	if thinking == nil {
		return "", false
	}
	switch thinking.ThinkingLevel {
	case genai.ThinkingLevelMinimal:
		return llm.ReasoningEffortMinimal, true
	case genai.ThinkingLevelLow:
		return llm.ReasoningEffortLow, true
	case genai.ThinkingLevelMedium:
		return llm.ReasoningEffortMedium, true
	case genai.ThinkingLevelHigh:
		return llm.ReasoningEffortHigh, true
	default:
		return "", false
	}
}

func normalizeClassificationModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.TrimPrefix(model, "models/")
}
