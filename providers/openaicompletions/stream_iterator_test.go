package openaicompletions

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// newTestStreamIterator creates a StreamIterator that reads from a synthetic
// SSE stream body, mirroring how Provider.Stream constructs the iterator.
func newTestStreamIterator(body string) *StreamIterator {
	reader := io.NopCloser(strings.NewReader(body))
	return &StreamIterator{
		body:            reader,
		reader:          bufio.NewReader(reader),
		contentBlocks:   map[int]*ContentBlockAccumulator{},
		toolCalls:       map[int]*ToolCallAccumulator{},
		toolCallIndices: map[int]int{},
		thinkingIndex:   -1,
		textIndex:       -1,
	}
}

// collectEvents drains the iterator and accumulates all events.
func collectEvents(t *testing.T, iterator *StreamIterator) ([]*llm.Event, *llm.ResponseAccumulator) {
	t.Helper()
	accumulator := llm.NewResponseAccumulator()
	var events []*llm.Event
	for iterator.Next() {
		event := iterator.Event()
		events = append(events, event)
		assert.NoError(t, accumulator.AddEvent(event))
	}
	assert.NoError(t, iterator.Err())
	return events, accumulator
}

// TestStreamIteratorUsageFromTrailingChunk verifies that the token usage
// delivered in the final empty-choices chunk (stream_options.include_usage)
// is reflected in the accumulated response, and that message_stop is still
// the last event.
func TestStreamIteratorUsageFromTrailingChunk(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}`,
		``,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{"content":" world"}}]}`,
		``,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"mistral-large","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	iterator := newTestStreamIterator(body)
	defer iterator.Close()
	events, accumulator := collectEvents(t, iterator)

	assert.NotEmpty(t, events)
	assert.Equal(t, llm.EventTypeMessageStop, events[len(events)-1].Type)
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	assert.Equal(t, "Hello world", response.Message().Text())
	assert.Equal(t, "stop", response.StopReason)
	assert.Equal(t, 12, response.Usage.InputTokens)
	assert.Equal(t, 7, response.Usage.OutputTokens)
}

func TestStreamIteratorPreservesProviderReportedZeroCost(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-free","object":"chat.completion.chunk","model":"free/model","choices":[{"index":0,"delta":{"role":"assistant","content":"cached"}}]}`,
		``,
		`data: {"id":"chatcmpl-free","object":"chat.completion.chunk","model":"free/model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: {"id":"chatcmpl-free","object":"chat.completion.chunk","model":"free/model","choices":[],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0,"cost":0}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	iterator := newTestStreamIterator(body)
	iterator.reportedCostCurrency = "USD"
	t.Cleanup(func() { assert.NoError(t, iterator.Close()) })
	_, accumulator := collectEvents(t, iterator)
	assert.NotNil(t, accumulator.Response().Usage.Cost)
	assert.Equal(t, 0.0, accumulator.Response().Usage.Cost.Total)
	assert.Equal(t, llm.CostSourceProviderReported, accumulator.Response().Usage.Cost.Source)
}

func TestStreamIteratorTreatsNullUsageAsUnavailable(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-null-usage","object":"chat.completion.chunk","model":"gpt-5.5","choices":[{"index":0,"delta":{"role":"assistant","content":"done"}}]}`,
		``,
		`data: {"id":"chatcmpl-null-usage","object":"chat.completion.chunk","model":"gpt-5.5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: {"id":"chatcmpl-null-usage","object":"chat.completion.chunk","model":"gpt-5.5","choices":[],"usage":null}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	iterator := newTestStreamIterator(body)
	iterator.reportedCostCurrency = "USD"
	t.Cleanup(func() { assert.NoError(t, iterator.Close()) })
	_, accumulator := collectEvents(t, iterator)
	response := accumulator.Response()
	assert.Equal(t, "done", response.Message().Text())
	assert.Nil(t, response.Usage.Cost)
	assert.True(t, response.Usage.CostEstimateUnavailable)
}

// TestStreamIteratorSkipsSSEComments verifies that SSE comment lines (any
// line beginning with ':') are ignored rather than parsed as JSON. OpenRouter
// emits ": OPENROUTER PROCESSING" keep-alive comments while a model is queued
// or warming up; parsing one as a data chunk previously failed with
// "invalid character ':' looking for beginning of value".
func TestStreamIteratorSkipsSSEComments(t *testing.T) {
	body := strings.Join([]string{
		`: OPENROUTER PROCESSING`,
		``,
		`data: {"id":"chatcmpl-3","object":"chat.completion.chunk","model":"z-ai/glm-5.2","choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"}}]}`,
		``,
		`: OPENROUTER PROCESSING`,
		``,
		`data: {"id":"chatcmpl-3","object":"chat.completion.chunk","model":"z-ai/glm-5.2","choices":[{"index":0,"delta":{"content":" there"}}]}`,
		``,
		`data: {"id":"chatcmpl-3","object":"chat.completion.chunk","model":"z-ai/glm-5.2","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	iterator := newTestStreamIterator(body)
	defer iterator.Close()
	events, accumulator := collectEvents(t, iterator)

	assert.NotEmpty(t, events)
	assert.Equal(t, llm.EventTypeMessageStop, events[len(events)-1].Type)
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	assert.Equal(t, "Hi there", response.Message().Text())
	assert.Equal(t, "stop", response.StopReason)
}

// TestStreamIteratorToolUseWithTrailingUsage verifies that tool-use streams
// still terminate correctly and report usage from the trailing usage chunk.
func TestStreamIteratorToolUseWithTrailingUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"calculator","arguments":""}}]}}]}`,
		``,
		`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"a\":15,"}}]}}]}`,
		``,
		`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"b\":27}"}}]}}]}`,
		``,
		`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		``,
		`data: {"id":"chatcmpl-2","object":"chat.completion.chunk","model":"mistral-large","choices":[],"usage":{"prompt_tokens":30,"completion_tokens":9,"total_tokens":39}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	iterator := newTestStreamIterator(body)
	defer iterator.Close()
	events, accumulator := collectEvents(t, iterator)

	assert.NotEmpty(t, events)
	assert.Equal(t, llm.EventTypeMessageStop, events[len(events)-1].Type)
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	assert.Equal(t, "tool_use", response.StopReason)
	toolCalls := response.ToolCalls()
	assert.Len(t, toolCalls, 1)
	assert.Equal(t, "call_123", toolCalls[0].ID)
	assert.Equal(t, "calculator", toolCalls[0].Name)
	assert.Equal(t, `{"a":15,"b":27}`, string(toolCalls[0].Input))
	assert.Equal(t, 30, response.Usage.InputTokens)
	assert.Equal(t, 9, response.Usage.OutputTokens)
}

// TestStreamIteratorTerminatesWithoutTrailingUsage verifies that streams
// without a trailing usage chunk (or [DONE] marker) still emit the final
// message_delta and message_stop events when the stream ends.
func TestStreamIteratorTerminatesWithoutTrailingUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-3","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"}}]}`,
		``,
		`data: {"id":"chatcmpl-3","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
	}, "\n")

	iterator := newTestStreamIterator(body)
	defer iterator.Close()
	events, accumulator := collectEvents(t, iterator)

	assert.NotEmpty(t, events)
	assert.Equal(t, llm.EventTypeMessageStop, events[len(events)-1].Type)
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	assert.Equal(t, "Hi", response.Message().Text())
	assert.Equal(t, "stop", response.StopReason)
}

// TestStreamIteratorUsageOnFinishChunk verifies that providers which include
// usage directly on the finish_reason chunk still report it correctly.
func TestStreamIteratorUsageOnFinishChunk(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-4","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{"role":"assistant","content":"Hey"}}]}`,
		``,
		`data: {"id":"chatcmpl-4","object":"chat.completion.chunk","model":"mistral-large","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	iterator := newTestStreamIterator(body)
	defer iterator.Close()
	events, accumulator := collectEvents(t, iterator)

	assert.NotEmpty(t, events)
	assert.Equal(t, llm.EventTypeMessageStop, events[len(events)-1].Type)
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	assert.Equal(t, 5, response.Usage.InputTokens)
	assert.Equal(t, 2, response.Usage.OutputTokens)
}

func TestStreamIteratorPreservesMistralThinkingChunks(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-thinking","model":"mistral-small-latest","choices":[{"index":0,"delta":{"role":"assistant","content":[{"type":"thinking","thinking":[{"type":"text","text":"First "}]}]}}]}`,
		``,
		`data: {"id":"chatcmpl-thinking","model":"mistral-small-latest","choices":[{"index":0,"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"then"}]},{"type":"text","text":"Answer"}]}}]}`,
		``,
		`data: {"id":"chatcmpl-thinking","model":"mistral-small-latest","choices":[{"index":0,"delta":{"content":" done"}}]}`,
		``,
		`data: {"id":"chatcmpl-thinking","model":"mistral-small-latest","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	iterator := newTestStreamIterator(body)
	iterator.providerName = "mistral-mistral-small-latest"
	t.Cleanup(func() { assert.NoError(t, iterator.Close()) })
	_, accumulator := collectEvents(t, iterator)
	response := accumulator.Response()
	assert.Len(t, response.Content, 2)
	thinking := response.Content[0].(*llm.ThinkingContent)
	assert.Equal(t, "First then", thinking.Thinking)
	assert.Equal(t, "true", thinking.Metadata[mistralThinkingMetadataKey])
	answer := response.Content[1].(*llm.TextContent)
	assert.Equal(t, "Answer done", answer.Text)

	replayed, err := convertMessagesForProvider([]*llm.Message{response.Message().Copy()}, iterator.providerName)
	assert.NoError(t, err)
	assert.Len(t, replayed, 1)
	assert.Len(t, replayed[0].ContentParts, 2)
	assert.Equal(t, "thinking", replayed[0].ContentParts[0].Type)
	assert.Equal(t, "First then", replayed[0].ContentParts[0].Thinking[0].Text)
	assert.Equal(t, "Answer done", replayed[0].ContentParts[1].Text)
}

func TestStreamIteratorPreservesOpenRouterReasoningDetails(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-reasoning","model":"google/gemini-3.1-pro-preview","choices":[{"index":0,"delta":{"role":"assistant","reasoning_details":[{"type":"reasoning.text","text":"First ","id":"reasoning-1","format":"google-gemini-v1","index":0}]}}]}`,
		``,
		`data: {"id":"chatcmpl-reasoning","model":"google/gemini-3.1-pro-preview","choices":[{"index":0,"delta":{"reasoning_details":[{"type":"reasoning.text","text":"then","id":"reasoning-1","format":"google-gemini-v1","index":0}]}}]}`,
		``,
		`data: {"id":"chatcmpl-reasoning","model":"google/gemini-3.1-pro-preview","choices":[{"index":0,"delta":{"content":"Answer"}}]}`,
		``,
		`data: {"id":"chatcmpl-reasoning","model":"google/gemini-3.1-pro-preview","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	iterator := newTestStreamIterator(body)
	iterator.providerName = "openrouter"
	t.Cleanup(func() { assert.NoError(t, iterator.Close()) })
	_, accumulator := collectEvents(t, iterator)
	response := accumulator.Response()
	assert.Len(t, response.Content, 2)
	thinking := response.Content[0].(*llm.ThinkingContent)
	assert.Equal(t, "First then", thinking.Thinking)
	var details []map[string]any
	assert.NoError(t, json.Unmarshal(
		[]byte(thinking.Metadata[openRouterReasoningDetailsMetadataKey]),
		&details,
	))
	assert.Len(t, details, 2)
	assert.Equal(t, "First ", details[0]["text"])
	assert.Equal(t, "then", details[1]["text"])

	replayed, err := convertMessagesForProvider(
		[]*llm.Message{response.Message().Copy()},
		iterator.providerName,
	)
	assert.NoError(t, err)
	assert.Len(t, replayed, 1)
	assert.Equal(t, "Answer", replayed[0].Content)
	assert.Empty(t, replayed[0].Reasoning)
	assert.Equal(t,
		thinking.Metadata[openRouterReasoningDetailsMetadataKey],
		string(replayed[0].ReasoningDetails),
	)
}

// TestStreamIteratorUsageDetails verifies that cached prompt tokens and
// reasoning tokens reported in the trailing usage chunk are carried into
// llm.Usage.
func TestStreamIteratorUsageDetails(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl-5","object":"chat.completion.chunk","model":"gpt-5","choices":[{"index":0,"delta":{"role":"assistant","content":"Hi"}}]}`,
		``,
		`data: {"id":"chatcmpl-5","object":"chat.completion.chunk","model":"gpt-5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: {"id":"chatcmpl-5","object":"chat.completion.chunk","model":"gpt-5","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150,"prompt_tokens_details":{"cached_tokens":60,"cache_write_tokens":20},"completion_tokens_details":{"reasoning_tokens":30}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	iterator := newTestStreamIterator(body)
	defer iterator.Close()
	_, accumulator := collectEvents(t, iterator)

	response := accumulator.Response()
	assert.Equal(t, 20, response.Usage.InputTokens)
	assert.Equal(t, 50, response.Usage.OutputTokens)
	assert.Equal(t, 60, response.Usage.CacheReadInputTokens)
	assert.Equal(t, 20, response.Usage.CacheCreationInputTokens)
	assert.Equal(t, 100, response.Usage.TotalInputTokens())
	assert.Equal(t, 30, response.Usage.ReasoningTokens)
}
