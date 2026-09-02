package google

import (
	"math"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

// TestEveryCatalogModelHasCapabilities is the drift guard for
// modelCapabilityTable. Thinking support varies within a family rather than by
// generation — gemini-3.5-flash can disable thinking and gemini-3.5-flash-lite
// cannot — so a new model has to be classified deliberately.
func TestEveryCatalogModelHasCapabilities(t *testing.T) {
	for _, model := range Catalog().Models {
		if model.Kind != "" && model.Kind != "text" {
			continue // only text models take thinking parameters
		}
		id := model.ID
		if id == "" {
			continue // alias-only entries carry no id of their own
		}
		t.Run(id, func(t *testing.T) {
			_, found := lookupEntry(id)
			assert.True(t, found,
				"model %q (%s) has no entry in modelCapabilityTable; add one so "+
					"its thinking parameters are gated", id, model.GoName)
		})
	}
}

func buildThinking(t *testing.T, model string, opts ...llm.Option) *genai.ThinkingConfig {
	t.Helper()
	cfg := &llm.Config{}
	cfg.Apply(append([]llm.Option{llm.WithModel(model)}, opts...)...)
	var req Request
	assert.NoError(t, New().applyRequestConfig(&req, cfg))
	return req.Thinking
}

func TestEffortMapsToThinkingLevel(t *testing.T) {
	tests := []struct {
		effort llm.ReasoningEffort
		want   genai.ThinkingLevel
	}{
		{llm.ReasoningEffortMinimal, genai.ThinkingLevelMinimal},
		{llm.ReasoningEffortLow, genai.ThinkingLevelLow},
		{llm.ReasoningEffortMedium, genai.ThinkingLevelMedium},
		{llm.ReasoningEffortHigh, genai.ThinkingLevelHigh},
		// Gemini's ladder stops at HIGH.
		{llm.ReasoningEffortXHigh, genai.ThinkingLevelHigh},
		{llm.ReasoningEffortMax, genai.ThinkingLevelHigh},
	}
	for _, tt := range tests {
		t.Run(string(tt.effort), func(t *testing.T) {
			thinking := buildThinking(t, ModelGemini36Flash, llm.WithReasoningEffort(tt.effort))
			assert.NotNil(t, thinking)
			assert.Equal(t, tt.want, thinking.ThinkingLevel)
			assert.Nil(t, thinking.ThinkingBudget)
		})
	}
}

func TestMinimalClampsOnProModels(t *testing.T) {
	// "Thinking level MINIMAL is not supported for this model" — clamp up to
	// the least eager level Pro accepts.
	thinking := buildThinking(t, ModelGemini31ProPreview,
		llm.WithReasoningEffort(llm.ReasoningEffortMinimal))
	assert.NotNil(t, thinking)
	assert.Equal(t, genai.ThinkingLevelLow, thinking.ThinkingLevel)
}

func TestMinimalClampsOnGemini37Flash(t *testing.T) {
	// Verified live: 3.7 Flash answers "Thinking level is unsupported:
	// THINKING_LEVEL_MINIMAL" even though the rest of the 3.x flash family
	// accepts it — clamp up to LOW like the Pro models do.
	thinking := buildThinking(t, ModelGemini37Flash,
		llm.WithReasoningEffort(llm.ReasoningEffortMinimal))
	assert.NotNil(t, thinking)
	assert.Equal(t, genai.ThinkingLevelLow, thinking.ThinkingLevel)
}

func TestGemini37FlashBudgetCeilingIsLowerThanTheFamily(t *testing.T) {
	// Verified live: 3.7 Flash reports "supported values are integers from 1
	// to 32768", not the 65535 ceiling the rest of the 3.x flash family uses.
	thinking := buildThinking(t, ModelGemini37Flash, llm.WithReasoningBudget(1_000_000))
	assert.NotNil(t, thinking)
	assert.Equal(t, int32(32768), *thinking.ThinkingBudget)
}

func TestGemini38FlashResolvesToItsOwnCapabilities(t *testing.T) {
	// 3.8 Flash sits between its neighbours: it rejects MINIMAL like 3.7 Flash
	// but keeps the family's 65535 budget ceiling rather than 3.7's 32768, so
	// landing on the 3.7 entry by prefix would be wrong in one direction and
	// landing on a generic 3.x entry wrong in the other. Both probed live.
	entry, found := lookupEntry(ModelGemini38Flash)
	assert.True(t, found)
	assert.Equal(t, "gemini-3.8-flash", entry.prefix)

	clamped := buildThinking(t, ModelGemini38Flash,
		llm.WithReasoningEffort(llm.ReasoningEffortMinimal))
	assert.NotNil(t, clamped)
	assert.Equal(t, genai.ThinkingLevelLow, clamped.ThinkingLevel)

	budget := buildThinking(t, ModelGemini38Flash, llm.WithReasoningBudget(1_000_000))
	assert.NotNil(t, budget)
	assert.Equal(t, int32(65535), *budget.ThinkingBudget)
}

func TestReasoningBudgetMapsToThinkingBudget(t *testing.T) {
	thinking := buildThinking(t, ModelGemini36Flash, llm.WithReasoningBudget(4096))
	assert.NotNil(t, thinking)
	assert.NotNil(t, thinking.ThinkingBudget)
	assert.Equal(t, int32(4096), *thinking.ThinkingBudget)
	assert.Equal(t, genai.ThinkingLevel(""), thinking.ThinkingLevel)
}

func TestBudgetAndEffortAreMutuallyExclusive(t *testing.T) {
	// "You can only set only one of thinking budget and thinking level."
	// The explicit budget is the more specific instruction, so it wins.
	thinking := buildThinking(t, ModelGemini36Flash,
		llm.WithReasoningBudget(4096),
		llm.WithReasoningEffort(llm.ReasoningEffortHigh))
	assert.NotNil(t, thinking)
	assert.NotNil(t, thinking.ThinkingBudget)
	assert.Equal(t, genai.ThinkingLevel(""), thinking.ThinkingLevel)
}

func TestThinkingBudgetClamps(t *testing.T) {
	thinking := buildThinking(t, ModelGemini36Flash, llm.WithReasoningBudget(1_000_000))
	assert.NotNil(t, thinking)
	assert.Equal(t, int32(65535), *thinking.ThinkingBudget)
}

func TestAdaptiveThinkingUsesDynamicBudget(t *testing.T) {
	thinking := buildThinking(t, ModelGemini36Flash, llm.WithAdaptiveThinking())
	assert.NotNil(t, thinking)
	assert.NotNil(t, thinking.ThinkingBudget)
	assert.Equal(t, int32(dynamicThinkingBudget), *thinking.ThinkingBudget)
	assert.True(t, thinking.IncludeThoughts)
}

func TestDisabledThinkingUsesZeroBudgetWhereSupported(t *testing.T) {
	thinking := buildThinking(t, ModelGemini35Flash,
		llm.WithThinking(llm.ThinkingTypeDisabled))
	assert.NotNil(t, thinking)
	assert.NotNil(t, thinking.ThinkingBudget)
	assert.Equal(t, int32(0), *thinking.ThinkingBudget)
}

func TestDisabledThinkingDegradesWhereUnsupported(t *testing.T) {
	// gemini-3.6-flash rejects thinkingBudget: 0, so the request asks for the
	// least eager level instead of sending a budget that 400s.
	thinking := buildThinking(t, ModelGemini36Flash,
		llm.WithThinking(llm.ThinkingTypeDisabled))
	assert.NotNil(t, thinking)
	assert.Nil(t, thinking.ThinkingBudget)
	assert.Equal(t, genai.ThinkingLevelMinimal, thinking.ThinkingLevel)
}

func TestEffortNoneDisablesThinking(t *testing.T) {
	thinking := buildThinking(t, ModelGemini35Flash,
		llm.WithReasoningEffort(llm.ReasoningEffortNone))
	assert.NotNil(t, thinking)
	assert.NotNil(t, thinking.ThinkingBudget)
	assert.Equal(t, int32(0), *thinking.ThinkingBudget)
}

func TestThinkingDisplayRequestsThoughts(t *testing.T) {
	thinking := buildThinking(t, ModelGemini36Flash,
		llm.WithReasoningEffort(llm.ReasoningEffortLow),
		llm.WithThinkingDisplay(llm.ThinkingDisplaySummarized))
	assert.NotNil(t, thinking)
	assert.True(t, thinking.IncludeThoughts)
}

func TestBudgetOnlyModelEmulatesEffortWithBudget(t *testing.T) {
	// The 2.5 generation answers "Thinking level is not supported for this
	// model", so effort becomes a budget instead.
	thinking := buildThinking(t, "gemini-2.5-flash", llm.WithReasoningEffort(llm.ReasoningEffortHigh))
	assert.NotNil(t, thinking)
	assert.Equal(t, genai.ThinkingLevel(""), thinking.ThinkingLevel)
	assert.NotNil(t, thinking.ThinkingBudget)
	assert.Equal(t, int32(16384), *thinking.ThinkingBudget)
}

func TestBudgetClampsToPerModelRange(t *testing.T) {
	// gemini-2.5-pro reports "choose a value between 128 and 32768".
	thinking := buildThinking(t, "gemini-2.5-pro", llm.WithReasoningBudget(60000))
	assert.NotNil(t, thinking)
	assert.Equal(t, int32(32768), *thinking.ThinkingBudget)

	thinking = buildThinking(t, "gemini-2.5-pro", llm.WithReasoningBudget(10))
	assert.NotNil(t, thinking)
	assert.Equal(t, int32(128), *thinking.ThinkingBudget)

	// gemini-2.5-flash-lite starts at 512.
	thinking = buildThinking(t, "gemini-2.5-flash-lite", llm.WithReasoningBudget(128))
	assert.NotNil(t, thinking)
	assert.Equal(t, int32(512), *thinking.ThinkingBudget)
}

func TestBudgetOnlyModelCannotDisableDegradesToMinimum(t *testing.T) {
	// gemini-2.5-pro rejects thinkingBudget: 0 and has no thinking level, so it
	// falls back to its smallest legal budget.
	thinking := buildThinking(t, "gemini-2.5-pro", llm.WithThinking(llm.ThinkingTypeDisabled))
	assert.NotNil(t, thinking)
	assert.NotNil(t, thinking.ThinkingBudget)
	assert.Equal(t, int32(128), *thinking.ThinkingBudget)
}

// TestUnknownModelBudgetDoesNotWrap guards the int32 narrowing on the
// unknown-model path: without a bound check a wild value wraps into a
// plausible-looking budget rather than being obviously clamped.
func TestUnknownModelBudgetDoesNotWrap(t *testing.T) {
	thinking := buildThinking(t, "my-tuned-gemini", llm.WithReasoningBudget(math.MaxInt32+1))
	assert.NotNil(t, thinking)
	assert.Equal(t, int32(math.MaxInt32), *thinking.ThinkingBudget)

	thinking = buildThinking(t, "my-tuned-gemini", llm.WithReasoningBudget(math.MinInt32-1))
	assert.NotNil(t, thinking)
	assert.Equal(t, int32(math.MinInt32), *thinking.ThinkingBudget)

	// In-range values still pass through untouched.
	thinking = buildThinking(t, "my-tuned-gemini", llm.WithReasoningBudget(4096))
	assert.NotNil(t, thinking)
	assert.Equal(t, int32(4096), *thinking.ThinkingBudget)
}

func TestRetiredModelPassesThrough(t *testing.T) {
	// Retired models 404; their parameters are forwarded rather than guessed.
	_, known := lookupCapabilities("gemini-1.5-pro")
	assert.False(t, known)
	_, found := lookupEntry("gemini-1.5-pro")
	assert.True(t, found)
}

func TestNoThinkingOptionsLeavesConfigNil(t *testing.T) {
	thinking := buildThinking(t, ModelGemini36Flash)
	assert.Nil(t, thinking)
}

func TestUnknownModelForwardsEffort(t *testing.T) {
	// A Vertex deployment or tuned model: Dive cannot know its ladder, so the
	// level is sent as asked rather than clamped against a guess.
	thinking := buildThinking(t, "my-tuned-gemini",
		llm.WithReasoningEffort(llm.ReasoningEffortMinimal))
	assert.NotNil(t, thinking)
	assert.Equal(t, genai.ThinkingLevelMinimal, thinking.ThinkingLevel)
}

func TestThinkingConfigReachesGenAIConfig(t *testing.T) {
	cfg := &llm.Config{}
	cfg.Apply(
		llm.WithModel(ModelGemini36Flash),
		llm.WithReasoningEffort(llm.ReasoningEffortHigh),
	)
	var req Request
	assert.NoError(t, New().applyRequestConfig(&req, cfg))

	genConfig, err := buildGenAIGenerateConfig(&req)
	assert.NoError(t, err)
	assert.NotNil(t, genConfig.ThinkingConfig)
	assert.Equal(t, genai.ThinkingLevelHigh, genConfig.ThinkingConfig.ThinkingLevel)
}
