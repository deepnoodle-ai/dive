// Package responsescontrol plans controls shared by OpenAI-compatible
// Responses API adapters without making request construction depend on the
// global model capability registry.
package responsescontrol

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
)

// Plan resolves the reasoning and temperature controls used by a Responses API
// request. Provider packages call it directly for both request construction and
// their registered network-free explanation.
func Plan(provider, model string, config *llm.Config) modelcaps.Plan {
	plan := modelcaps.Plan{
		Model:       model,
		Effort:      modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
		Budget:      modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
		Thinking:    modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
		Temperature: modelcaps.ControlDecision{Action: modelcaps.ControlNotRequested},
	}

	if config.ReasoningEffort != "" {
		effort, send := modelcaps.ResolveEffort(provider, model, config.ReasoningEffort, config.Logger)
		if send {
			plan.Effort = modelcaps.ControlDecision{
				Action:   modelcaps.ControlApplied,
				Adjusted: effort != config.ReasoningEffort,
			}
			plan.Effective.ReasoningEffort = effort
		} else {
			plan.Effort = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "model does not accept a reasoning effort",
			}
		}
	}
	if config.ReasoningBudget != nil {
		plan.Budget = modelcaps.ControlDecision{
			Action: modelcaps.ControlOmitted,
			Reason: "provider does not accept a reasoning budget",
		}
	}
	if config.Thinking != "" {
		plan.Thinking = modelcaps.ControlDecision{
			Action: modelcaps.ControlOmitted,
			Reason: "provider does not accept a thinking control",
		}
	}
	if config.Temperature != nil {
		if modelcaps.AcceptsTemperature(provider, model) {
			temperature := *config.Temperature
			plan.Temperature = modelcaps.ControlDecision{Action: modelcaps.ControlApplied}
			plan.Effective.Temperature = &temperature
		} else {
			if config.Logger != nil {
				config.Logger.Warn("model does not support temperature; omitting it", "model", model)
			}
			plan.Temperature = modelcaps.ControlDecision{
				Action: modelcaps.ControlOmitted,
				Reason: "model does not accept temperature",
			}
		}
	}
	return plan
}
