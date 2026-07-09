package google

import (
	"errors"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestGoogleReminderRenderingUsesUserFallback(t *testing.T) {
	message := &llm.Message{Role: llm.System, Content: []llm.Content{&llm.ReminderContent{
		Name: "mode", Tier: llm.ReminderTierOperator, Content: "Read only",
	}}}
	rendered, err := renderReminderMessages([]*llm.Message{message}, llm.OperatorAuthorityBestEffort)
	assert.NoError(t, err)
	assert.Equal(t, llm.User, rendered[0].Role)
	assert.Contains(t, rendered[0].Content[0].(*llm.TextContent).Text, `name="mode"`)

	_, err = renderReminderMessages([]*llm.Message{message}, llm.OperatorAuthorityStrict)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, llm.ErrOperatorAuthorityUnavailable))
}
