package anthropic

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestEveryCatalogModelHasCapabilities is the drift guard for
// modelCapabilityTable: adding a model to catalog.json without classifying its
// reasoning, thinking, and sampling support fails here rather than at runtime
// against the API.
func TestEveryCatalogModelHasCapabilities(t *testing.T) {
	for _, model := range Catalog().Models {
		if model.Kind != "" && model.Kind != "text" {
			continue // only text models take reasoning parameters
		}
		id := model.ID
		if id == "" {
			continue // alias-only entries carry no id of their own
		}
		t.Run(id, func(t *testing.T) {
			_, known := lookupCapabilities(id)
			assert.True(t, known,
				"model %q (%s) has no entry in modelCapabilityTable; add one so its "+
					"reasoning and thinking parameters are gated", id, model.GoName)
		})
	}
}

func TestLookupCapabilitiesLongestPrefixWins(t *testing.T) {
	// "claude-opus-4" and "claude-opus-4-5" both prefix this id; the narrower
	// entry must win, or Opus 4.5 loses its native effort parameter.
	caps, known := lookupCapabilities("claude-opus-4-5-20251101")
	assert.True(t, known)
	assert.Equal(t, reasoningNative, caps.reasoningKind())

	caps, known = lookupCapabilities("claude-opus-4-20250514")
	assert.True(t, known)
	assert.Equal(t, reasoningLegacyBudget, caps.reasoningKind())
}

func TestLookupCapabilitiesUnknownModel(t *testing.T) {
	_, known := lookupCapabilities("some-gateway/custom-tuned-model")
	assert.False(t, known)
}

func TestLookupCapabilitiesAcceptsVendorPrefix(t *testing.T) {
	caps, known := lookupCapabilities("anthropic/claude-sonnet-5")
	assert.True(t, known)
	assert.True(t, caps.thinkingOnByDefault)
}

// TestAdaptiveThinkingUnsupportedFallsBackToBudget covers the 4.5 generation,
// which answers 400 with "adaptive thinking is not supported on this model".
// The CLI's --show-thinking asks for adaptive thinking on every model, so this
// path has to degrade rather than fail.
func TestAdaptiveThinkingUnsupportedFallsBackToBudget(t *testing.T) {
	for _, model := range []string{ModelClaudeHaiku45, ModelClaudeSonnet45, ModelClaudeOpus45} {
		req := buildReq(t, model, llm.WithAdaptiveThinking())
		assert.NotNil(t, req.Thinking, "model %s", model)
		assert.Equal(t, "enabled", req.Thinking.Type, "model %s", model)
		assert.Equal(t, defaultThinkingBudget, req.Thinking.BudgetTokens, "model %s", model)
	}
}

func TestAdaptiveThinkingUnsupportedHonorsExplicitBudget(t *testing.T) {
	req := buildReq(t, ModelClaudeSonnet45,
		llm.WithAdaptiveThinking(),
		llm.WithReasoningBudget(9000))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "enabled", req.Thinking.Type)
	assert.Equal(t, 9000, req.Thinking.BudgetTokens)
}

func TestAdaptiveThinkingSupportedStaysAdaptive(t *testing.T) {
	for _, model := range []string{ModelClaudeSonnet46, ModelClaudeOpus47, ModelClaudeSonnet5} {
		req := buildReq(t, model, llm.WithAdaptiveThinking())
		assert.NotNil(t, req.Thinking, "model %s", model)
		assert.Equal(t, "adaptive", req.Thinking.Type, "model %s", model)
	}
}

// TestBudgetAndEffortCombineOnNativeModel covers a case Dive used to reject
// outright: Opus 4.5 and Sonnet 4.6 accept a manual budget and an effort in the
// same request (verified against the live API).
func TestBudgetAndEffortCombineOnNativeModel(t *testing.T) {
	for _, model := range []string{ModelClaudeOpus45, ModelClaudeSonnet46} {
		req := buildReq(t, model,
			llm.WithReasoningBudget(8000),
			llm.WithReasoningEffort(llm.ReasoningEffortHigh))
		assert.NotNil(t, req.OutputConfig, "model %s", model)
		assert.Equal(t, "high", req.OutputConfig.Effort, "model %s", model)
		assert.NotNil(t, req.Thinking, "model %s", model)
		assert.Equal(t, 8000, req.Thinking.BudgetTokens, "model %s", model)
	}
}

// TestDisabledThinkingCapNeverRaisesEffort guards a regression: the Opus 5 cap
// was applied by passing the cap alone as the supported set, which clamped a
// below-cap request *up* to it — asking for low with thinking disabled sent
// high. The cap must only ever lower an effort.
func TestDisabledThinkingCapNeverRaisesEffort(t *testing.T) {
	for _, tt := range []struct {
		requested llm.ReasoningEffort
		want      string
	}{
		{llm.ReasoningEffortLow, "low"},
		{llm.ReasoningEffortMedium, "medium"},
		{llm.ReasoningEffortHigh, "high"},
		// Above the cap, it still clamps down.
		{llm.ReasoningEffortXHigh, "high"},
		{llm.ReasoningEffortMax, "high"},
	} {
		t.Run(string(tt.requested), func(t *testing.T) {
			req := buildReq(t, ModelClaudeOpus5,
				llm.WithReasoningEffort(tt.requested),
				llm.WithThinking(llm.ThinkingTypeDisabled))
			assert.NotNil(t, req.OutputConfig)
			assert.Equal(t, tt.want, req.OutputConfig.Effort)
		})
	}
}

func TestLookupCapabilitiesStripsAllVendorSegments(t *testing.T) {
	caps, known := lookupCapabilities("openrouter/anthropic/claude-sonnet-5")
	assert.True(t, known)
	assert.True(t, caps.thinkingOnByDefault)
}

func TestReasoningBudgetBelowMinimumClamps(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus46, llm.WithReasoningBudget(10))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, minThinkingBudget, req.Thinking.BudgetTokens)
}

func TestUnknownModelKeepsParametersUntouched(t *testing.T) {
	// Dive cannot know what a gateway model accepts, so it must not gate.
	req := buildReq(t, "my-proxy-claude", llm.WithTemperature(0.5))
	assert.NotNil(t, req.Temperature)
}
