package openai

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/modelcaps"
)

// resolveResponsesReasoningEffort maps the requested effort onto what the model
// accepts. The bool reports whether to send a reasoning parameter at all:
// gpt-4o and gpt-4.1 have none, and sending one is a 400.
func resolveResponsesReasoningEffort(
	providerName, model string,
	config *llm.Config,
) (llm.ReasoningEffort, bool) {
	return modelcaps.ResolveEffort(providerName, model, config.ReasoningEffort, config.Logger)
}

// modelAcceptsTemperature reports whether the model takes a temperature.
func modelAcceptsTemperature(providerName, model string) bool {
	return modelcaps.AcceptsTemperature(providerName, model)
}

// modelReasons reports whether the model produces reasoning, regardless of
// whether this request named an effort level.
func modelReasons(providerName, model string) bool {
	return modelcaps.SupportsReasoning(providerName, model)
}
