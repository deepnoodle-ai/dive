package openaicompletions

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// Chat Completions has no task budget, so the setting is dropped with one
// warning.
func TestTaskBudgetIgnored(t *testing.T) {
	logger := &recordingLogger{}
	var req Request
	err := New(WithModel("gpt-5.4-mini")).applyRequestConfig(&req, &llm.Config{
		TaskBudget: &llm.TaskBudget{Total: 40000},
		Logger:     logger,
	})
	assert.NoError(t, err)
	body, err := json.Marshal(req)
	assert.NoError(t, err)
	assert.NotContains(t, string(body), "budget")
	assert.Equal(t, []string{"provider does not support task budgets; ignoring the task budget"}, logger.warnings)
}
