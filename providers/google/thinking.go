package google

import (
	"github.com/deepnoodle-ai/dive/llm"
	"google.golang.org/genai"
)

// thinkingLevelFor maps Dive's provider-neutral effort onto Gemini's ladder.
// Gemini stops at HIGH, so xhigh and max clamp there.
func thinkingLevelFor(effort llm.ReasoningEffort) (genai.ThinkingLevel, bool) {
	switch effort {
	case llm.ReasoningEffortMinimal:
		return genai.ThinkingLevelMinimal, true
	case llm.ReasoningEffortLow:
		return genai.ThinkingLevelLow, true
	case llm.ReasoningEffortMedium:
		return genai.ThinkingLevelMedium, true
	case llm.ReasoningEffortHigh, llm.ReasoningEffortXHigh, llm.ReasoningEffortMax:
		return genai.ThinkingLevelHigh, true
	default:
		return "", false
	}
}

// buildThinkingConfig maps Dive's reasoning and thinking options onto Gemini's
// thinkingConfig. It returns nil when the request should carry no thinking
// configuration at all.
//
// Gemini rejects a request that sets both a level and a budget ("You can only
// set only one of thinking budget and thinking level"), so an explicit budget
// wins and the effort is dropped with a warning — the budget is the more
// specific instruction.
//
// Settings the model cannot take are clamped or dropped rather than sent:
// effort clamps to the model's ladder, a budget clamps to [-1, 65535], and a
// request to disable thinking on a model that always thinks degrades to the
// least eager level it does accept.
func buildThinkingConfig(model string, config *llm.Config) *genai.ThinkingConfig {
	caps, known := lookupCapabilities(model)

	// Surfacing thought summaries is orthogonal to how much the model thinks.
	includeThoughts := config.ThinkingDisplay != "" ||
		config.Thinking == llm.ThinkingTypeAdaptive ||
		config.Thinking == llm.ThinkingTypeEnabled

	thinkingOff := config.Thinking == llm.ThinkingTypeDisabled ||
		config.ReasoningEffort == llm.ReasoningEffortNone

	switch {
	case thinkingOff:
		if !known || caps.canDisableThinking {
			budget := int32(0)
			return &genai.ThinkingConfig{ThinkingBudget: &budget}
		}
		// The model always thinks. Ask for as little as it allows rather than
		// sending a budget it rejects.
		if level, ok := leastEagerLevel(caps); ok {
			warnf(config, "model cannot disable thinking; using its least eager thinking level",
				"model", model, "level", string(level))
			return &genai.ThinkingConfig{ThinkingLevel: level}
		}
		budget := int32(caps.minBudget)
		warnf(config, "model cannot disable thinking; using its smallest thinking budget",
			"model", model, "budget", budget)
		return &genai.ThinkingConfig{ThinkingBudget: &budget}

	case config.ReasoningBudget != nil:
		if config.ReasoningEffort != "" {
			warnf(config, "Gemini accepts either a thinking budget or a thinking level, not both; keeping the budget",
				"model", model, "effort", config.ReasoningEffort)
		}
		budget := clampThinkingBudget(config, model, caps, known, *config.ReasoningBudget)
		return &genai.ThinkingConfig{
			ThinkingBudget:  &budget,
			IncludeThoughts: includeThoughts,
		}

	case config.ReasoningEffort != "":
		effort := config.ReasoningEffort
		// The 2.5 generation has no thinking level, so effort is emulated with
		// a budget the same way Anthropic's pre-4.5 models do it.
		if known && !caps.supportsThinkingLevel() {
			budget := clampThinkingBudget(config, model, caps, known, effortBudget(effort))
			return &genai.ThinkingConfig{
				ThinkingBudget:  &budget,
				IncludeThoughts: includeThoughts,
			}
		}
		if known {
			clamped, changed := llm.ClampReasoningEffort(effort, caps.efforts)
			if changed {
				warnf(config, "model does not support the requested thinking level; clamping",
					"model", model, "requested", effort, "using", clamped)
			}
			effort = clamped
		}
		level, ok := thinkingLevelFor(effort)
		if !ok {
			// A provider-specific or misspelled value: leave it to the API to
			// report rather than silently swallowing it.
			warnf(config, "unrecognized reasoning effort for Gemini; omitting the thinking level",
				"model", model, "effort", effort)
			if includeThoughts {
				return &genai.ThinkingConfig{IncludeThoughts: true}
			}
			return nil
		}
		return &genai.ThinkingConfig{
			ThinkingLevel:   level,
			IncludeThoughts: includeThoughts,
		}

	case config.Thinking == llm.ThinkingTypeAdaptive:
		// Gemini's equivalent of "decide for yourself" is a dynamic budget.
		budget := int32(dynamicThinkingBudget)
		return &genai.ThinkingConfig{ThinkingBudget: &budget, IncludeThoughts: true}

	case includeThoughts:
		// Thinking was not configured, but the caller wants to see it.
		return &genai.ThinkingConfig{IncludeThoughts: true}
	}

	return nil
}

// leastEagerLevel returns the lowest thinking level a model accepts.
func leastEagerLevel(caps modelCapabilities) (genai.ThinkingLevel, bool) {
	if len(caps.efforts) == 0 {
		return "", false
	}
	return thinkingLevelFor(caps.efforts[0])
}

// effortBudget maps an effort level to a thinking budget for models that have
// no thinking level. The values sit inside every budget-only model's range.
func effortBudget(effort llm.ReasoningEffort) int {
	switch effort {
	case llm.ReasoningEffortMinimal, llm.ReasoningEffortLow:
		return 1024
	case llm.ReasoningEffortMedium:
		return 4096
	case llm.ReasoningEffortHigh, llm.ReasoningEffortXHigh, llm.ReasoningEffortMax:
		return 16384
	default:
		return 4096
	}
}

// clampThinkingBudget keeps the budget inside the range the model validates
// against. The bounds differ per model, and the API reports them in its error
// text: 3.x takes [1, 65535], gemini-2.5-pro [128, 32768], and
// gemini-2.5-flash-lite [512, 24576].
//
// A dynamic budget (-1) is passed through: every model that takes budgets
// accepts it, and it means "choose for yourself" rather than a token count.
func clampThinkingBudget(config *llm.Config, model string, caps modelCapabilities, known bool, budget int) int32 {
	if budget == dynamicThinkingBudget {
		return dynamicThinkingBudget
	}
	if !known {
		return int32(budget)
	}
	if budget == 0 && caps.canDisableThinking {
		return 0
	}
	switch {
	case budget < caps.minBudget:
		warnf(config, "thinking budget is below this model's minimum; clamping",
			"model", model, "requested", budget, "using", caps.minBudget)
		return int32(caps.minBudget)
	case budget > caps.maxBudget:
		warnf(config, "thinking budget is above this model's maximum; clamping",
			"model", model, "requested", budget, "using", caps.maxBudget)
		return int32(caps.maxBudget)
	}
	return int32(budget)
}

func warnf(config *llm.Config, msg string, args ...any) {
	if config.Logger != nil {
		config.Logger.Warn(msg, args...)
	}
}
