package openaicompletions

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestReminderRenderingFallsBackForChatCompletions(t *testing.T) {
	message := &llm.Message{Role: llm.System, Content: []llm.Content{&llm.ReminderContent{
		Name: "mode", Tier: llm.ReminderTierOperator, Content: "Read only",
	}}}
	converted, err := convertMessages([]*llm.Message{message})
	assert.NoError(t, err)
	assert.Len(t, converted, 1)
	assert.Equal(t, "user", converted[0].Role)
	assert.Contains(t, converted[0].Content, `name="mode"`)
}
