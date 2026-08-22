package google

import (
	"context"
	"encoding/base64"
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
		TotalTokenCount:         135,
	}
	final.ResponseID = "google-response-id"
	final.ModelVersion = "gemini-2.5-pro"

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
	assert.Equal(t, 40, messageDelta.Usage.InputTokens)

	response := accumulator.Response()
	assert.Equal(t, "Hello world", response.Message().Text())
	assert.Equal(t, "stop", response.StopReason)
	assert.Equal(t, 40, response.Usage.InputTokens)
	assert.Equal(t, 35, response.Usage.OutputTokens)
	assert.Equal(t, 60, response.Usage.CacheReadInputTokens)
	assert.Equal(t, 100, response.Usage.TotalInputTokens())
	assert.Equal(t, 10, response.Usage.ReasoningTokens)
	assert.True(t, response.Usage.CacheCreationInputTokensUnavailable)
	assert.Equal(t, "google-response-id", response.ID)
	assert.Equal(t, "gemini-2.5-pro", response.Model)
}

func TestStreamIteratorPreservesContentWhenUsageTotalDoesNotReconcile(t *testing.T) {
	final := textChunk(" answer")
	final.Candidates[0].FinishReason = genai.FinishReasonStop
	final.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     10,
		CandidatesTokenCount: 5,
		TotalTokenCount:      99,
	}

	iterator := NewStreamIteratorFromSeq(context.Background(),
		chunkSeq(textChunk("completed"), final), ModelGemini25Pro)
	t.Cleanup(func() { assert.NoError(t, iterator.Close()) })

	events, accumulator := collectStreamEvents(t, iterator)
	assert.Equal(t, llm.EventTypeMessageStop, events[len(events)-1].Type)
	response := accumulator.Response()
	assert.Equal(t, "completed answer", response.Message().Text())
	assert.Equal(t, 10, response.Usage.InputTokens)
	assert.Equal(t, 5, response.Usage.OutputTokens)
	assert.True(t, response.Usage.CostEstimateUnavailable)
	assert.Nil(t, response.Usage.Cost)
}

func TestStreamIteratorUsesServedModelAndPreservesRegionalCost(t *testing.T) {
	final := textChunk("done")
	final.ResponseID = "served-response-id"
	final.ModelVersion = ModelGemini35Flash
	final.Candidates[0].FinishReason = genai.FinishReasonStop
	final.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     1_000_000,
		CandidatesTokenCount: 1_000_000,
		TotalTokenCount:      2_000_000,
		PromptTokensDetails: []*genai.ModalityTokenCount{{
			Modality: genai.MediaModalityText, TokenCount: 1_000_000,
		}},
		CandidatesTokensDetails: []*genai.ModalityTokenCount{{
			Modality: genai.MediaModalityText, TokenCount: 1_000_000,
		}},
	}

	iterator := newStreamIteratorFromSeq(context.Background(),
		chunkSeq(textChunk("starting "), final), "gemini-request-alias", googlePricingContext{
			vertexAI: true,
			location: "us-central1",
		})
	t.Cleanup(func() { assert.NoError(t, iterator.Close()) })

	_, accumulator := collectStreamEvents(t, iterator)
	response := accumulator.Response()
	assert.Equal(t, "served-response-id", response.ID)
	assert.Equal(t, ModelGemini35Flash, response.Model)
	assert.NotNil(t, response.Usage.Cost)
	assert.InDelta(t, 1.65, response.Usage.Cost.Input, 1e-12)
	assert.InDelta(t, 9.90, response.Usage.Cost.Output, 1e-12)
	assert.InDelta(t, 11.55, response.Usage.Cost.Total, 1e-12)
	assert.Equal(t, ModelGemini35Flash, response.Usage.Cost.Model)
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
			TotalTokenCount:      70,
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
	assert.Equal(t, "cGFyaXMtc2ln", first.Metadata[googleThoughtSignatureMetadataKey])

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
			TotalTokenCount:      42,
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
		TotalTokenCount:      15,
	}

	iterator := NewStreamIteratorFromSeq(context.Background(),
		chunkSeq(final), "gemini-2.5-pro")
	defer iterator.Close()

	_, accumulator := collectStreamEvents(t, iterator)
	assert.True(t, accumulator.IsComplete())
	assert.Equal(t, "max_tokens", accumulator.Response().StopReason)
}

func TestStreamIteratorPreservesSignedThinkingAndEmptyTextParts(t *testing.T) {
	thoughtSignature := []byte("thought-sig")
	textSignature := []byte("text-sig")
	chunks := chunkSeq(
		&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{{Text: "thinking ", Thought: true}}},
		}}},
		&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{{
				Text: "summary", Thought: true, ThoughtSignature: thoughtSignature,
			}}},
		}}},
		&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{{Text: "answer"}}},
		}}},
		&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []*genai.Part{{ThoughtSignature: textSignature}}},
			FinishReason: genai.FinishReasonStop,
		}}},
	)

	iterator := NewStreamIteratorFromSeq(context.Background(), chunks, ModelGemini36Flash)
	t.Cleanup(func() { assert.NoError(t, iterator.Close()) })
	_, accumulator := collectStreamEvents(t, iterator)
	response := accumulator.Response()
	assert.Len(t, response.Content, 4)

	firstThought := response.Content[0].(*llm.ThinkingContent)
	assert.Equal(t, "thinking ", firstThought.Thinking)
	assert.Equal(t, "true", firstThought.Metadata[googleThoughtMetadataKey])
	secondThought := response.Content[1].(*llm.ThinkingContent)
	assert.Equal(t, "summary", secondThought.Thinking)
	assert.Equal(t, base64.StdEncoding.EncodeToString(thoughtSignature),
		secondThought.Metadata[googleThoughtSignatureMetadataKey])
	answer := response.Content[2].(*llm.TextContent)
	assert.Equal(t, "answer", answer.Text)
	empty := response.Content[3].(*llm.TextContent)
	assert.Equal(t, "", empty.Text)
	assert.Equal(t, base64.StdEncoding.EncodeToString(textSignature),
		empty.Metadata[googleThoughtSignatureMetadataKey])

	contents, err := messagesToContents([]*llm.Message{response.Message().Copy()})
	assert.NoError(t, err)
	assert.Len(t, contents[0].Parts, 4)
	assert.Equal(t, thoughtSignature, contents[0].Parts[1].ThoughtSignature)
	assert.Equal(t, textSignature, contents[0].Parts[3].ThoughtSignature)
}

func TestStreamIteratorSeparatesSignedToUnsignedParts(t *testing.T) {
	tests := []struct {
		name    string
		thought bool
	}{
		{name: "thinking", thought: true},
		{name: "text", thought: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signature := []byte("signed-part")
			chunks := chunkSeq(
				&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
					Content: &genai.Content{Parts: []*genai.Part{{
						Text: "signed", Thought: tt.thought, ThoughtSignature: signature,
					}}},
				}}},
				&genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
					Content:      &genai.Content{Parts: []*genai.Part{{Text: "unsigned", Thought: tt.thought}}},
					FinishReason: genai.FinishReasonStop,
				}}},
			)

			iterator := NewStreamIteratorFromSeq(context.Background(), chunks, ModelGemini36Flash)
			t.Cleanup(func() { assert.NoError(t, iterator.Close()) })
			_, accumulator := collectStreamEvents(t, iterator)
			response := accumulator.Response()
			assert.Len(t, response.Content, 2)

			if tt.thought {
				first := response.Content[0].(*llm.ThinkingContent)
				second := response.Content[1].(*llm.ThinkingContent)
				assert.Equal(t, "signed", first.Thinking)
				assert.Equal(t, "unsigned", second.Thinking)
				assert.Equal(t, base64.StdEncoding.EncodeToString(signature),
					first.Metadata[googleThoughtSignatureMetadataKey])
				assert.Empty(t, second.Metadata[googleThoughtSignatureMetadataKey])
			} else {
				first := response.Content[0].(*llm.TextContent)
				second := response.Content[1].(*llm.TextContent)
				assert.Equal(t, "signed", first.Text)
				assert.Equal(t, "unsigned", second.Text)
				assert.Equal(t, base64.StdEncoding.EncodeToString(signature),
					first.Metadata[googleThoughtSignatureMetadataKey])
				assert.Empty(t, second.Metadata[googleThoughtSignatureMetadataKey])
			}
		})
	}
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

// TestStreamIteratorClosesDanglingBlocks verifies that a stream which simply
// ends — no FinishReason, no usage chunk — still closes the block it left
// open and terminates with message_delta then message_stop, so consumers see
// a balanced block lifecycle whatever kind of block was streaming last.
func TestStreamIteratorClosesDanglingBlocks(t *testing.T) {
	partChunk := func(part *genai.Part) *genai.GenerateContentResponse {
		return &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{{
				Content: &genai.Content{Role: "model", Parts: []*genai.Part{part}},
			}},
		}
	}
	tests := []struct {
		name    string
		chunk   *genai.GenerateContentResponse
		content llm.ContentType
	}{
		{name: "text", chunk: partChunk(&genai.Part{Text: "Hi"}), content: llm.ContentTypeText},
		{name: "thinking", chunk: partChunk(&genai.Part{Text: "hmm", Thought: true}), content: llm.ContentTypeThinking},
		{name: "function call", chunk: partChunk(&genai.Part{FunctionCall: &genai.FunctionCall{Name: "calc", Args: map[string]any{"a": 1}}}), content: llm.ContentTypeToolUse},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iterator := NewStreamIteratorFromSeq(context.Background(),
				chunkSeq(tc.chunk), "gemini-2.5-pro")
			defer iterator.Close()
			events, accumulator := collectStreamEvents(t, iterator)

			var types []llm.EventType
			starts, stops, stopIndex, deltaIndex := 0, 0, -1, -1
			for i, event := range events {
				types = append(types, event.Type)
				switch event.Type {
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
