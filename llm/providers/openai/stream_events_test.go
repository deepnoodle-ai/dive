package openai

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamIterator_HelloResponseEvents tests the stream iterator using the real
// OpenAI Responses API events from the hello response fixture.
func TestStreamIterator_HelloResponseEvents(t *testing.T) {
	// Read the fixture file content (newline-delimited JSON)
	fixtureContent, err := os.ReadFile("fixtures/events-hello-response.json")
	require.NoError(t, err)

	// Convert to SSE format (add "data: " prefix to each line)
	lines := strings.Split(string(fixtureContent), "\n")
	var sseData strings.Builder
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			sseData.WriteString("data: " + line + "\n")
		}
	}
	sseData.WriteString("data: [DONE]\n")

	// Create the stream iterator
	reader := strings.NewReader(sseData.String())
	iterator := &StreamIterator{
		reader: llm.NewServerSentEventsReader[StreamEvent](io.NopCloser(reader)),
		body:   io.NopCloser(reader),
	}
	defer iterator.Close()

	// Collect all events
	var events []*llm.Event
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event, "Event should not be nil")
		events = append(events, event)
	}

	require.NoError(t, iterator.Err(), "Iterator should not have an error")

	// Verify we got events
	require.NotEmpty(t, events, "Should receive at least one event")

	// Test event sequence and types
	t.Run("event sequence validation", func(t *testing.T) {
		var eventTypes []llm.EventType
		for _, event := range events {
			eventTypes = append(eventTypes, event.Type)
		}

		// Print actual event types for debugging
		t.Logf("Actual event types: %v", eventTypes)

		// Should start with message_start
		assert.Equal(t, llm.EventTypeMessageStart, eventTypes[0], "First event should be message_start")

		// Should contain content block events
		hasContentBlockStart := false
		hasContentBlockDelta := false
		hasMessageStop := false

		for _, eventType := range eventTypes {
			switch eventType {
			case llm.EventTypeContentBlockStart:
				hasContentBlockStart = true
			case llm.EventTypeContentBlockDelta:
				hasContentBlockDelta = true
			case llm.EventTypeMessageStop:
				hasMessageStop = true
			}
		}

		assert.True(t, hasContentBlockStart, "Should have content_block_start event")
		assert.True(t, hasContentBlockDelta, "Should have content_block_delta events")
		assert.True(t, hasMessageStop, "Should have message_stop event")
	})

	t.Run("message start event validation", func(t *testing.T) {
		messageStartEvent := events[0]
		require.Equal(t, llm.EventTypeMessageStart, messageStartEvent.Type)
		require.NotNil(t, messageStartEvent.Message, "Message start event should have a message")

		msg := messageStartEvent.Message
		assert.Equal(t, "resp_6839c72e8234819a8d43cde34055486201a715438e484503", msg.ID)
		assert.Equal(t, "gpt-4o-2024-08-06", msg.Model)
		assert.Equal(t, llm.Assistant, msg.Role)
		assert.Empty(t, msg.Content, "Initial message should have empty content")
	})

	t.Run("content events validation", func(t *testing.T) {
		// Test the response accumulator to verify proper event handling
		accum := llm.NewResponseAccumulator()
		for _, event := range events {
			err := accum.AddEvent(event)
			require.NoError(t, err, "Accumulator should handle all events without error")
		}

		assert.True(t, accum.IsComplete(), "Accumulator should be complete after all events")

		response := accum.Response()
		require.NotNil(t, response, "Final response should not be nil")
		require.NotEmpty(t, response.Content, "Response should have content")

		// Verify the final text content
		textContent, ok := response.Content[0].(*llm.TextContent)
		require.True(t, ok, "First content block should be text content")
		assert.Equal(t, "Hello! How can I assist you today?", textContent.Text)
	})

	t.Run("bug identification - missing sequence numbers", func(t *testing.T) {
		// The fixture shows the OpenAI API includes sequence_number fields in events,
		// but the current StreamEvent struct doesn't capture these.
		// This could be causing ordering issues in event processing.

		// Also, the fixture shows detailed event types like:
		// - response.created
		// - response.in_progress
		// - response.output_item.added
		// - response.content_part.added
		// - response.output_text.delta
		// - response.output_text.done
		// - response.content_part.done
		// - response.output_item.done
		// - response.completed
		//
		// But the current implementation only looks at "type" field which appears to be
		// "response" for all events, and then inspects the response.output array.
		//
		// This suggests the event processing logic may not be handling the granular
		// event types properly.

		t.Logf("Current implementation may be missing granular event type handling")
		t.Logf("OpenAI sends specific event types like 'response.output_text.delta' but we only process 'response' type")
	})

	t.Run("bug identification - delta handling", func(t *testing.T) {
		// Count delta events - should be one for each delta in the fixture
		deltaCount := 0
		for _, event := range events {
			if event.Type == llm.EventTypeContentBlockDelta {
				deltaCount++
			}
		}

		// The fixture has 9 delta events (Hello, !, " How", " can", " I", " assist", " you", " today", "?")
		// But we might not be processing them correctly
		t.Logf("Received %d delta events", deltaCount)

		// This test will help identify if we're missing deltas or not processing them correctly
		assert.Greater(t, deltaCount, 0, "Should have at least one delta event")
	})
}

// TestStreamIterator_RealEventTypes tests parsing of the actual detailed event types
// from the OpenAI Responses API to identify parsing issues.
func TestStreamIterator_RealEventTypes(t *testing.T) {
	// Test with a single delta event to understand the parsing issue
	deltaEventJSON := `{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_6839c72f5958819a94ecca371ebfe5f801a715438e484503","output_index":0,"content_index":0,"delta":"Hello"}`

	sseData := "data: " + deltaEventJSON + "\ndata: [DONE]\n"

	reader := strings.NewReader(sseData)
	iterator := &StreamIterator{
		reader: llm.NewServerSentEventsReader[StreamEvent](io.NopCloser(reader)),
		body:   io.NopCloser(reader),
	}
	defer iterator.Close()

	// Try to process this event
	hasEvent := iterator.Next()

	if iterator.Err() != nil {
		t.Logf("Error processing delta event: %v", iterator.Err())
		// This suggests the current parsing logic can't handle the granular event types
	} else if !hasEvent {
		t.Logf("No event generated from delta event - this indicates a parsing issue")
	} else {
		event := iterator.Event()
		t.Logf("Successfully processed event: %+v", event)
	}
}

// TestBugReproduction_MissingStreamEventFields tests whether the StreamEvent struct
// is missing important fields from the actual OpenAI API.
func TestBugReproduction_MissingStreamEventFields(t *testing.T) {
	// This test reproduces the likely bug: the StreamEvent struct doesn't match
	// the actual OpenAI API event structure.

	// Test parsing a real API event
	realEvent := `{
		"type": "response.output_text.delta",
		"sequence_number": 4,
		"item_id": "msg_6839c72f5958819a94ecca371ebfe5f801a715438e484503",
		"output_index": 0,
		"content_index": 0,
		"delta": "Hello"
	}`

	var event StreamEvent
	err := json.Unmarshal([]byte(realEvent), &event)

	// This will likely succeed but miss important fields
	require.NoError(t, err)

	// Check what we captured vs what we missed
	t.Logf("Captured event type: %s", event.Type)
	t.Logf("Event response field: %+v", event.Response)

	// The issue is likely that this event structure is different from what
	// the StreamEvent struct expects. The real API sends events with fields like:
	// - sequence_number
	// - item_id
	// - output_index
	// - content_index
	// - delta
	//
	// But StreamEvent only has Type and Response fields.
}
