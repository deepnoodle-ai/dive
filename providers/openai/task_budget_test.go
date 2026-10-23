package openai

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

type taskBudgetLogger struct {
	llm.NullLogger
	warnings []string
}

func (l *taskBudgetLogger) Warn(msg string, _ ...any) { l.warnings = append(l.warnings, msg) }
func (l *taskBudgetLogger) With(...any) llm.Logger    { return l }

// OpenAI has no task budget, so the setting is dropped with one warning.
func TestTaskBudgetIgnored(t *testing.T) {
	logger := &taskBudgetLogger{}
	params, err := New(WithModel("gpt-5.4-mini")).buildRequestParams(&llm.Config{
		Messages:   []*llm.Message{llm.NewUserTextMessage("hi")},
		TaskBudget: &llm.TaskBudget{Total: 40000},
		Logger:     logger,
	})
	assert.NoError(t, err)
	body, err := json.Marshal(params)
	assert.NoError(t, err)
	assert.NotContains(t, string(body), "budget")
	assert.Equal(t, []string{"provider does not support task budgets; ignoring the task budget"}, logger.warnings)
}
