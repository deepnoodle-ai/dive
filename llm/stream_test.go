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
