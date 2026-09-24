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

	answer := func(content ...llm.Content) *Response {
		return &Response{Items: []*ResponseItem{{
			Type:    ResponseItemTypeMessage,
			Message: &llm.Message{Role: llm.Assistant, Content: content},
		}}}
	}

	t.Run("skips a trailing empty signed text block", func(t *testing.T) {
		// Gemini can end a reply with an empty part that only carries a
		// thought signature; the google provider keeps it as its own block.
		resp := answer(
			&llm.TextContent{Text: "3 purple"},
			&llm.TextContent{
				Text:     "",
				Metadata: llm.ProviderMetadata{"google.thought_signature": "c2ln"},
			},
		)
		assert.Equal(t, "3 purple", resp.OutputText())
	})

	t.Run("joins adjacent citation fragments with no separator", func(t *testing.T) {
		// Anthropic splits an answer into text blocks at citation boundaries.
		resp := answer(
			&llm.TextContent{Text: "According to the report, "},
			&llm.TextContent{
				Text:      "revenue grew 12%",
				Citations: []llm.Citation{&llm.WebSearchResultLocation{URL: "https://example.com"}},
			},
			&llm.TextContent{Text: " in 2025."},
		)
		assert.Equal(t, "According to the report, revenue grew 12% in 2025.", resp.OutputText())
	})

	t.Run("separates text blocks divided by other content", func(t *testing.T) {
		resp := answer(
			&llm.ThinkingContent{Thinking: "plan"},
			&llm.TextContent{Text: "Searching now."},
			&llm.ThinkingContent{Thinking: "read results"},
			&llm.TextContent{Text: ""},
			&llm.TextContent{Text: "The answer is 42."},
		)
		assert.Equal(t, "Searching now.\n\nThe answer is 42.", resp.OutputText())
	})

	t.Run("returns empty string when all text blocks are empty", func(t *testing.T) {
		resp := answer(&llm.TextContent{Text: ""}, &llm.TextContent{Text: ""})
		assert.Equal(t, "", resp.OutputText())
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
