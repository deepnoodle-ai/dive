package dive

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestResponse_OutputText(t *testing.T) {
	t.Run("returns text from last message", func(t *testing.T) {
		resp := &Response{
			Items: []*ResponseItem{
				{
					Type:    ResponseItemTypeMessage,
					Message: (&llm.Message{Role: llm.Assistant}).WithText("first"),
				},
				{
					Type:    ResponseItemTypeMessage,
					Message: (&llm.Message{Role: llm.Assistant}).WithText("second"),
				},
			},
		}
		assert.Equal(t, "second", resp.OutputText())
	})

	t.Run("returns empty string when no messages", func(t *testing.T) {
		resp := &Response{}
		assert.Equal(t, "", resp.OutputText())
	})

	t.Run("returns empty string when message has no text content", func(t *testing.T) {
		resp := &Response{
			Items: []*ResponseItem{
				{
					Type:    ResponseItemTypeMessage,
					Message: &llm.Message{Role: llm.Assistant, Content: []llm.Content{}},
				},
			},
		}
		assert.Equal(t, "", resp.OutputText())
	})

	t.Run("returns last text content from message with multiple text blocks", func(t *testing.T) {
		resp := &Response{
			Items: []*ResponseItem{
				{
					Type:    ResponseItemTypeMessage,
					Message: (&llm.Message{Role: llm.Assistant}).WithText("aaa", "bbb"),
				},
			},
		}
		assert.Equal(t, "bbb", resp.OutputText())
	})

	t.Run("skips non-message items", func(t *testing.T) {
		resp := &Response{
			Items: []*ResponseItem{
				{
					Type:    ResponseItemTypeMessage,
					Message: (&llm.Message{Role: llm.Assistant}).WithText("the answer"),
				},
				{
					Type: ResponseItemTypeToolCall,
					ToolCall: &llm.ToolUseContent{
						ID:   "1",
						Name: "bash",
					},
				},
			},
		}
		assert.Equal(t, "the answer", resp.OutputText())
	})
}

func TestResponse_ToolCallResults(t *testing.T) {
	t.Run("returns all tool call results", func(t *testing.T) {
		resp := &Response{
			Items: []*ResponseItem{
				{
					Type:           ResponseItemTypeToolCallResult,
					ToolCallResult: &ToolCallResult{Name: "tool1"},
				},
				{
					Type:    ResponseItemTypeMessage,
					Message: (&llm.Message{Role: llm.Assistant}).WithText("msg"),
				},
				{
					Type:           ResponseItemTypeToolCallResult,
					ToolCallResult: &ToolCallResult{Name: "tool2"},
				},
			},
		}
		results := resp.ToolCallResults()
		assert.Equal(t, 2, len(results))
		assert.Equal(t, "tool1", results[0].Name)
		assert.Equal(t, "tool2", results[1].Name)
	})

	t.Run("returns nil when no tool call results", func(t *testing.T) {
		resp := &Response{
			Items: []*ResponseItem{
				{
					Type:    ResponseItemTypeMessage,
					Message: (&llm.Message{Role: llm.Assistant}).WithText("hello"),
				},
			},
		}
		results := resp.ToolCallResults()
		assert.Equal(t, 0, len(results))
	})
}

func TestResponse_NoIDField(t *testing.T) {
	resp := &Response{
		Model: "test-model",
	}
	data, err := json.Marshal(resp)
	assert.NoError(t, err)

	var m map[string]any
	err = json.Unmarshal(data, &m)
	assert.NoError(t, err)

	_, hasID := m["id"]
	assert.False(t, hasID, "Response JSON should not contain an 'id' field")
	assert.Equal(t, "test-model", m["model"])
}

func TestResponseItemType(t *testing.T) {
	assert.Equal(t, ResponseItemType("message"), ResponseItemTypeMessage)
	assert.Equal(t, ResponseItemType("tool_call"), ResponseItemTypeToolCall)
	assert.Equal(t, ResponseItemType("tool_call_result"), ResponseItemTypeToolCallResult)
	assert.Equal(t, ResponseItemType("model_event"), ResponseItemTypeModelEvent)
}
