package orchestration

import (
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// The answer so far is the last assistant message the incomplete turn kept,
// which may have no message item, as text a stream was cut off in does not.
func TestPartialAnswerReadsOutputMessages(t *testing.T) {
	response := &dive.Response{
		Status: dive.ResponseStatusIncomplete,
		Items: []*dive.ResponseItem{{
			Type:    dive.ResponseItemTypeMessage,
			Message: llm.NewAssistantTextMessage("Let me check..."),
		}},
		OutputMessages: []*llm.Message{
			llm.NewAssistantTextMessage("Let me check..."),
			llm.NewUserTextMessage("tool results"),
			llm.NewAssistantTextMessage("The answer is"),
			llm.NewUserTextMessage("outcome"),
		},
		Turn: &dive.Turn{Outcome: &dive.TurnOutcome{Reason: dive.TurnReasonStreamInterrupted}},
	}
	assert.Equal(t, partialAnswer(response), "The answer is")
	assert.Contains(t, subagentOutput(response), "The answer is")
	assert.Equal(t, partialAnswer(&dive.Response{Status: dive.ResponseStatusCompleted}), "")
}
