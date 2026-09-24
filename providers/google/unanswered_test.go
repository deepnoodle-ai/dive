package google

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// unansweredHistory has a call answered only in part mid-history and an
// unanswered call at the tail.
func unansweredHistory() []*llm.Message {
	return []*llm.Message{
		llm.NewUserTextMessage("hi"),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_a", Name: "lookup", Input: []byte(`{}`)},
			&llm.ToolUseContent{ID: "call_b", Name: "lookup", Input: []byte(`{}`)},
		}},
		llm.NewToolResultMessage(&llm.ToolResultContent{ToolUseID: "call_a", Content: "found"}),
		llm.NewAssistantTextMessage("partly done"),
		llm.NewUserTextMessage("next"),
		{Role: llm.Assistant, Content: []llm.Content{
			&llm.ToolUseContent{ID: "call_c", Name: "lookup", Input: []byte(`{}`)},
		}},
	}
}

func TestGoogleAnswersUnansweredToolCalls(t *testing.T) {
	history := unansweredHistory()
	rendered, err := renderReminderMessages(history)
	assert.NoError(t, err)
	contents, err := messagesToContents(rendered)
	assert.NoError(t, err)
	body, err := json.Marshal(contents)
	assert.NoError(t, err)
	assert.Contains(t, string(body), `"id":"call_b"`)
	assert.Contains(t, string(body), `"id":"call_c"`)
	// One unknown-result answer for each unanswered call.
	assert.Equal(t, strings.Count(string(body), "Unknown result:"), 2)
	assert.Equal(t, len(history), 6)
}
