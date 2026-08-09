package openaicompletions

import (
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
)

// resolveReasoningEffort maps the requested effort onto what the model accepts.
// The bool reports whether to send a reasoning_effort field at all: several
// models reject the parameter outright rather than ignoring it.
//
// Model capabilities come from the shared modelcaps tables, so the Chat
// Completions path and the Responses path agree about the same model. Anything
// modelcaps does not recognize — a Mistral, DeepSeek, or other OpenRouter model
// — is forwarded untouched, except Mistral's own endpoint, which has no
// reasoning parameter at all.
func (p *Provider) resolveReasoningEffort(model string, config *llm.Config) (ReasoningEffort, bool) {
	effort := config.ReasoningEffort
	if effort == "" {
		return "", false
	}
	if _, known := modelcaps.Lookup(p.Name(), model); known {
		resolved, send := modelcaps.ResolveEffort(p.Name(), model, effort, config.Logger)
		return ReasoningEffort(resolved), send
	}
	if strings.Contains(p.endpoint, "api.mistral.ai") {
		if config.Logger != nil {
			config.Logger.Warn("provider does not support reasoning effort; omitting option",
				"provider", "mistral", "model", model, "reasoning_effort", effort)
		}
		return "", false
	}
	return ReasoningEffort(effort), true
}

// normalizeToolReasoningEffort handles Chat Completions constraints that only
// apply when function tools and reasoning are requested together. GPT-5.4 mini
// rejects that combination unless reasoning_effort is "none".
func normalizeToolReasoningEffort(model string, effort ReasoningEffort, hasFunctionTools bool) (ReasoningEffort, bool) {
	if !hasFunctionTools || effort == ReasoningEffortNone {
		return effort, false
	}

	model = strings.TrimPrefix(strings.ToLower(model), "openai/")
	if model == ModelGPT54Mini || strings.HasPrefix(model, ModelGPT54Mini+"-") {
		return ReasoningEffortNone, true
	}
	return effort, false
}
