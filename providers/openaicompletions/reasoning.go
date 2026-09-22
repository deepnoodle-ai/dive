package openaicompletions

import (
	"slices"
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

// chatToolsRequireNoReasoning reports whether OpenAI's Chat Completions
// endpoint rejects function tools for a model unless reasoning_effort is
// "none". That holds for gpt-5.4 through gpt-6: any other effort is refused,
// and gpt-5.6 and gpt-6 default to a reasoning effort, so leaving the field
// off fails too.
//
// Only bare OpenAI ids qualify. OpenRouter's "openai/..." ids route to the
// Responses API, which accepts reasoning with tools. A model that rejects
// "none" itself (gpt-6-astra) is left alone, since forcing it would only trade
// one 400 for another.
func chatToolsRequireNoReasoning(model string) bool {
	id := strings.ToLower(strings.TrimSpace(model))
	restricted := false
	for _, family := range []string{"gpt-5.4", "gpt-5.5", "gpt-5.6", "gpt-6"} {
		if id == family || strings.HasPrefix(id, family+"-") {
			restricted = true
			break
		}
	}
	if !restricted {
		return false
	}
	caps, known := modelcaps.Lookup("openai", id)
	return known && slices.Contains(caps.Efforts, llm.ReasoningEffortNone)
}
