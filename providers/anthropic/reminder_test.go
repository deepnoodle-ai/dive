package anthropic

import (
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func operatorReminderMessage() *llm.Message {
	return &llm.Message{Role: llm.System, Content: []llm.Content{&llm.ReminderContent{
		Name: "mode", Tier: llm.ReminderTierOperator, Content: "Read only",
	}}}
}

func TestAnthropicReminderRendering(t *testing.T) {
	provider := New(WithModel(ModelClaudeOpus48))

	t.Run("native system at legal placement", func(t *testing.T) {
		messages := []*llm.Message{llm.NewUserTextMessage("continue"), operatorReminderMessage()}
		rendered, err := provider.renderReminders(messages, ModelClaudeOpus48)
		assert.NoError(t, err)
		assert.Equal(t, llm.System, rendered[1].Role)
		assert.Contains(t, rendered[1].Content[0].(*llm.TextContent).Text, `name="mode"`)
	})

	t.Run("illegal placement downgrades to tagged user", func(t *testing.T) {
		messages := []*llm.Message{operatorReminderMessage(), llm.NewUserTextMessage("continue")}
		rendered, err := provider.renderReminders(messages, ModelClaudeOpus48)
		assert.NoError(t, err)
		assert.Equal(t, llm.User, rendered[0].Role)
	})

	t.Run("leading reminder with no user turn downgrades to tagged user", func(t *testing.T) {
		messages := []*llm.Message{operatorReminderMessage()}
		rendered, err := provider.renderReminders(messages, ModelClaudeOpus48)
		assert.NoError(t, err)
		assert.Equal(t, llm.User, rendered[0].Role)
	})

	t.Run("native system after server tool use turn", func(t *testing.T) {
		serverToolUseMessage := &llm.Message{Role: llm.Assistant, Content: []llm.Content{
			&llm.ServerToolUseContent{ID: "srvtool_1", Name: "web_search", Input: map[string]any{"query": "weather"}},
		}}
		messages := []*llm.Message{llm.NewUserTextMessage("continue"), serverToolUseMessage, operatorReminderMessage()}
		rendered, err := provider.renderReminders(messages, ModelClaudeOpus48)
		assert.NoError(t, err)
		assert.Equal(t, llm.System, rendered[2].Role)
		assert.Contains(t, rendered[2].Content[0].(*llm.TextContent).Text, `name="mode"`)
	})

	t.Run("custom endpoint is not assumed native", func(t *testing.T) {
		custom := New(WithEndpoint("https://example.test/v1/messages"), WithModel(ModelClaudeOpus48))
		messages := []*llm.Message{llm.NewUserTextMessage("continue"), operatorReminderMessage()}
		rendered, err := custom.renderReminders(messages, ModelClaudeOpus48)
		assert.NoError(t, err)
		assert.Equal(t, llm.User, rendered[1].Role)
	})
}
