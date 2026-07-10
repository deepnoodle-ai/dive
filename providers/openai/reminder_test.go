package openai

import (
	"encoding/json"
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
			Messages: []*llm.Message{llm.NewUserTextMessage("continue"), message},
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

	t.Run("custom endpoint from environment falls back to user", func(t *testing.T) {
		t.Setenv("OPENAI_BASE_URL", "https://proxy.example.test/v1")
		provider := New()
		params, err := provider.buildRequestParams(&llm.Config{Messages: []*llm.Message{message}})
		assert.NoError(t, err)
		body, err := json.Marshal(params)
		assert.NoError(t, err)
		assert.Contains(t, string(body), `"role":"user"`)
	})
}
