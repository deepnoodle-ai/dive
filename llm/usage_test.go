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

func TestUsageTotalInputTokens(t *testing.T) {
	usage := Usage{
		InputTokens:              40,
		CacheCreationInputTokens: 10,
		CacheReadInputTokens:     50,
	}
	assert.Equal(t, 100, usage.TotalInputTokens())
}

func TestUsageUnmarshalNativeInputDetails(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantInput int
		wantRead  int
		wantWrite int
		wantCost  bool
	}{
		{
			name:      "responses details",
			payload:   `{"input_tokens":100,"input_tokens_details":{"cached_tokens":70}}`,
			wantInput: 30,
			wantRead:  70,
		},
		{
			name:      "responses cache write details",
			payload:   `{"input_tokens":100,"input_tokens_details":{"cached_tokens":20,"cache_write_tokens":70}}`,
			wantInput: 10,
			wantRead:  20,
			wantWrite: 70,
		},
		{
			name:      "chat completions details",
			payload:   `{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":70}}`,
			wantInput: 30,
			wantRead:  70,
		},
		{
			name:      "chat completions cache write details",
			payload:   `{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":20,"cache_write_tokens":70}}`,
			wantInput: 10,
			wantRead:  20,
			wantWrite: 70,
		},
		{
			name: "responses wins when both native shapes are present",
			payload: `{
				"input_tokens":100,
				"input_tokens_details":{"cached_tokens":70},
				"prompt_tokens":80,
				"prompt_tokens_details":{"cached_tokens":20}
			}`,
			wantInput: 30,
			wantRead:  70,
		},
		{
			name: "canonical fields win over native details",
			payload: `{
				"input_tokens":40,
				"cache_read_input_tokens":10,
				"input_tokens_details":{"cached_tokens":30},
				"cost":{"input":1,"total":99}
			}`,
			wantInput: 40,
			wantRead:  10,
			wantCost:  true,
		},
		{
			name: "explicit canonical zero wins",
			payload: `{
				"input_tokens":40,
				"cache_read_input_tokens":0,
				"input_tokens_details":{"cached_tokens":30},
				"cost":{"input":1,"total":99}
			}`,
			wantInput: 40,
			wantRead:  0,
			wantCost:  true,
		},
		{
			name:      "cached tokens clamp above prompt",
			payload:   `{"input_tokens":10,"input_tokens_details":{"cached_tokens":20}}`,
			wantInput: 0,
			wantRead:  10,
		},
		{
			name:      "cache writes clamp to remaining prompt",
			payload:   `{"input_tokens":10,"input_tokens_details":{"cached_tokens":7,"cache_write_tokens":8}}`,
			wantInput: 0,
			wantRead:  7,
			wantWrite: 3,
		},
		{
			name:      "negative native counts clamp to zero",
			payload:   `{"input_tokens":-10,"input_tokens_details":{"cached_tokens":-20}}`,
			wantInput: 0,
			wantRead:  0,
		},
		{
			name: "normalization clears stale cost",
			payload: `{
				"input_tokens":100,
				"input_tokens_details":{"cached_tokens":70},
				"cost":{"input":1,"cache_read":2,"total":3}
			}`,
			wantInput: 30,
			wantRead:  70,
		},
		{
			name: "no token change preserves cost",
			payload: `{
				"input_tokens":100,
				"input_tokens_details":{"cached_tokens":0},
				"cost":{"input":1,"total":99}
			}`,
			wantInput: 100,
			wantRead:  0,
			wantCost:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var usage Usage
			assert.NoError(t, json.Unmarshal([]byte(tt.payload), &usage))
			assert.Equal(t, tt.wantInput, usage.InputTokens)
			assert.Equal(t, tt.wantRead, usage.CacheReadInputTokens)
			assert.Equal(t, tt.wantWrite, usage.CacheCreationInputTokens)
			assert.Equal(t, tt.wantInput+tt.wantRead+tt.wantWrite, usage.TotalInputTokens())
			if tt.wantCost {
				assert.NotNil(t, usage.Cost)
				assert.Equal(t, 99.0, usage.Cost.Total)
			} else {
				assert.Nil(t, usage.Cost)
			}
		})
	}
}

func TestUsageCanonicalRoundTripPreservesCost(t *testing.T) {
	original := Usage{
		InputTokens:          40,
		CacheReadInputTokens: 10,
		Cost: &Cost{
			Input:     1,
			CacheRead: 2,
			Total:     99,
			Currency:  "USD",
			Model:     "test-model",
		},
	}
	body, err := json.Marshal(original)
	assert.NoError(t, err)

	var decoded Usage
	assert.NoError(t, json.Unmarshal(body, &decoded))
	assert.Equal(t, 40, decoded.InputTokens)
	assert.Equal(t, 10, decoded.CacheReadInputTokens)
	assert.NotNil(t, decoded.Cost)
	assert.Equal(t, 99.0, decoded.Cost.Total)
	assert.Equal(t, "test-model", decoded.Cost.Model)
}

func TestUsageAddAndCopyPreserveBillingDetails(t *testing.T) {
	usage := &Usage{
		InputTokens:        10,
		OutputTokens:       20,
		ToolUseInputTokens: 3,
		ServiceTier:        "standard",
		ModalityTokens: map[string]ModalityTokenUsage{
			"text": {InputTokens: 10, OutputTokens: 20},
		},
		Cost: &Cost{Input: 1, Total: 1, Currency: "USD"},
	}
	usage.Add(&Usage{
		InputTokens:                             5,
		OutputTokens:                            7,
		CacheReadInputTokens:                    11,
		ToolUseInputTokens:                      2,
		ReasoningTokens:                         4,
		ServiceTier:                             "priority",
		InputModalityTokenDetailsIncomplete:     true,
		OutputModalityTokenDetailsIncomplete:    true,
		CacheReadModalityTokenDetailsIncomplete: true,
		CacheCreationInputTokensUnavailable:     true,
		ModalityTokens: map[string]ModalityTokenUsage{
			"text":  {InputTokens: 5, OutputTokens: 7},
			"audio": {CacheReadInputTokens: 11},
		},
		Cost: &Cost{Output: 2, Total: 2},
	})

	assert.Equal(t, 15, usage.InputTokens)
	assert.Equal(t, 27, usage.OutputTokens)
	assert.Equal(t, 11, usage.CacheReadInputTokens)
	assert.Equal(t, 5, usage.ToolUseInputTokens)
	assert.Equal(t, 4, usage.ReasoningTokens)
	assert.Equal(t, "mixed", usage.ServiceTier)
	assert.True(t, usage.InputModalityTokenDetailsIncomplete)
	assert.True(t, usage.OutputModalityTokenDetailsIncomplete)
	assert.True(t, usage.CacheReadModalityTokenDetailsIncomplete)
	assert.True(t, usage.CacheCreationInputTokensUnavailable)
	assert.Equal(t, ModalityTokenUsage{InputTokens: 15, OutputTokens: 27}, usage.ModalityTokens["text"])
	assert.Equal(t, ModalityTokenUsage{CacheReadInputTokens: 11}, usage.ModalityTokens["audio"])
	assert.NotNil(t, usage.Cost)
	assert.Equal(t, 3.0, usage.Cost.Total)

	copy := usage.Copy()
	copy.ModalityTokens["text"] = ModalityTokenUsage{InputTokens: 999}
	copy.Cost.Total = 999
	assert.Equal(t, 15, usage.ModalityTokens["text"].InputTokens)
	assert.Equal(t, 3.0, usage.Cost.Total)

	usage.Add(&Usage{CostEstimateUnavailable: true})
	assert.True(t, usage.CostEstimateUnavailable)
	assert.Nil(t, usage.Cost)
}
