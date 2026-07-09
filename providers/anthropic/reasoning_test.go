package anthropic

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

// buildReq applies the given options to a config for the given model and
// returns the resulting Anthropic request (without messages).
func buildReq(t *testing.T, model string, opts ...llm.Option) *Request {
	t.Helper()
	cfg := &llm.Config{}
	cfg.Apply(append([]llm.Option{llm.WithModel(model)}, opts...)...)
	p := New()
	var req Request
	assert.NoError(t, p.applyRequestConfig(&req, cfg))
	return &req
}

func reasoningTestTool() llm.Tool {
	return llm.NewToolDefinition().
		WithName("lookup").
		WithDescription("Look up a value").
		WithSchema(&schema.Schema{Type: "object"})
}

func TestReasoningEffortOpus48UsesOutputConfig(t *testing.T) {
	// On Opus 4.8 effort maps to output_config.effort (no thinking implied),
	// and the new xhigh level is passed through.
	req := buildReq(t, ModelClaudeOpus48, llm.WithReasoningEffort(llm.ReasoningEffortXHigh))
	assert.NotNil(t, req.OutputConfig)
	assert.Equal(t, "xhigh", req.OutputConfig.Effort)
	assert.Nil(t, req.Thinking)
}

func TestReasoningEffortMinimalMapsToLow(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus48, llm.WithReasoningEffort(llm.ReasoningEffortMinimal))
	assert.NotNil(t, req.OutputConfig)
	assert.Equal(t, "low", req.OutputConfig.Effort)
}

func TestReasoningEffortNoneErrors(t *testing.T) {
	cfg := &llm.Config{}
	cfg.Apply(
		llm.WithModel(ModelClaudeOpus48),
		llm.WithReasoningEffort(llm.ReasoningEffortNone),
	)
	var req Request
	err := New().applyRequestConfig(&req, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestReasoningEffortXHighUnsupportedNativeModelMapsToHigh(t *testing.T) {
	req := buildReq(t, ModelClaudeSonnet46, llm.WithReasoningEffort(llm.ReasoningEffortXHigh))
	assert.NotNil(t, req.OutputConfig)
	assert.Equal(t, "high", req.OutputConfig.Effort)
}

func TestReasoningBudgetOpus48FallsBackToAdaptive(t *testing.T) {
	// Opus 4.7/4.8 reject manual budgets; a budget transparently becomes
	// adaptive thinking so existing callers keep working.
	req := buildReq(t, ModelClaudeOpus48, llm.WithReasoningBudget(8000))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "adaptive", req.Thinking.Type)
	assert.Equal(t, 0, req.Thinking.BudgetTokens)
}

func TestAdaptiveThinkingOpus48(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus48, llm.WithAdaptiveThinking())
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "adaptive", req.Thinking.Type)
}

func TestThinkingDisplay(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus48,
		llm.WithAdaptiveThinking(),
		llm.WithThinkingDisplay(llm.ThinkingDisplaySummarized))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "summarized", req.Thinking.Display)
}

func TestSummarizedThinkingDisplayOnDefaultOmittedModel(t *testing.T) {
	req := buildReq(t, ModelClaudeSonnet5,
		llm.WithAdaptiveThinking(),
		llm.WithThinkingDisplay(llm.ThinkingDisplaySummarized))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "adaptive", req.Thinking.Type)
	assert.Equal(t, "summarized", req.Thinking.Display)
}

func TestReasoningEffortAndAdaptiveCombine(t *testing.T) {
	// Effort and adaptive thinking are orthogonal on Opus 4.8.
	req := buildReq(t, ModelClaudeOpus48,
		llm.WithReasoningEffort(llm.ReasoningEffortMax),
		llm.WithAdaptiveThinking())
	assert.NotNil(t, req.OutputConfig)
	assert.Equal(t, "max", req.OutputConfig.Effort)
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "adaptive", req.Thinking.Type)
}

func TestReasoningBudgetOpus46KeepsManual(t *testing.T) {
	// Opus 4.6 still accepts manual budgets (deprecated but functional).
	req := buildReq(t, ModelClaudeOpus46, llm.WithReasoningBudget(8000))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "enabled", req.Thinking.Type)
	assert.Equal(t, 8000, req.Thinking.BudgetTokens)
	assert.Nil(t, req.OutputConfig)
}

func TestReasoningEffortLegacyModelMapsToBudget(t *testing.T) {
	// Models without the native effort parameter emulate it with a budget.
	req := buildReq(t, ModelClaude37Sonnet20250219, llm.WithReasoningEffort(llm.ReasoningEffortMedium))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "enabled", req.Thinking.Type)
	assert.Equal(t, 4096, req.Thinking.BudgetTokens)
	assert.Nil(t, req.OutputConfig)
}

func TestReasoningEffortMinimalLegacyModelMapsToLowBudget(t *testing.T) {
	req := buildReq(t, ModelClaude37Sonnet20250219, llm.WithReasoningEffort(llm.ReasoningEffortMinimal))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "enabled", req.Thinking.Type)
	assert.Equal(t, 1024, req.Thinking.BudgetTokens)
	assert.Nil(t, req.OutputConfig)
}

func TestThinkingDisabled(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus48,
		llm.WithReasoningBudget(8000),
		llm.WithThinking(llm.ThinkingTypeDisabled))
	assert.Nil(t, req.Thinking)
}

func TestThinkingDropsTemperature(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus46,
		llm.WithReasoningBudget(8000),
		llm.WithTemperature(0.7))
	assert.NotNil(t, req.Thinking)
	assert.Nil(t, req.Temperature)
}

func TestThinkingWithPrefillErrors(t *testing.T) {
	cfg := &llm.Config{}
	cfg.Apply(
		llm.WithModel(ModelClaudeOpus46),
		llm.WithReasoningBudget(8000),
		llm.WithPrefill("prefilled answer", ""),
	)
	var req Request
	err := New().applyRequestConfig(&req, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prefilled assistant responses")
}

func TestThinkingWithForcedToolChoiceErrors(t *testing.T) {
	cfg := &llm.Config{}
	cfg.Apply(
		llm.WithModel(ModelClaudeOpus46),
		llm.WithReasoningBudget(8000),
		llm.WithTools(reasoningTestTool()),
		llm.WithToolChoice(&llm.ToolChoice{
			Type: llm.ToolChoiceTypeTool,
			Name: "lookup",
		}),
	)
	var req Request
	err := New().applyRequestConfig(&req, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool_choice auto or none")
}

func TestDefaultThinkingWithForcedToolChoiceErrors(t *testing.T) {
	cfg := &llm.Config{}
	cfg.Apply(
		llm.WithModel(ModelClaudeFable5),
		llm.WithTools(reasoningTestTool()),
		llm.WithToolChoice(llm.ToolChoiceAny),
	)
	var req Request
	err := New().applyRequestConfig(&req, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tool_choice auto or none")
}

func TestThinkingAllowsToolChoiceAutoAndNone(t *testing.T) {
	for _, choice := range []*llm.ToolChoice{llm.ToolChoiceAuto, llm.ToolChoiceNone} {
		req := buildReq(t, ModelClaudeOpus46,
			llm.WithReasoningBudget(8000),
			llm.WithTools(reasoningTestTool()),
			llm.WithToolChoice(choice))
		assert.NotNil(t, req.ToolChoice)
		assert.Equal(t, ToolChoiceType(choice.Type), req.ToolChoice.Type)
	}
}

func TestManualThinkingBudgetMustBeLessThanMaxTokens(t *testing.T) {
	cfg := &llm.Config{}
	cfg.Apply(
		llm.WithModel(ModelClaudeOpus46),
		llm.WithMaxTokens(4096),
		llm.WithReasoningBudget(4096),
	)
	var req Request
	err := New().applyRequestConfig(&req, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be less than max_tokens")
}

func TestInterleavedThinkingAllowsBudgetAtOrAboveMaxTokens(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus45,
		llm.WithMaxTokens(4096),
		llm.WithReasoningBudget(8192),
		llm.WithFeatures(FeatureInterleavedThinking))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, 8192, req.Thinking.BudgetTokens)
}

func TestEffortWithThinkingDisabledLegacyModelErrors(t *testing.T) {
	// On a model without the native effort parameter, effort is emulated with a
	// thinking budget — which would override an explicit thinking disable. That
	// conflict must error rather than silently re-enable thinking.
	cfg := &llm.Config{}
	cfg.Apply(
		llm.WithModel(ModelClaude37Sonnet20250219),
		llm.WithReasoningEffort(llm.ReasoningEffortHigh),
		llm.WithThinking(llm.ThinkingTypeDisabled),
	)
	var req Request
	err := New().applyRequestConfig(&req, cfg)
	assert.Error(t, err)
}

func TestEffortWithThinkingDisabledNativeModelOK(t *testing.T) {
	// On a native-effort model, effort and a thinking disable are orthogonal:
	// effort goes to output_config and thinking stays off, with no error.
	req := buildReq(t, ModelClaudeOpus48,
		llm.WithReasoningEffort(llm.ReasoningEffortHigh),
		llm.WithThinking(llm.ThinkingTypeDisabled))
	assert.Nil(t, req.Thinking)
	assert.NotNil(t, req.OutputConfig)
	assert.Equal(t, "high", req.OutputConfig.Effort)
}

func TestSpeedFastSetsRequestField(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus48, llm.WithSpeed(llm.SpeedFast))
	assert.Equal(t, "fast", req.Speed)
}

func TestSpeedFastAddsBetaHeader(t *testing.T) {
	p := New()
	cfg := &llm.Config{Speed: llm.SpeedFast}
	httpReq, err := p.createRequest(context.Background(), []byte("{}"), cfg, false)
	assert.NoError(t, err)
	assert.Contains(t, httpReq.Header.Get("anthropic-beta"), FeatureFastMode)
}

func TestModelCapabilityHelpers(t *testing.T) {
	assert.True(t, modelSupportsEffortParam(ModelClaudeOpus48))
	assert.True(t, modelSupportsEffortParam(ModelClaudeSonnet46))
	assert.True(t, modelSupportsEffortParam(ModelClaudeOpus45))
	assert.True(t, modelSupportsEffortParam(ModelClaudeFable5))
	assert.True(t, modelSupportsEffortParam(ModelClaudeMythos5))
	assert.True(t, modelSupportsEffortParam(ModelClaudeSonnet5))
	assert.False(t, modelSupportsEffortParam(ModelClaude37Sonnet20250219))

	assert.True(t, modelRejectsManualThinking(ModelClaudeOpus47))
	assert.True(t, modelRejectsManualThinking(ModelClaudeOpus48))
	assert.True(t, modelRejectsManualThinking(ModelClaudeFable5))
	assert.True(t, modelRejectsManualThinking(ModelClaudeMythos5))
	assert.True(t, modelRejectsManualThinking(ModelClaudeSonnet5))
	assert.False(t, modelRejectsManualThinking(ModelClaudeOpus46))
	assert.False(t, modelRejectsManualThinking(ModelClaudeSonnet46))

	assert.True(t, modelSupportsXHighEffort(ModelClaudeFable5))
	assert.True(t, modelSupportsMaxEffort(ModelClaudeFable5))
	assert.True(t, modelSupportsXHighEffort(ModelClaudeMythos5))
	assert.True(t, modelSupportsMaxEffort(ModelClaudeMythos5))

	// Sonnet 5 supports max effort (like Sonnet 4.6) but not xhigh.
	assert.False(t, modelSupportsXHighEffort(ModelClaudeSonnet5))
	assert.True(t, modelSupportsMaxEffort(ModelClaudeSonnet5))

	// Sonnet 5 rejects sampling params and defaults thinking on.
	assert.True(t, modelRejectsTemperature(ModelClaudeSonnet5))
	assert.True(t, modelDefaultsThinkingOn(ModelClaudeSonnet5))
	assert.True(t, modelRunsThinkingByDefault(ModelClaudeFable5))
	assert.True(t, modelRunsThinkingByDefault(ModelClaudeMythos5))
	assert.True(t, modelRunsThinkingByDefault(ModelClaudeSonnet5))
	assert.False(t, modelRunsThinkingByDefault(ModelClaudeOpus48))
	assert.False(t, modelDefaultsThinkingOn(ModelClaudeFable5))
	assert.False(t, modelDefaultsThinkingOn(ModelClaudeSonnet46))
}

func TestReasoningEffortFable5UsesOutputConfig(t *testing.T) {
	// Fable 5 takes the native effort parameter, including xhigh.
	req := buildReq(t, ModelClaudeFable5, llm.WithReasoningEffort(llm.ReasoningEffortXHigh))
	assert.NotNil(t, req.OutputConfig)
	assert.Equal(t, "xhigh", req.OutputConfig.Effort)
	assert.Nil(t, req.Thinking)
}

func TestReasoningBudgetFable5FallsBackToAdaptive(t *testing.T) {
	// Fable 5 rejects manual budgets; a budget transparently becomes
	// adaptive thinking so existing callers keep working.
	req := buildReq(t, ModelClaudeFable5, llm.WithReasoningBudget(8000))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "adaptive", req.Thinking.Type)
	assert.Equal(t, 0, req.Thinking.BudgetTokens)
}

func TestThinkingDisabledFable5OmitsThinkingParam(t *testing.T) {
	// Fable 5 rejects an explicit thinking disable; Dive omits the thinking
	// parameter entirely, which is the accepted form.
	req := buildReq(t, ModelClaudeFable5, llm.WithThinking(llm.ThinkingTypeDisabled))
	assert.Nil(t, req.Thinking)
}

func TestReasoningEffortSonnet5UsesOutputConfig(t *testing.T) {
	// Sonnet 5 takes the native effort parameter. It does not support xhigh, so
	// that request degrades to high rather than erroring.
	req := buildReq(t, ModelClaudeSonnet5, llm.WithReasoningEffort(llm.ReasoningEffortXHigh))
	assert.NotNil(t, req.OutputConfig)
	assert.Equal(t, "high", req.OutputConfig.Effort)
	assert.Nil(t, req.Thinking)
}

func TestReasoningBudgetSonnet5FallsBackToAdaptive(t *testing.T) {
	// Manual extended thinking returns a 400 on Sonnet 5; a budget transparently
	// becomes adaptive thinking so existing callers keep working.
	req := buildReq(t, ModelClaudeSonnet5, llm.WithReasoningBudget(8000))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "adaptive", req.Thinking.Type)
	assert.Equal(t, 0, req.Thinking.BudgetTokens)
}

func TestThinkingDisabledSonnet5EmitsExplicitDisable(t *testing.T) {
	// Sonnet 5 defaults thinking on, so a disable must be sent explicitly —
	// omitting the parameter would leave adaptive thinking enabled.
	req := buildReq(t, ModelClaudeSonnet5, llm.WithThinking(llm.ThinkingTypeDisabled))
	assert.NotNil(t, req.Thinking)
	assert.Equal(t, "disabled", req.Thinking.Type)
}

func TestSonnet5RejectsTemperature(t *testing.T) {
	// Sampling parameters return a 400 on Sonnet 5; Dive drops temperature.
	req := buildReq(t, ModelClaudeSonnet5, llm.WithTemperature(0.7))
	assert.Nil(t, req.Temperature)
}
