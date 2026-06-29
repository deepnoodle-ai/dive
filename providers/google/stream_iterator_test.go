package google

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

// chunkSeq builds a streaming sequence from fixture chunks.
func chunkSeq(chunks ...*genai.GenerateContentResponse) iter.Seq2[*genai.GenerateContentResponse, error] {
	return func(yield func(*genai.GenerateContentResponse, error) bool) {
		for _, chunk := range chunks {
			if !yield(chunk, nil) {
				return
			}
		}
	}
}

// collectStreamEvents drains the iterator and accumulates all events.
func collectStreamEvents(t *testing.T, iterator *StreamIterator) ([]*llm.Event, *llm.ResponseAccumulator) {
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

func textChunk(text string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{{Text: text}},
				},
			},
		},
	}
}

func TestStreamIteratorUsageAndStopReason(t *testing.T) {
	final := textChunk(" world")
	final.Candidates[0].FinishReason = genai.FinishReasonStop
	final.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:        100,
		CandidatesTokenCount:    25,
		CachedContentTokenCount: 60,
		ThoughtsTokenCount:      10,
	}

	iterator := NewStreamIteratorFromSeq(context.Background(),
		chunkSeq(textChunk("Hello"), final), "gemini-2.5-pro")
	defer iterator.Close()

	events, accumulator := collectStreamEvents(t, iterator)
	assert.True(t, accumulator.IsComplete())
	assert.Equal(t, llm.EventTypeMessageStop, events[len(events)-1].Type)

	// The message_delta event carries usage and the stop reason
	var messageDelta *llm.Event
	for _, event := range events {
		if event.Type == llm.EventTypeMessageDelta {
			messageDelta = event
		}
	}
	assert.NotNil(t, messageDelta)
	assert.Equal(t, "stop", messageDelta.Delta.StopReason)
	assert.NotNil(t, messageDelta.Usage)
	assert.Equal(t, 100, messageDelta.Usage.InputTokens)

	response := accumulator.Response()
	assert.Equal(t, "Hello world", response.Message().Text())
	assert.Equal(t, "stop", response.StopReason)
	assert.Equal(t, 100, response.Usage.InputTokens)
	assert.Equal(t, 25, response.Usage.OutputTokens)
	assert.Equal(t, 60, response.Usage.CacheReadInputTokens)
	assert.Equal(t, 10, response.Usage.ReasoningTokens)
}

func TestStreamIteratorParallelFunctionCalls(t *testing.T) {
	chunk := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Role: "model",
					Parts: []*genai.Part{
						{Text: "Let me check both."},
						{FunctionCall: &genai.FunctionCall{
							Name: "get_weather",
							Args: map[string]any{"city": "Paris"},
						}, ThoughtSignature: []byte("paris-sig")},
						{FunctionCall: &genai.FunctionCall{
							Name: "get_weather",
							Args: map[string]any{"city": "Tokyo"},
						}},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     50,
			CandidatesTokenCount: 20,
		},
	}

	iterator := NewStreamIteratorFromSeq(context.Background(),
		chunkSeq(chunk), "gemini-2.5-pro")
	defer iterator.Close()

	events, accumulator := collectStreamEvents(t, iterator)
	assert.True(t, accumulator.IsComplete())

	// Content block indices are 0-based and contiguous
	var startIndices []int
	for _, event := range events {
		if event.Type == llm.EventTypeContentBlockStart {
			assert.NotNil(t, event.Index)
			startIndices = append(startIndices, *event.Index)
		}
	}
	assert.Equal(t, []int{0, 1, 2}, startIndices)

	// All parallel function calls are surfaced
	response := accumulator.Response()
	assert.Len(t, response.Content, 3)

	text, ok := response.Content[0].(*llm.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "Let me check both.", text.Text)

	first, ok := response.Content[1].(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "get_weather", first.Name)
	assert.True(t, strings.Contains(string(first.Input), "Paris"))
	assert.Equal(t, "cGFyaXMtc2ln", first.ProviderMetadata[googleThoughtSignatureMetadataKey])

	second, ok := response.Content[2].(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "get_weather", second.Name)
	assert.True(t, strings.Contains(string(second.Input), "Tokyo"))

	// Tool call IDs are unique
	assert.NotEqual(t, first.ID, second.ID)

	// Usage and stop reason still arrive
	assert.Equal(t, 50, response.Usage.InputTokens)
	assert.Equal(t, 20, response.Usage.OutputTokens)
	assert.Equal(t, "stop", response.StopReason)
}

func TestStreamIteratorFunctionCallsAcrossChunks(t *testing.T) {
	callChunk := func(name string, args map[string]any) *genai.GenerateContentResponse {
		return &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: name, Args: args}}},
					},
				},
			},
		}
	}
	final := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{FinishReason: genai.FinishReasonStop},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     30,
			CandidatesTokenCount: 12,
		},
	}

	iterator := NewStreamIteratorFromSeq(context.Background(), chunkSeq(
		callChunk("search", map[string]any{"query": "first"}),
		callChunk("search", map[string]any{"query": "second"}),
		final,
	), "gemini-2.5-pro")
	defer iterator.Close()

	_, accumulator := collectStreamEvents(t, iterator)
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	assert.Len(t, response.Content, 2)
	first, ok := response.Content[0].(*llm.ToolUseContent)
	assert.True(t, ok)
	second, ok := response.Content[1].(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.NotEqual(t, first.ID, second.ID)
	assert.Equal(t, "stop", response.StopReason)
	assert.Equal(t, 30, response.Usage.InputTokens)
	assert.Equal(t, 12, response.Usage.OutputTokens)
}

func TestStreamIteratorMaxTokensStopReason(t *testing.T) {
	final := textChunk("truncated")
	final.Candidates[0].FinishReason = genai.FinishReasonMaxTokens
	final.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     10,
		CandidatesTokenCount: 5,
	}

	iterator := NewStreamIteratorFromSeq(context.Background(),
		chunkSeq(final), "gemini-2.5-pro")
	defer iterator.Close()

	_, accumulator := collectStreamEvents(t, iterator)
	assert.True(t, accumulator.IsComplete())
	assert.Equal(t, "max_tokens", accumulator.Response().StopReason)
}

func TestStreamIteratorStreamError(t *testing.T) {
	seq := func(yield func(*genai.GenerateContentResponse, error) bool) {
		if !yield(textChunk("partial"), nil) {
			return
		}
		yield(nil, errors.New("boom"))
	}
	iterator := NewStreamIteratorFromSeq(context.Background(), seq, "gemini-2.5-pro")
	defer iterator.Close()

	for iterator.Next() {
	}
	assert.Error(t, iterator.Err())
}
