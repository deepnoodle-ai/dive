package llm

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestContextManagementConfig(t *testing.T) {
	t.Run("ContextManagementConfig structure", func(t *testing.T) {
		config := &ContextManagementConfig{
			Edits: []ContextManagementEdit{
				{
					Type: "clear_tool_uses_20250919",
					Trigger: &ContextManagementTrigger{
						Type:  "input_tokens",
						Value: 30000,
					},
					Keep: &ContextManagementKeep{
						Type:  "tool_uses",
						Value: 3,
					},
					ClearAtLeast: &ContextManagementTrigger{
						Type:  "input_tokens",
						Value: 5000,
					},
					ExcludeTools:    []string{"web_search"},
					ClearToolInputs: true,
				},
				{
					Type: "clear_thinking_20251015",
					Keep: &ContextManagementKeep{
						Type:  "thinking_turns",
						Value: "all",
					},
				},
			},
		}

		assert.Len(t, config.Edits, 2)

		edit1 := config.Edits[0]
		assert.Equal(t, "clear_tool_uses_20250919", edit1.Type)
		assert.Equal(t, "input_tokens", edit1.Trigger.Type)
		assert.Equal(t, 30000, edit1.Trigger.Value)
		assert.Equal(t, "tool_uses", edit1.Keep.Type)
		assert.Equal(t, 3, edit1.Keep.Value)
		assert.Equal(t, "input_tokens", edit1.ClearAtLeast.Type)
		assert.Equal(t, 5000, edit1.ClearAtLeast.Value)
		assert.Len(t, edit1.ExcludeTools, 1)
		assert.Equal(t, "web_search", edit1.ExcludeTools[0])
		assert.True(t, edit1.ClearToolInputs)

		edit2 := config.Edits[1]
		assert.Equal(t, "clear_thinking_20251015", edit2.Type)
		assert.Equal(t, "thinking_turns", edit2.Keep.Type)
		assert.Equal(t, "all", edit2.Keep.Value)
	})

	t.Run("WithContextManagement option", func(t *testing.T) {
		ctx := context.Background()
		config := &Config{}
		cmConfig := &ContextManagementConfig{
			Edits: []ContextManagementEdit{
				{Type: "test_strategy"},
			},
		}

		option := WithContextManagement(cmConfig)
		option(config)

		assert.NotNil(t, config.ContextManagement)
		assert.Equal(t, cmConfig, config.ContextManagement)
		assert.Len(t, config.ContextManagement.Edits, 1)
		assert.Equal(t, "test_strategy", config.ContextManagement.Edits[0].Type)

		// Verify FireHooks doesn't crash (basic check)
		err := config.FireHooks(ctx, &HookContext{})
		assert.NoError(t, err)
	})
}
