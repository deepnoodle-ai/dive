package openai

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestOpenAIReminderRendering(t *testing.T) {
	message := &llm.Message{Role: llm.System, Content: []llm.Content{&llm.ReminderContent{
		Name: "budget", Tier: llm.ReminderTierOperator, Content: "Wrap up",
	}}}

	t.Run("first party uses developer role", func(t *testing.T) {
		provider := New(WithModel("gpt-5.4-mini"))
		params, err := provider.buildRequestParams(&llm.Config{
			Messages:          []*llm.Message{llm.NewUserTextMessage("continue"), message},
			OperatorAuthority: llm.OperatorAuthorityStrict,
		})
		assert.NoError(t, err)
		body, err := json.Marshal(params)
		assert.NoError(t, err)
		assert.Contains(t, string(body), `"role":"developer"`)
		assert.Contains(t, string(body), `system-reminder name=\"budget\"`)
	})

	t.Run("embedded provider falls back to user", func(t *testing.T) {
		provider := New(WithName("grok"), WithEndpoint("https://api.x.ai/v1"))
		params, err := provider.buildRequestParams(&llm.Config{Messages: []*llm.Message{message}})
		assert.NoError(t, err)
		body, err := json.Marshal(params)
		assert.NoError(t, err)
		assert.Contains(t, string(body), `"role":"user"`)
	})

	t.Run("embedded provider strict fails before request", func(t *testing.T) {
		provider := New(WithName("grok"), WithEndpoint("https://api.x.ai/v1"))
		_, err := provider.buildRequestParams(&llm.Config{
			Messages: []*llm.Message{message}, OperatorAuthority: llm.OperatorAuthorityStrict,
		})
		assert.Error(t, err)
		assert.True(t, errors.Is(err, llm.ErrOperatorAuthorityUnavailable))
	})
}
