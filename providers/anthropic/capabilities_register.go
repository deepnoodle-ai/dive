package anthropic

import (
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
)

type classificationSpec struct {
	entryPrefix string
	scopes      []modelcaps.VerificationScope
}

var anthropicVerificationScopes = []modelcaps.VerificationScope{
	modelcaps.VerificationAnthropicMessages,
}

var anthropicClassificationSpecs = map[string]classificationSpec{
	"claude-3-5-haiku-20241022":  {entryPrefix: "claude-3-5-haiku"},
	"claude-3-5-sonnet-20241022": {entryPrefix: "claude-3-5-sonnet"},
	"claude-3-7-sonnet-20250219": {entryPrefix: "claude-3-7-sonnet"},
	"claude-sonnet-4-20250514":   {entryPrefix: "claude-sonnet-4"},
	"claude-opus-4-20250514":     {entryPrefix: "claude-opus-4"},
	"claude-opus-4-1-20250805":   {entryPrefix: "claude-opus-4-1"},
	"claude-haiku-4-5-20251001":  {entryPrefix: "claude-haiku-4-5", scopes: anthropicVerificationScopes},
	"claude-sonnet-4-5-20250929": {entryPrefix: "claude-sonnet-4-5", scopes: anthropicVerificationScopes},
	"claude-opus-4-5-20251101":   {entryPrefix: "claude-opus-4-5", scopes: anthropicVerificationScopes},
	"claude-haiku-4-5":           {entryPrefix: "claude-haiku-4-5", scopes: anthropicVerificationScopes},
	"claude-sonnet-4-5":          {entryPrefix: "claude-sonnet-4-5", scopes: anthropicVerificationScopes},
	"claude-opus-4-5":            {entryPrefix: "claude-opus-4-5", scopes: anthropicVerificationScopes},
	"claude-sonnet-4-6":          {entryPrefix: "claude-sonnet-4-6", scopes: anthropicVerificationScopes},
	"claude-opus-4-6":            {entryPrefix: "claude-opus-4-6", scopes: anthropicVerificationScopes},
	"claude-opus-4-7":            {entryPrefix: "claude-opus-4-7", scopes: anthropicVerificationScopes},
	"claude-opus-4-8":            {entryPrefix: "claude-opus-4-8", scopes: anthropicVerificationScopes},
	"claude-opus-5":              {entryPrefix: "claude-opus-5", scopes: anthropicVerificationScopes},
	"claude-fable-5":             {entryPrefix: "claude-fable-5", scopes: anthropicVerificationScopes},
	"claude-mythos-5":            {entryPrefix: "claude-mythos-5"},
	"claude-sonnet-5":            {entryPrefix: "claude-sonnet-5", scopes: anthropicVerificationScopes},
}

var legacyEmulatedEfforts = []llm.ReasoningEffort{
	llm.ReasoningEffortMinimal,
	llm.ReasoningEffortLow,
	llm.ReasoningEffortMedium,
	llm.ReasoningEffortHigh,
	llm.ReasoningEffortXHigh,
	llm.ReasoningEffortMax,
}

type requestControlPlan struct {
	thinking     *Thinking
	outputConfig *OutputConfig
	temperature  *float64
	explanation  modelcaps.Plan
	err          error
}

func init() {
	modelcaps.MustRegister("anthropic", modelcaps.Resolver{
		Classify: classifyModelControls,
		Explain:  explainModelControls,
	})
}

func classifyModelControls(model string) (modelcaps.Classification, bool) {
	canonical := normalizeClassificationModel(model)
	spec, ok := anthropicClassificationSpecs[canonical]
	if !ok {
		return modelcaps.Classification{}, false
	}
	entry, ok := lookupCapabilityEntry(canonical)
	if !ok || entry.prefix != spec.entryPrefix {
		return modelcaps.Classification{}, false
	}

	caps := entry.caps
	reasoning := modelcaps.ReasoningClassification{
		NativeEfforts:      caps.efforts,
		AdaptiveThinking:   caps.adaptive,
		CanDisableThinking: canDisableThinking(caps),
	}
	if caps.manualBudget {
		minimum := minThinkingBudget
		reasoning.Budget = &modelcaps.ReasoningBudgetClassification{Minimum: &minimum}
	}
	if caps.reasoningKind() == reasoningLegacyBudget {
		reasoning.EmulatedEfforts = legacyEmulatedEfforts
	}
	scopes := spec.scopes
	if entry.notLiveProbed {
		scopes = nil
	}
	return modelcaps.Classification{
		Model:              canonical,
		Temperature:        caps.temperature,
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
	maxTokens := config.MaxTokens
	if maxTokens == nil {
		value := DefaultMaxTokens
		maxTokens = &value
	}
	return planRequestControls(config.Model, maxTokens, &config).explanation, true
}

func planRequestControls(model string, maxTokens *int, config *llm.Config) requestControlPlan {
	controlRequest := Request{Model: model, MaxTokens: maxTokens}
	if err := applyReasoningConfig(&controlRequest, config); err != nil {
		return rejectedControlPlan(model, err)
	}
	if requestHasThinkingEnabled(model, controlRequest.Thinking) && config.Prefill != "" {
		return rejectedControlPlan(model,
			fmt.Errorf("anthropic extended thinking cannot be used with prefilled assistant responses"))
	}
	if config.ToolChoice != nil && len(config.Tools) > 0 &&
		requestThinkingBlocksForcedToolChoice(model, controlRequest.Thinking) &&
		forcedToolChoice(config.ToolChoice.Type) {
		return rejectedControlPlan(model,
			fmt.Errorf("anthropic extended thinking only supports tool_choice auto or none; got %q", config.ToolChoice.Type))
	}
	if modelAcceptsTemperature(model) && !requestHasThinkingEnabled(model, controlRequest.Thinking) {
		controlRequest.Temperature = config.Temperature
	} else if config.Temperature != nil {
		warnf(config, "temperature is not supported by this Anthropic request and will be ignored",
			"model", model)
	}

	return requestControlPlan{
		thinking:     controlRequest.Thinking,
		outputConfig: controlRequest.OutputConfig,
		temperature:  controlRequest.Temperature,
		explanation:  projectControlPlan(model, config, &controlRequest),
	}
}

func rejectedControlPlan(model string, err error) requestControlPlan {
	return requestControlPlan{
		explanation: modelcaps.Plan{
			Model:           model,
			Rejected:        true,
			RejectionReason: err.Error(),
		},
		err: err,
	}
}

func projectControlPlan(model string, config *llm.Config, request *Request) modelcaps.Plan {
	caps, known := lookupCapabilities(model)
	plan := modelcaps.Plan{
		Model:       model,
		Effort:      modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
		Budget:      modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
		Thinking:    modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
		Temperature: modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
	}

	if request.OutputConfig != nil {
		plan.Effective.ReasoningEffort = llm.ReasoningEffort(request.OutputConfig.Effort)
	}
	if request.Thinking != nil {
		plan.Effective.Thinking = llm.ThinkingType(request.Thinking.Type)
		if request.Thinking.Type == string(llm.ThinkingTypeEnabled) {
			budget := request.Thinking.BudgetTokens
			plan.Effective.ReasoningBudget = &budget
		}
	} else if modelRunsThinkingByDefault(model) {
		plan.Effective.Thinking = llm.ThinkingTypeAdaptive
	} else if config.Thinking == llm.ThinkingTypeDisabled {
		plan.Effective.Thinking = llm.ThinkingTypeDisabled
	}
	if request.Temperature != nil {
		temperature := *request.Temperature
		plan.Effective.Temperature = &temperature
	}

	effortOwnsBudget := false
	if config.ReasoningEffort != "" {
		switch {
		case request.OutputConfig != nil:
			plan.Effort = modelcaps.ControlDecision{
				Action:   modelcaps.ControlApplied,
				Adjusted: llm.ReasoningEffort(request.OutputConfig.Effort) != config.ReasoningEffort,
			}
		case known && caps.reasoningKind() == reasoningLegacyBudget &&
			config.Thinking != llm.ThinkingTypeDisabled && config.ReasoningBudget == nil &&
			request.Thinking != nil && request.Thinking.Type == string(llm.ThinkingTypeEnabled):
			mapped, recognized := legacyEffortBudget(config.ReasoningEffort)
			if recognized {
				effortOwnsBudget = true
				plan.Effort = modelcaps.ControlDecision{
					Action:   modelcaps.ControlEmulated,
					Adjusted: request.Thinking.BudgetTokens != mapped,
				}
				break
			}
			fallthrough
		default:
			plan.Effort = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "reasoning effort has no effect on the final request",
			}
		}
	}

	if config.ReasoningBudget != nil {
		switch {
		case request.Thinking != nil && request.Thinking.Type == string(llm.ThinkingTypeEnabled):
			plan.Budget = modelcaps.ControlDecision{
				Action:   modelcaps.ControlApplied,
				Adjusted: request.Thinking.BudgetTokens != *config.ReasoningBudget,
			}
		case request.Thinking != nil && request.Thinking.Type == string(llm.ThinkingTypeAdaptive):
			plan.Budget = modelcaps.ControlDecision{
				Action: modelcaps.ControlEmulated,
				Reason: "manual reasoning budget is represented by adaptive thinking",
			}
		default:
			plan.Budget = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "reasoning budget has no effect on the final request",
			}
		}
	} else if request.Thinking != nil && request.Thinking.Type == string(llm.ThinkingTypeEnabled) && !effortOwnsBudget {
		plan.Budget = modelcaps.ControlDecision{
			Action: modelcaps.ControlDefaulted,
			Reason: "a default supplies the effective reasoning budget",
		}
	}

	switch config.Thinking {
	case "":
		if modelRunsThinkingByDefault(model) && config.ReasoningBudget == nil && !effortOwnsBudget {
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlDefaulted}
		}
	case llm.ThinkingTypeDisabled:
		if requestHasThinkingEnabled(model, request.Thinking) {
			plan.Thinking = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "model cannot disable thinking",
			}
		} else {
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlApplied}
		}
	case llm.ThinkingTypeAdaptive:
		switch {
		case request.Thinking != nil && request.Thinking.Type == string(llm.ThinkingTypeAdaptive):
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlApplied}
		case request.Thinking != nil && request.Thinking.Type == string(llm.ThinkingTypeEnabled):
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlEmulated}
		default:
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlOmitted}
		}
	case llm.ThinkingTypeEnabled:
		switch {
		case request.Thinking != nil && request.Thinking.Type == string(llm.ThinkingTypeEnabled):
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlApplied}
		case request.Thinking != nil && request.Thinking.Type == string(llm.ThinkingTypeAdaptive):
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlEmulated}
		default:
			plan.Thinking = modelcaps.ControlDecision{Action: modelcaps.ControlOmitted}
		}
	default:
		plan.Thinking = modelcaps.ControlDecision{
			Action: modelcaps.ControlOmitted,
			Reason: "unrecognized thinking control",
		}
	}

	if config.Temperature != nil {
		if request.Temperature != nil {
			plan.Temperature = modelcaps.ControlDecision{Action: modelcaps.ControlApplied}
		} else {
			plan.Temperature = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "temperature is not meaningful for the final request",
			}
		}
	}
	return plan
}

func canDisableThinking(caps modelCapabilities) bool {
	hasReasoning := len(caps.efforts) > 0 || caps.manualBudget || caps.adaptive || caps.thinkingOnByDefault
	return hasReasoning && (!caps.thinkingOnByDefault || caps.explicitDisable)
}

func normalizeClassificationModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.TrimPrefix(model, "anthropic/")
}
