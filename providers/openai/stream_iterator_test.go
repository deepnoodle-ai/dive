package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/openai/openai-go/v3/responses"
)

// mockStreamSource replays pre-parsed SDK events through the StreamSource
// interface used by the iterator.
type mockStreamSource struct {
	events []responses.ResponseStreamEventUnion
	pos    int
}

func (m *mockStreamSource) Next() bool {
	if m.pos < len(m.events) {
		m.pos++
		return true
	}
	return false
}

func (m *mockStreamSource) Current() responses.ResponseStreamEventUnion {
	return m.events[m.pos-1]
}

func (m *mockStreamSource) Err() error { return nil }

func (m *mockStreamSource) Close() error { return nil }

// loadFixtureEvents parses an SSE fixture file into SDK stream events.
func loadFixtureEvents(t *testing.T, path string) []responses.ResponseStreamEventUnion {
	t.Helper()
	data, err := os.ReadFile(path)
	assert.NoError(t, err)

	var events []responses.ResponseStreamEventUnion
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		var event responses.ResponseStreamEventUnion
		assert.NoError(t, json.Unmarshal(bytes.TrimPrefix(line, []byte("data: ")), &event))
		events = append(events, event)
	}
	assert.NoError(t, scanner.Err())
	assert.NotEmpty(t, events)
	return events
}

// TestStreamIteratorZeroBasedIndices verifies that content block events carry
// the zero-based output_index from the OpenAI Responses API as-is. The fixture
// stream has a single message item at output_index 0, so every indexed event
// must carry index 0 (never -1).
func TestStreamIteratorZeroBasedIndices(t *testing.T) {
	source := &mockStreamSource{events: loadFixtureEvents(t, "fixtures/events-hello.txt")}
	iterator := newOpenAIStreamIterator(source, &llm.Config{})
	defer iterator.Close()

	accumulator := llm.NewResponseAccumulator()
	var indexedEventCount int
	var firstBlockStartIndex *int
	for iterator.Next() {
		event := iterator.Event()
		if event.Index != nil {
			indexedEventCount++
			assert.Equal(t, 0, *event.Index)
		}
		if event.Type == llm.EventTypeContentBlockStart && firstBlockStartIndex == nil {
			firstBlockStartIndex = event.Index
		}
		assert.NoError(t, accumulator.AddEvent(event))
	}
	assert.NoError(t, iterator.Err())

	// The first content block start must carry index 0.
	assert.NotNil(t, firstBlockStartIndex)
	assert.Equal(t, 0, *firstBlockStartIndex)
	assert.NotEmpty(t, indexedEventCount)

	// The accumulated response should match the fixture.
	assert.True(t, accumulator.IsComplete())
	response := accumulator.Response()
	assert.Equal(t, "Hello! How can I assist you today?", response.Message().Text())
	assert.Equal(t, 140, response.Usage.InputTokens)
	assert.Equal(t, 11, response.Usage.OutputTokens)
}

func TestStreamIteratorNormalizesUsageBuckets(t *testing.T) {
	tests := []struct {
		name       string
		eventType  string
		prompt     int
		cached     int
		written    int
		wantInput  int
		wantCached int
		wantWrite  int
	}{
		{name: "completed valid", eventType: "response.completed", prompt: 100, cached: 20, written: 70, wantInput: 10, wantCached: 20, wantWrite: 70},
		{name: "completed upper clamp", eventType: "response.completed", prompt: 10, cached: 20, wantInput: 0, wantCached: 10},
		{name: "completed write clamp", eventType: "response.completed", prompt: 10, cached: 7, written: 8, wantInput: 0, wantCached: 7, wantWrite: 3},
		{name: "completed lower clamp", eventType: "response.completed", prompt: 10, cached: -20, written: -5, wantInput: 10, wantCached: 0},
		{name: "incomplete valid", eventType: "response.incomplete", prompt: 100, cached: 20, written: 70, wantInput: 10, wantCached: 20, wantWrite: 70},
		{name: "incomplete upper clamp", eventType: "response.incomplete", prompt: 10, cached: 20, wantInput: 0, wantCached: 10},
		{name: "incomplete negative prompt", eventType: "response.incomplete", prompt: -10, cached: 5, written: 5, wantInput: 0, wantCached: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var event responses.ResponseStreamEventUnion
			assert.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{
				"type":%q,
				"sequence_number":1,
				"response":{
					"status":"%s",
					"usage":{
						"input_tokens":%d,
						"output_tokens":5,
						"input_tokens_details":{"cached_tokens":%d,"cache_write_tokens":%d}
					}
				}
			}`, tt.eventType, eventStatus(tt.eventType), tt.prompt, tt.cached, tt.written)), &event))

			iterator := newOpenAIStreamIterator(&mockStreamSource{}, &llm.Config{})
			events, err := iterator.processOpenAIEvent(event)
			assert.NoError(t, err)
			assert.NotEmpty(t, events)
			assert.NotNil(t, events[0].Usage)
			assert.Equal(t, tt.wantInput, events[0].Usage.InputTokens)
			assert.Equal(t, tt.wantCached, events[0].Usage.CacheReadInputTokens)
			assert.Equal(t, tt.wantWrite, events[0].Usage.CacheCreationInputTokens)
			assert.Equal(t, max(0, tt.prompt), events[0].Usage.TotalInputTokens())
		})
	}
}

func eventStatus(eventType string) string {
	if eventType == "response.incomplete" {
		return "incomplete"
	}
	return "completed"
}

func TestStreamIteratorAccumulatorPricesDisjointUsage(t *testing.T) {
	payloads := []string{
		`{"type":"response.created","sequence_number":1,"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"in_progress"}}`,
		`{"type":"response.completed","sequence_number":2,"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":100,"output_tokens":10,"input_tokens_details":{"cached_tokens":60,"cache_write_tokens":20}}}}`,
	}
	events := make([]responses.ResponseStreamEventUnion, 0, len(payloads))
	for _, payload := range payloads {
		var event responses.ResponseStreamEventUnion
		assert.NoError(t, json.Unmarshal([]byte(payload), &event))
		events = append(events, event)
	}

	iterator := newOpenAIStreamIterator(&mockStreamSource{events: events}, &llm.Config{})
	t.Cleanup(func() {
		assert.NoError(t, iterator.Close())
	})
	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		assert.NoError(t, accumulator.AddEvent(iterator.Event()))
	}
	assert.NoError(t, iterator.Err())

	usage := accumulator.Response().Usage
	assert.Equal(t, 20, usage.InputTokens)
	assert.Equal(t, 60, usage.CacheReadInputTokens)
	assert.Equal(t, 20, usage.CacheCreationInputTokens)
	assert.Equal(t, 100, usage.TotalInputTokens())
	assert.NotNil(t, usage.Cost)
	want := TextModelPricing[ModelGPT56Sol].CostOf(&usage)
	assert.Equal(t, want.Input, usage.Cost.Input)
	assert.Equal(t, want.CacheRead, usage.Cost.CacheRead)
	assert.Equal(t, want.CacheWrite, usage.Cost.CacheWrite)
	assert.Equal(t, want.Total, usage.Cost.Total)
}

func TestStreamIteratorPreservesReplayableReasoning(t *testing.T) {
	payloads := []string{
		`{"type":"response.created","sequence_number":1,"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"in_progress"}}`,
		`{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"rs_1","type":"reasoning","status":"in_progress","summary":[]}}`,
		`{"type":"response.reasoning_summary_part.added","sequence_number":3,"item_id":"rs_1","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`,
		`{"type":"response.reasoning_summary_text.delta","sequence_number":4,"item_id":"rs_1","output_index":0,"summary_index":0,"delta":"why"}`,
		`{"type":"response.reasoning_summary_part.done","sequence_number":5,"item_id":"rs_1","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":"why"}}`,
		`{"type":"response.output_item.done","sequence_number":6,"output_index":0,"item":{"id":"rs_1","type":"reasoning","status":"completed","encrypted_content":"encrypted-reasoning","summary":[{"type":"summary_text","text":"why"}]}}`,
		`{"type":"response.completed","sequence_number":7,"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens_details":{"reasoning_tokens":1}}}}`,
	}
	events := make([]responses.ResponseStreamEventUnion, 0, len(payloads))
	for _, payload := range payloads {
		var event responses.ResponseStreamEventUnion
		assert.NoError(t, json.Unmarshal([]byte(payload), &event))
		events = append(events, event)
	}

	iterator := newOpenAIStreamIterator(&mockStreamSource{events: events}, &llm.Config{})
	t.Cleanup(func() {
		assert.NoError(t, iterator.Close())
	})
	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		assert.NoError(t, accumulator.AddEvent(iterator.Event()))
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())

	message := accumulator.Response().Message()
	assert.Equal(t, 1, len(message.Content))
	thinking, ok := message.Content[0].(*llm.ThinkingContent)
	assert.True(t, ok)
	assert.Equal(t, "rs_1", thinking.ID)
	assert.Equal(t, "why", thinking.Thinking)
	assert.Equal(t, "encrypted-reasoning", thinking.Signature)
	assert.Equal(t, `["why"]`, thinking.Metadata[openAIReasoningSummaryMetadataKey])

	replayed, err := encodeMessages([]*llm.Message{message})
	assert.NoError(t, err)
	replayedJSON, err := json.Marshal(replayed)
	assert.NoError(t, err)
	assert.Contains(t, string(replayedJSON), `"id":"rs_1"`)
	assert.Contains(t, string(replayedJSON), `"encrypted_content":"encrypted-reasoning"`)
	assert.Contains(t, string(replayedJSON), `"text":"why"`)
}

func TestStreamIteratorPreservesRawReasoningText(t *testing.T) {
	payloads := []string{
		`{"type":"response.created","sequence_number":1,"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"in_progress"}}`,
		`{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"rs_raw","type":"reasoning","status":"in_progress","summary":[],"content":[]}}`,
		`{"type":"response.reasoning_text.done","sequence_number":3,"item_id":"rs_raw","output_index":0,"content_index":0,"text":"raw reasoning"}`,
		`{"type":"response.output_item.done","sequence_number":5,"output_index":0,"item":{"id":"rs_raw","type":"reasoning","status":"completed","encrypted_content":"encrypted-raw","summary":[],"content":[{"type":"reasoning_text","text":"raw reasoning"}]}}`,
		`{"type":"response.completed","sequence_number":6,"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens_details":{"reasoning_tokens":1}}}}`,
	}
	events := make([]responses.ResponseStreamEventUnion, 0, len(payloads))
	for _, payload := range payloads {
		var event responses.ResponseStreamEventUnion
		assert.NoError(t, json.Unmarshal([]byte(payload), &event))
		events = append(events, event)
	}

	iterator := newOpenAIStreamIterator(&mockStreamSource{events: events}, &llm.Config{})
	t.Cleanup(func() { assert.NoError(t, iterator.Close()) })
	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		assert.NoError(t, accumulator.AddEvent(iterator.Event()))
	}
	assert.NoError(t, iterator.Err())

	message := accumulator.Response().Message()
	thinking := message.Content[0].(*llm.ThinkingContent)
	assert.Equal(t, "rs_raw", thinking.ID)
	assert.Equal(t, "raw reasoning", thinking.Thinking)
	assert.Equal(t, "encrypted-raw", thinking.Signature)
	assert.Equal(t, `["raw reasoning"]`, thinking.Metadata[openAIReasoningContentMetadataKey])

	replayed, err := encodeMessages([]*llm.Message{message})
	assert.NoError(t, err)
	replayedJSON, err := json.Marshal(replayed)
	assert.NoError(t, err)
	assert.Contains(t, string(replayedJSON), `"content":[{"text":"raw reasoning","type":"reasoning_text"}]`)
}

// parseStreamPayloads decodes raw SSE event payloads into SDK stream events.
func parseStreamPayloads(t *testing.T, payloads ...string) []responses.ResponseStreamEventUnion {
	t.Helper()
	events := make([]responses.ResponseStreamEventUnion, 0, len(payloads))
	for _, payload := range payloads {
		var event responses.ResponseStreamEventUnion
		assert.NoError(t, json.Unmarshal([]byte(payload), &event))
		events = append(events, event)
	}
	return events
}

// TestStreamIteratorClosesDanglingBlocks covers a response that ends without
// the output_item.done event that normally closes a block. Whichever kind of
// block was left open — text, function call, or reasoning — the iterator must
// close it exactly once, and before message_delta, so consumers always see a
// balanced block lifecycle in the same order the other providers emit.
func TestStreamIteratorClosesDanglingBlocks(t *testing.T) {
	created := `{"type":"response.created","sequence_number":1,"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"in_progress"}}`
	incomplete := `{"type":"response.incomplete","sequence_number":9,"response":{"id":"resp_1","model":"gpt-5.6-sol","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0,"cache_write_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`

	cases := []struct {
		name     string
		payloads []string
		content  llm.ContentType
	}{
		{
			name: "text",
			payloads: []string{
				`{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"msg_1","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
				`{"type":"response.content_part.added","sequence_number":3,"item_id":"msg_1","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
				`{"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Partial"}`,
			},
			content: llm.ContentTypeText,
		},
		{
			name: "function_call",
			payloads: []string{
				`{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"lookup","arguments":"","status":"in_progress"}}`,
				`{"type":"response.function_call_arguments.delta","sequence_number":3,"item_id":"fc_1","output_index":0,"delta":"{\"q\":\"x\"}"}`,
			},
			content: llm.ContentTypeToolUse,
		},
		{
			name: "reasoning",
			payloads: []string{
				`{"type":"response.output_item.added","sequence_number":2,"output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[],"status":"in_progress"}}`,
				`{"type":"response.reasoning_summary_part.added","sequence_number":3,"item_id":"rs_1","output_index":0,"summary_index":0,"part":{"type":"summary_text","text":""}}`,
				`{"type":"response.reasoning_summary_text.delta","sequence_number":4,"item_id":"rs_1","output_index":0,"summary_index":0,"delta":"Thinking"}`,
			},
			content: llm.ContentTypeThinking,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payloads := append(append([]string{created}, tc.payloads...), incomplete)
			iterator := newOpenAIStreamIterator(&mockStreamSource{events: parseStreamPayloads(t, payloads...)}, &llm.Config{})
			defer iterator.Close()

			accumulator := llm.NewResponseAccumulator()
			var types []llm.EventType
			for iterator.Next() {
				event := iterator.Event()
				types = append(types, event.Type)
				assert.NoError(t, accumulator.AddEvent(event))
			}
			assert.NoError(t, iterator.Err())

			var starts, stops int
			stopIndex, deltaIndex := -1, -1
			for i, eventType := range types {
				switch eventType {
				case llm.EventTypeContentBlockStart:
					starts++
				case llm.EventTypeContentBlockStop:
					stops++
					stopIndex = i
				case llm.EventTypeMessageDelta:
					deltaIndex = i
				}
			}
			assert.Equal(t, 1, starts)
			assert.Equal(t, 1, stops)
			assert.True(t, deltaIndex >= 0, "expected a message_delta event")
			assert.True(t, stopIndex < deltaIndex, "content_block_stop must precede message_delta, got %v", types)
			assert.Equal(t, llm.EventTypeMessageStop, types[len(types)-1])

			// The partial block still reaches consumers as a well-formed message.
			assert.True(t, accumulator.IsComplete())
			message := accumulator.Response().Message()
			assert.Equal(t, 1, len(message.Content))
			assert.Equal(t, tc.content, message.Content[0].Type())
		})
	}
}
