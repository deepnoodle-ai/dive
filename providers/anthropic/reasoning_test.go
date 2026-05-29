package anthropic

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
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

func TestReasoningEffortOpus48UsesOutputConfig(t *testing.T) {
	// On Opus 4.8 effort maps to output_config.effort (no thinking implied),
	// and the new xhigh level is passed through.
	req := buildReq(t, ModelClaudeOpus48, llm.WithReasoningEffort(llm.ReasoningEffortXHigh))
	assert.NotNil(t, req.OutputConfig)
	assert.Equal(t, "xhigh", req.OutputConfig.Effort)
	assert.Nil(t, req.Thinking)
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

func TestThinkingDisabled(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus48,
		llm.WithReasoningBudget(8000),
		llm.WithThinking(llm.ThinkingTypeDisabled))
	assert.Nil(t, req.Thinking)
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
	assert.False(t, modelSupportsEffortParam(ModelClaude37Sonnet20250219))

	assert.True(t, modelRejectsManualThinking(ModelClaudeOpus47))
	assert.True(t, modelRejectsManualThinking(ModelClaudeOpus48))
	assert.False(t, modelRejectsManualThinking(ModelClaudeOpus46))
	assert.False(t, modelRejectsManualThinking(ModelClaudeSonnet46))
}
