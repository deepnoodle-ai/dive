package modelcaps

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestResolveEffortDropsForModelsWithoutReasoning(t *testing.T) {
	// gpt-4o and gpt-4.1 answer 400 "Unsupported parameter: 'reasoning.effort'".
	// The CLI defaults effort to medium, so this fires on every request unless
	// the parameter is dropped.
	for _, model := range []string{"gpt-4o", "gpt-4.1"} {
		effort, send := ResolveEffort("openai", model, llm.ReasoningEffortMedium, nil)
		assert.False(t, send, "model %s", model)
		assert.Equal(t, llm.ReasoningEffort(""), effort, "model %s", model)
	}
}

func TestResolveEffortDropsForGrokModelsWithoutReasoning(t *testing.T) {
	// "does not support parameter reasoningEffort" — including a model whose
	// own name says reasoning.
	for _, model := range []string{
		"grok-build-0.1",
		"grok-code-fast-1",
		"grok-4.20-0309-reasoning",
		"grok-4.20-0309-non-reasoning",
	} {
		_, send := ResolveEffort("grok", model, llm.ReasoningEffortMedium, nil)
		assert.False(t, send, "model %s", model)
	}
}

func TestResolveEffortClampsDown(t *testing.T) {
	tests := []struct {
		provider  string
		model     string
		requested llm.ReasoningEffort
		want      llm.ReasoningEffort
	}{
		// gpt-5.5 stops at xhigh.
		{"openai", "gpt-5.5", llm.ReasoningEffortMax, llm.ReasoningEffortXHigh},
		// gpt-5.1 stops at high.
		{"openai", "gpt-5.1", llm.ReasoningEffortXHigh, llm.ReasoningEffortHigh},
		// gpt-5-pro accepts high and nothing else, in either direction.
		{"openai", "gpt-5-pro", llm.ReasoningEffortMax, llm.ReasoningEffortHigh},
		{"openai", "gpt-5-pro", llm.ReasoningEffortLow, llm.ReasoningEffortHigh},
		// The pro variant is narrower than its family, not wider.
		{"openai", "gpt-5.2-pro", llm.ReasoningEffortLow, llm.ReasoningEffortMedium},
		// The chat-tuned model takes medium only.
		{"openai", "gpt-5.3-chat-latest", llm.ReasoningEffortHigh, llm.ReasoningEffortMedium},
		// grok-4.5 accepts xhigh, so max clamps one level, not two.
		{"grok", "grok-4.5", llm.ReasoningEffortMax, llm.ReasoningEffortXHigh},
		// Only the multi-agent model accepts max.
		{"grok", "grok-4.20-multi-agent-0309", llm.ReasoningEffortMax, llm.ReasoningEffortMax},
	}
	for _, tt := range tests {
		t.Run(tt.model+"/"+string(tt.requested), func(t *testing.T) {
			effort, send := ResolveEffort(tt.provider, tt.model, tt.requested, nil)
			assert.True(t, send)
			assert.Equal(t, tt.want, effort)
		})
	}
}

func TestResolveEffortClampsUpToLeastEager(t *testing.T) {
	// none sits below every graduated level, so it clamps up to the least eager
	// level the model accepts rather than being sent and rejected.
	effort, send := ResolveEffort("openai", "gpt-5", llm.ReasoningEffortNone, nil)
	assert.True(t, send)
	assert.Equal(t, llm.ReasoningEffortMinimal, effort)

	// o-series has no minimal either.
	effort, send = ResolveEffort("openai", "o3", llm.ReasoningEffortMinimal, nil)
	assert.True(t, send)
	assert.Equal(t, llm.ReasoningEffortLow, effort)

	// grok-4.5 is the only Grok model that rejects none.
	effort, send = ResolveEffort("grok", "grok-4.5", llm.ReasoningEffortNone, nil)
	assert.True(t, send)
	assert.Equal(t, llm.ReasoningEffortMinimal, effort)
}

func TestResolveEffortKeepsSupportedLevels(t *testing.T) {
	// none is a real level on gpt-5.1+ and must not be clamped away.
	effort, send := ResolveEffort("openai", "gpt-5.4", llm.ReasoningEffortNone, nil)
	assert.True(t, send)
	assert.Equal(t, llm.ReasoningEffortNone, effort)
}

func TestResolveEffortPassesThroughUnknownModels(t *testing.T) {
	// A gateway or fine-tune: Dive cannot know what it accepts.
	effort, send := ResolveEffort("openrouter", "deepseek/deepseek-r1", llm.ReasoningEffortMax, nil)
	assert.True(t, send)
	assert.Equal(t, llm.ReasoningEffortMax, effort)

	// An unverified entry behaves the same way.
	effort, send = ResolveEffort("openai", "gpt-5.1-codex", llm.ReasoningEffortMax, nil)
	assert.True(t, send)
	assert.Equal(t, llm.ReasoningEffortMax, effort)
}

func TestResolveEffortPassesThroughUnrecognizedValues(t *testing.T) {
	// A provider-specific or misspelled value is forwarded so the API can
	// report it, rather than being silently swallowed.
	effort, send := ResolveEffort("openai", "gpt-5.5", llm.ReasoningEffort("superdeep"), nil)
	assert.True(t, send)
	assert.Equal(t, llm.ReasoningEffort("superdeep"), effort)
}

func TestAcceptsTemperature(t *testing.T) {
	for _, model := range []string{"gpt-5", "gpt-5.5", "gpt-5.6", "o3", "o4-mini"} {
		assert.False(t, AcceptsTemperature("openai", model), "model %s", model)
	}
	for _, model := range []string{"gpt-4o", "gpt-4.1", "gpt-5.1", "gpt-5.4"} {
		assert.True(t, AcceptsTemperature("openai", model), "model %s", model)
	}
	// Every Grok model accepts temperature.
	for _, model := range []string{"grok-4.5", "grok-build-0.1", "grok-3"} {
		assert.True(t, AcceptsTemperature("grok", model), "model %s", model)
	}
	// Unknown models are assumed to accept it.
	assert.True(t, AcceptsTemperature("openrouter", "deepseek/deepseek-r1"))
}

func TestLookupLongestPrefixWins(t *testing.T) {
	// "gpt-5" and "gpt-5-pro" both match; the narrower entry must win.
	caps, known := Lookup("openai", "gpt-5-pro")
	assert.True(t, known)
	assert.Equal(t, 1, len(caps.Efforts))

	caps, known = Lookup("openai", "gpt-5")
	assert.True(t, known)
	assert.Equal(t, 4, len(caps.Efforts))
}

func TestLookupHandlesVendorPrefixes(t *testing.T) {
	caps, known := Lookup("openrouter", "openai/gpt-5.6")
	assert.True(t, known)
	assert.Equal(t, llm.ReasoningEffortMax, caps.Efforts[len(caps.Efforts)-1])

	_, known = Lookup("openrouter", "x-ai/grok-build-0.1")
	assert.True(t, known)
}

func TestTableForUnknownVendorIsNil(t *testing.T) {
	assert.Nil(t, TableFor("openrouter", "mistralai/mistral-large"))
}
