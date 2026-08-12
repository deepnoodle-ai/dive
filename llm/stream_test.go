package llm

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestResponseAccumulatorSkipsUnknownBlockTypes(t *testing.T) {
	acc := NewResponseAccumulator()
	idx0, idx1 := 0, 1

	assert.NoError(t, acc.AddEvent(&Event{
		Type:    EventTypeMessageStart,
		Message: &Response{ID: "msg_1", Role: Assistant},
	}))

	// An unrecognized content block type (e.g. a server-tool block) must not
	// be stored as a nil entry.
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockStart,
		Index: &idx0,
		ContentBlock: &EventContentBlock{
			Type: ContentType("server_tool_use"),
			ID:   "srvtoolu_1",
			Name: "web_search",
		},
	}))

	// Deltas and stops for the skipped block must not panic or error
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockDelta,
		Index: &idx0,
		Delta: &EventDelta{
			Type:        EventDeltaTypeInputJSON,
			PartialJSON: `{"query":"weather"}`,
		},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockStop,
		Index: &idx0,
	}))

	// A normal text block alongside it still accumulates
	assert.NoError(t, acc.AddEvent(&Event{
		Type:         EventTypeContentBlockStart,
		Index:        &idx1,
		ContentBlock: &EventContentBlock{Type: ContentTypeText},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockDelta,
		Index: &idx1,
		Delta: &EventDelta{Type: EventDeltaTypeText, Text: "hello"},
	}))
	assert.NoError(t, acc.AddEvent(&Event{Type: EventTypeMessageStop}))

	response := acc.Response()
	assert.NotNil(t, response)
	assert.Len(t, response.Content, 1)
	for _, content := range response.Content {
		assert.NotNil(t, content)
	}
	textContent, ok := response.Content[0].(*TextContent)
	assert.True(t, ok)
	assert.Equal(t, textContent.Text, "hello")
}

func TestResponseAccumulatorUsageBeforeMessageStart(t *testing.T) {
	acc := NewResponseAccumulator()
	// A usage-bearing event before message_start must not panic
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypePing,
		Usage: &Usage{InputTokens: 5},
	}))
	assert.Nil(t, acc.Response())
}

func TestResponseAccumulatorUsageIncludesReasoningAndSpeed(t *testing.T) {
	acc := NewResponseAccumulator()
	assert.NoError(t, acc.AddEvent(&Event{
		Type:    EventTypeMessageStart,
		Message: &Response{ID: "msg_1", Role: Assistant},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type: EventTypeMessageDelta,
		Delta: &EventDelta{
			StopReason: "end_turn",
		},
		Usage: &Usage{
			InputTokens:     10,
			OutputTokens:    20,
			ReasoningTokens: 7,
			Speed:           "fast",
		},
	}))
	assert.NoError(t, acc.AddEvent(&Event{Type: EventTypeMessageStop}))

	usage := acc.Response().Usage
	assert.Equal(t, usage.InputTokens, 10)
	assert.Equal(t, usage.OutputTokens, 20)
	assert.Equal(t, usage.ReasoningTokens, 7)
	assert.Equal(t, usage.Speed, "fast")
}

func TestResponseAccumulatorCumulativeUsageFrames(t *testing.T) {
	// Anthropic's message_delta repeats the full cumulative usage that
	// message_start seeded; the accumulator must not count it twice.
	acc := NewResponseAccumulator()
	assert.NoError(t, acc.AddEvent(&Event{
		Type: EventTypeMessageStart,
		Message: &Response{
			ID:   "msg_1",
			Role: Assistant,
			Usage: Usage{
				InputTokens:              14,
				CacheCreationInputTokens: 8012,
				CacheReadInputTokens:     120000,
				OutputTokens:             4,
			},
		},
	}))
	idx := 0
	assert.NoError(t, acc.AddEvent(&Event{
		Type:         EventTypeContentBlockStart,
		Index:        &idx,
		ContentBlock: &EventContentBlock{Type: ContentTypeText},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockDelta,
		Index: &idx,
		Delta: &EventDelta{Type: EventDeltaTypeText, Text: "hello"},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeMessageDelta,
		Delta: &EventDelta{StopReason: "end_turn"},
		Usage: &Usage{
			InputTokens:              14,
			CacheCreationInputTokens: 8012,
			CacheReadInputTokens:     120000,
			OutputTokens:             7,
		},
	}))
	assert.NoError(t, acc.AddEvent(&Event{Type: EventTypeMessageStop}))

	usage := acc.Response().Usage
	assert.Equal(t, usage.InputTokens, 14)
	assert.Equal(t, usage.CacheCreationInputTokens, 8012)
	assert.Equal(t, usage.CacheReadInputTokens, 120000)
	assert.Equal(t, usage.OutputTokens, 7)
	assert.Equal(t, usage.TotalInputTokens(), 128026)
}

func TestResponseAccumulatorOutputOnlyDeltaKeepsSeededInputUsage(t *testing.T) {
	// Some message_delta frames carry only output_tokens; the input buckets
	// seeded by message_start must survive the merge.
	acc := NewResponseAccumulator()
	assert.NoError(t, acc.AddEvent(&Event{
		Type: EventTypeMessageStart,
		Message: &Response{
			ID:   "msg_1",
			Role: Assistant,
			Usage: Usage{
				InputTokens:              14,
				CacheCreationInputTokens: 8012,
				CacheReadInputTokens:     120000,
				OutputTokens:             1,
			},
		},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeMessageDelta,
		Delta: &EventDelta{StopReason: "end_turn"},
		Usage: &Usage{OutputTokens: 104},
	}))
	assert.NoError(t, acc.AddEvent(&Event{Type: EventTypeMessageStop}))

	usage := acc.Response().Usage
	assert.Equal(t, usage.InputTokens, 14)
	assert.Equal(t, usage.CacheCreationInputTokens, 8012)
	assert.Equal(t, usage.CacheReadInputTokens, 120000)
	assert.Equal(t, usage.OutputTokens, 104)
}

func TestResponseAccumulatorPreservesOmittedThinkingSignature(t *testing.T) {
	acc := NewResponseAccumulator()
	idx0, idx1 := 0, 1

	assert.NoError(t, acc.AddEvent(&Event{
		Type:    EventTypeMessageStart,
		Message: &Response{ID: "msg_1", Role: Assistant},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockStart,
		Index: &idx0,
		ContentBlock: &EventContentBlock{
			Type:      ContentTypeThinking,
			Thinking:  "",
			Signature: "",
		},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockDelta,
		Index: &idx0,
		Delta: &EventDelta{
			Type:      EventDeltaTypeSignature,
			Signature: "encrypted-thinking",
		},
	}))
	assert.NoError(t, acc.AddEvent(&Event{Type: EventTypeContentBlockStop, Index: &idx0}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:         EventTypeContentBlockStart,
		Index:        &idx1,
		ContentBlock: &EventContentBlock{Type: ContentTypeText},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockDelta,
		Index: &idx1,
		Delta: &EventDelta{Type: EventDeltaTypeText, Text: "done"},
	}))
	assert.NoError(t, acc.AddEvent(&Event{Type: EventTypeMessageStop}))

	response := acc.Response()
	assert.Len(t, response.Content, 2)
	thinking, ok := response.Content[0].(*ThinkingContent)
	assert.True(t, ok)
	assert.Equal(t, "", thinking.Thinking)
	assert.Equal(t, "encrypted-thinking", thinking.Signature)
	text, ok := response.Content[1].(*TextContent)
	assert.True(t, ok)
	assert.Equal(t, "done", text.Text)
}

func TestResponseAccumulatorMergesProviderMetadataDelta(t *testing.T) {
	acc := NewResponseAccumulator()
	index := 0
	assert.NoError(t, acc.AddEvent(&Event{
		Type:    EventTypeMessageStart,
		Message: &Response{ID: "msg_1", Role: Assistant},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockStart,
		Index: &index,
		ContentBlock: &EventContentBlock{
			Type:     ContentTypeThinking,
			Metadata: ProviderMetadata{"provider.initial": "true"},
		},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockDelta,
		Index: &index,
		Delta: &EventDelta{
			Type: EventDeltaTypeMetadata,
			Metadata: ProviderMetadata{
				"provider.initial": "updated",
				"provider.detail":  "preserved",
			},
		},
	}))
	assert.NoError(t, acc.AddEvent(&Event{Type: EventTypeMessageStop}))

	thinking := acc.Response().Content[0].(*ThinkingContent)
	assert.Equal(t, "updated", thinking.Metadata["provider.initial"])
	assert.Equal(t, "preserved", thinking.Metadata["provider.detail"])
}

func TestResponseAccumulatorPreservesContentMetadataAndThinkingID(t *testing.T) {
	acc := NewResponseAccumulator()
	textIndex, thinkingIndex := 0, 1

	assert.NoError(t, acc.AddEvent(&Event{
		Type:    EventTypeMessageStart,
		Message: &Response{ID: "msg_1", Role: Assistant},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockStart,
		Index: &textIndex,
		ContentBlock: &EventContentBlock{
			Type:     ContentTypeText,
			Metadata: ProviderMetadata{"google.thought_signature": "text-sig"},
		},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockDelta,
		Index: &textIndex,
		Delta: &EventDelta{Type: EventDeltaTypeText, Text: "answer"},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockStart,
		Index: &thinkingIndex,
		ContentBlock: &EventContentBlock{
			Type:     ContentTypeThinking,
			ID:       "rs_123",
			Metadata: ProviderMetadata{"google.thought": "true"},
		},
	}))
	assert.NoError(t, acc.AddEvent(&Event{
		Type:  EventTypeContentBlockDelta,
		Index: &thinkingIndex,
		Delta: &EventDelta{Type: EventDeltaTypeThinking, Thinking: "summary"},
	}))
	assert.NoError(t, acc.AddEvent(&Event{Type: EventTypeMessageStop}))

	response := acc.Response()
	text := response.Content[0].(*TextContent)
	assert.Equal(t, "text-sig", text.Metadata["google.thought_signature"])
	thinking := response.Content[1].(*ThinkingContent)
	assert.Equal(t, "rs_123", thinking.ID)
	assert.Equal(t, "true", thinking.Metadata["google.thought"])
}
