package llm

import (
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestUsageUnmarshalAnthropicThinkingTokens(t *testing.T) {
	var usage Usage
	err := json.Unmarshal([]byte(`{
		"input_tokens": 25,
		"output_tokens": 348,
		"output_tokens_details": {
			"thinking_tokens": 312
		}
	}`), &usage)

	assert.NoError(t, err)
	assert.Equal(t, 25, usage.InputTokens)
	assert.Equal(t, 348, usage.OutputTokens)
	assert.Equal(t, 312, usage.ReasoningTokens)
}

func TestUsageUnmarshalOpenAIReasoningTokens(t *testing.T) {
	var usage Usage
	err := json.Unmarshal([]byte(`{
		"input_tokens": 25,
		"output_tokens": 348,
		"output_tokens_details": {
			"reasoning_tokens": 123
		}
	}`), &usage)

	assert.NoError(t, err)
	assert.Equal(t, 123, usage.ReasoningTokens)
}
