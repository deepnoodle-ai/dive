package anthropic

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// warningLogger records the message of each warning.
type warningLogger struct {
	llm.NullLogger
	warnings []string
}

func (l *warningLogger) Warn(msg string, _ ...any) {
	l.warnings = append(l.warnings, msg)
}

func (l *warningLogger) With(...any) llm.Logger { return l }

func TestTaskBudgetSentWithBetaHeader(t *testing.T) {
	_, captured := generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeHaiku55),
		llm.WithReasoningEffort(llm.ReasoningEffortMedium),
		llm.WithTaskBudget(llm.TaskBudget{Total: 40000}),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
	)
	assert.Equal(t, map[string]any{
		"effort":      "medium",
		"task_budget": map[string]any{"type": "tokens", "total": float64(40000)},
	}, captured.body["output_config"])
	assert.Equal(t, []string{FeatureTaskBudgets}, captured.betas)
}

// The task budget beta is joined with the other beta headers, not sent in
// place of them.
func TestTaskBudgetBetaMergesWithOtherBetas(t *testing.T) {
	_, captured := generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeOpus55),
		llm.WithThinkingDisplay(llm.ThinkingDisplayUpdates),
		llm.WithFeatures(FeatureContext1M),
		llm.WithTaskBudget(llm.TaskBudget{Total: 40000}),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
	)
	assert.Equal(t, []string{FeatureContext1M, FeatureThinkingDisplayUpdates, FeatureTaskBudgets}, captured.betas)
}

func TestTaskBudgetWithoutEffort(t *testing.T) {
	req := buildReq(t, ModelClaudeOpus48, llm.WithTaskBudget(llm.TaskBudget{Total: 20000}))
	assert.Equal(t, &OutputConfig{TaskBudget: &TaskBudget{Type: "tokens", Total: 20000}}, req.OutputConfig)
}

func TestTaskBudgetRemaining(t *testing.T) {
	remaining := 12345
	_, captured := generateCaptured(t, okResponse, nil,
		llm.WithModel(ModelClaudeOpus55),
		llm.WithTaskBudget(llm.TaskBudget{Total: 40000, Remaining: &remaining}),
		llm.WithMessages(llm.NewUserTextMessage("hi")),
	)
	outputConfig := captured.body["output_config"].(map[string]any)
	assert.Equal(t, map[string]any{
		"type": "tokens", "total": float64(40000), "remaining": float64(12345),
	}, outputConfig["task_budget"])
}

// A model without task budgets gets no task_budget field and no beta header,
// with one warning, so a ModelSettings can move between models.
func TestTaskBudgetDroppedOnUnsupportedModel(t *testing.T) {
	for _, model := range []string{ModelClaudeSonnet5, ModelClaudeOpus46, ModelClaudeHaiku45} {
		t.Run(model, func(t *testing.T) {
			logger := &warningLogger{}
			_, captured := generateCaptured(t, okResponse, nil,
				llm.WithModel(model),
				llm.WithLogger(logger),
				llm.WithTaskBudget(llm.TaskBudget{Total: 40000}),
				llm.WithMessages(llm.NewUserTextMessage("hi")),
			)
			assert.Nil(t, captured.body["output_config"])
			assert.Len(t, captured.betas, 0)
			assert.Equal(t, []string{"model does not support task budgets; ignoring the task budget"}, logger.warnings)
		})
	}
}

// Dive cannot tell what an unknown model accepts, so it sends the budget as
// asked, as it does effort.
func TestTaskBudgetPassedThroughForUnknownModel(t *testing.T) {
	req := buildReq(t, "custom-model", llm.WithTaskBudget(llm.TaskBudget{Total: 30000}))
	assert.Equal(t, &TaskBudget{Type: "tokens", Total: 30000}, req.OutputConfig.TaskBudget)
}

func TestTaskBudgetBelowMinimumIsRejected(t *testing.T) {
	// The minimum applies on every model, since none accepts less.
	for _, model := range []string{ModelClaudeHaiku55, ModelClaudeSonnet5} {
		t.Run(model, func(t *testing.T) {
			provider := New(WithAPIKey("test-key"), WithEndpoint("http://127.0.0.1:0"))
			_, err := provider.Generate(context.Background(),
				llm.WithModel(model),
				llm.WithTaskBudget(llm.TaskBudget{Total: 19999}),
				llm.WithMessages(llm.NewUserTextMessage("hi")),
			)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "task budget total 19999 is below the minimum of 20000 tokens")
		})
	}
}

func TestTaskBudgetRemainingOutOfRangeIsRejected(t *testing.T) {
	for _, remaining := range []int{-1, 40001} {
		provider := New(WithAPIKey("test-key"), WithEndpoint("http://127.0.0.1:0"))
		_, err := provider.Generate(context.Background(),
			llm.WithModel(ModelClaudeHaiku55),
			llm.WithTaskBudget(llm.TaskBudget{Total: 40000, Remaining: &remaining}),
			llm.WithMessages(llm.NewUserTextMessage("hi")),
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be between 0 and the total 40000")
	}
}
