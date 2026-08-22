package openaicompletions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
)

type StreamIterator struct {
	reader            *bufio.Reader
	body              io.ReadCloser
	err               error
	currentEvent      *llm.Event
	toolCalls         map[int]*ToolCallAccumulator
	contentBlocks     map[int]*ContentBlockAccumulator
	responseID        string
	responseModel     string
	usage             Usage
	prefill           string
	prefillClosingTag string
	eventCount        int
	nextBlockIndex    int
	closeOnce         sync.Once
	eventQueue        []*llm.Event
	// finalEvents holds the message_delta and message_stop events generated
	// when a finish_reason is seen. They are deferred until the trailing
	// usage chunk (stream_options.include_usage) has been consumed, so the
	// message_delta carries the real token usage.
	finalEvents []*llm.Event
	// terminated records that the terminal message_delta and message_stop
	// events have been returned, whether built from a finish_reason or
	// synthesized at [DONE]/EOF, so a stream that signals its end more than
	// once (finish_reason, then [DONE], then EOF) terminates exactly once.
	terminated bool
	// thinkingIndex and textIndex track the sequential content block index
	// assigned to each block type, or -1 if not yet started.
	thinkingIndex              int
	textIndex                  int
	reportedCostCurrency       string
	providerName               string
	openRouterReasoningDetails []json.RawMessage
	// toolCallIndices maps OpenAI tool call indices to sequential block indices.
	toolCallIndices map[int]int
}

type ToolCallAccumulator struct {
	ID         string
	Type       string
	Name       string
	Arguments  string
	IsComplete bool
}

type ContentBlockAccumulator struct {
	Type       string
	Text       string
	IsComplete bool
}

// Next advances to the next event in the stream. Returns false when the stream
// is complete or an error occurs.
func (s *StreamIterator) Next() bool {
	// If we have events in the queue, use the first one
	if len(s.eventQueue) > 0 {
		s.currentEvent = s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return true
	}

	// Try to get more events
	for {
		events, err := s.next()
		if err != nil {
			if err != io.EOF {
				// EOF is expected when stream ends
				s.Close()
				s.err = err
			}
			return false
		}

		// If we got events, use the first one and queue the rest
		if len(events) > 0 {
			s.currentEvent = events[0]
			if len(events) > 1 {
				s.eventQueue = append(s.eventQueue, events[1:]...)
			}
			return true
		}
	}
}

// Event returns the current event. Should only be called after a successful Next().
func (s *StreamIterator) Event() *llm.Event {
	return s.currentEvent
}

// next processes a single line from the stream and returns events if any are ready
func (s *StreamIterator) next() ([]*llm.Event, error) {
	line, err := s.reader.ReadBytes('\n')
	if err != nil {
		// If the stream ends before a trailing usage chunk or [DONE] marker
		// arrives, terminate the message here so it still ends with
		// content_block_stop, message_delta, and message_stop.
		if err == io.EOF {
			if events := s.endStream(); len(events) > 0 {
				return events, nil
			}
		}
		return nil, err
	}
	// Skip empty lines
	if len(bytes.TrimSpace(line)) == 0 {
		return nil, nil
	}
	// Skip SSE comment lines. Per the SSE spec any line beginning with ':'
	// is a comment and must be ignored. OpenRouter emits ": OPENROUTER
	// PROCESSING" keep-alive comments while a model is queued/warming up.
	if bytes.HasPrefix(bytes.TrimSpace(line), []byte(":")) {
		return nil, nil
	}
	// Parse the event type from the SSE format
	if bytes.HasPrefix(line, []byte("event: ")) {
		return nil, nil
	}
	// Remove "data: " prefix if present
	line = bytes.TrimPrefix(line, []byte("data: "))
	// Check for stream end
	if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
		return s.endStream(), nil
	}
	// Unmarshal the event
	var event StreamResponse
	if err := json.Unmarshal(line, &event); err != nil {
		return nil, err
	}
	if event.ID != "" {
		s.responseID = event.ID
	}
	if event.Model != "" {
		s.responseModel = event.Model
	}
	if event.Usage.present || event.Usage.TotalTokens > 0 || event.Usage.Cost != nil {
		s.usage = event.Usage
	}
	if len(event.Choices) == 0 {
		// With stream_options.include_usage set, the API sends a final chunk
		// with empty choices that carries the usage, after the finish_reason
		// chunk. Now that usage is known, flush the deferred final events.
		return s.flushFinalEvents(), nil
	}
	choice := event.Choices[0]
	var events []*llm.Event
	var processErr error

	// Emit message start event if this is the first event
	if s.eventCount == 0 {
		s.eventCount++
		events = append(events, &llm.Event{
			Type: llm.EventTypeMessageStart,
			Message: &llm.Response{
				ID:      s.responseID,
				Type:    "message",
				Role:    llm.Assistant,
				Model:   s.responseModel,
				Content: []llm.Content{},
				Usage:   s.usage.toLLMUsage(),
			},
		})
	}

	if choice.Delta.Reasoning != "" {
		metadata := llm.ProviderMetadata(nil)
		if s.providerName == "openrouter" {
			metadata = llm.ProviderMetadata{openRouterReasoningMetadataKey: "true"}
		}
		events = s.appendThinkingEvents(events, choice.Delta.Reasoning, metadata)
	}
	if s.providerName == "openrouter" && len(bytes.TrimSpace(choice.Delta.ReasoningDetails)) > 0 &&
		!bytes.Equal(bytes.TrimSpace(choice.Delta.ReasoningDetails), []byte("null")) {
		events, processErr = s.appendOpenRouterReasoningDetails(
			events,
			choice.Delta.ReasoningDetails,
			choice.Delta.Reasoning == "",
		)
		if processErr != nil {
			return nil, processErr
		}
	}

	// Mistral emits array-shaped content deltas while thinking. Preserve each
	// ThinkChunk as thinking and each TextChunk as answer text.
	for _, part := range choice.Delta.ContentParts {
		switch part.Type {
		case "thinking":
			thinking := joinThinkingParts(part.Thinking)
			if thinking != "" {
				events = s.appendThinkingEvents(events, thinking,
					llm.ProviderMetadata{mistralThinkingMetadataKey: "true"})
			}
		case "text":
			events, processErr = s.flushOpenRouterReasoningDetails(events)
			if processErr != nil {
				return nil, processErr
			}
			events = s.appendTextEvents(events, part.Text)
		}
	}

	// Handle ordinary string-shaped text content.
	if choice.Delta.Content != "" {
		events, processErr = s.flushOpenRouterReasoningDetails(events)
		if processErr != nil {
			return nil, processErr
		}
		events = s.appendTextEvents(events, choice.Delta.Content)
	}

	if len(choice.Delta.ToolCalls) > 0 {
		events, processErr = s.flushOpenRouterReasoningDetails(events)
		if processErr != nil {
			return nil, processErr
		}
		for _, toolCallDelta := range choice.Delta.ToolCalls {
			// Map OpenAI tool call index to a sequential display index
			index, known := s.toolCallIndices[toolCallDelta.Index]
			if !known {
				// Stop any previous content blocks and tool calls still open
				events = s.closeOpenBlocks(events)
				index = s.nextBlockIndex
				s.nextBlockIndex++
				s.toolCallIndices[toolCallDelta.Index] = index
				s.toolCalls[index] = &ToolCallAccumulator{Type: "function"}
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &index,
					ContentBlock: &llm.EventContentBlock{
						ID:   toolCallDelta.ID,
						Name: toolCallDelta.Function.Name,
						Type: llm.ContentTypeToolUse,
					},
				})
			}
			toolCall := s.toolCalls[index]
			if toolCallDelta.ID != "" {
				toolCall.ID = toolCallDelta.ID
				// Update the ContentBlock in the event queue if it exists
				for _, queuedEvent := range s.eventQueue {
					if queuedEvent.Type == llm.EventTypeContentBlockStart && queuedEvent.Index != nil && *queuedEvent.Index == index {
						if queuedEvent.ContentBlock == nil {
							queuedEvent.ContentBlock = &llm.EventContentBlock{Type: "tool_use"}
						}
						queuedEvent.ContentBlock.ID = toolCallDelta.ID
					}
				}
			}
			if toolCallDelta.Type != "" {
				toolCall.Type = toolCallDelta.Type
			}
			if toolCallDelta.Function.Name != "" {
				toolCall.Name = toolCallDelta.Function.Name
				// Update the ContentBlock in the event queue if it exists
				for _, queuedEvent := range s.eventQueue {
					if queuedEvent.Type == llm.EventTypeContentBlockStart && queuedEvent.Index != nil && *queuedEvent.Index == index {
						if queuedEvent.ContentBlock == nil {
							queuedEvent.ContentBlock = &llm.EventContentBlock{Type: "tool_use"}
						}
						queuedEvent.ContentBlock.Name = toolCallDelta.Function.Name
					}
				}
			}
			if toolCallDelta.Function.Arguments != "" {
				toolCall.Arguments += toolCallDelta.Function.Arguments
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockDelta,
					Index: &index,
					Delta: &llm.EventDelta{
						Type:        llm.EventDeltaTypeInputJSON,
						PartialJSON: toolCallDelta.Function.Arguments,
					},
				})
			}
		}
	}

	if choice.FinishReason != "" {
		events, processErr = s.flushOpenRouterReasoningDetails(events)
		if processErr != nil {
			return nil, processErr
		}
		events = s.closeOpenBlocks(events)
		// Build the message_delta event with the stop reason, but defer it
		// (along with message_stop) until the trailing usage chunk, [DONE]
		// marker, or EOF, so the message_delta carries the real token usage.
		stopReason := choice.FinishReason
		if stopReason == "tool_calls" {
			stopReason = "tool_use" // Match Anthropic
		}
		s.finalEvents = []*llm.Event{
			{
				Type:  llm.EventTypeMessageDelta,
				Delta: &llm.EventDelta{StopReason: stopReason},
				Usage: &llm.Usage{},
			},
			{
				Type: llm.EventTypeMessageStop,
			},
		}
	}

	return events, nil
}

func (s *StreamIterator) appendThinkingEvents(events []*llm.Event, text string, metadata llm.ProviderMetadata) []*llm.Event {
	if text == "" {
		return events
	}
	events = s.ensureThinkingBlock(events, metadata)
	index := s.thinkingIndex
	return append(events, &llm.Event{
		Type:  llm.EventTypeContentBlockDelta,
		Index: &index,
		Delta: &llm.EventDelta{
			Type:     llm.EventDeltaTypeThinking,
			Thinking: text,
		},
	})
}

func (s *StreamIterator) ensureThinkingBlock(events []*llm.Event, metadata llm.ProviderMetadata) []*llm.Event {
	if s.thinkingIndex < 0 {
		s.thinkingIndex = s.nextBlockIndex
		s.nextBlockIndex++
		s.contentBlocks[s.thinkingIndex] = &ContentBlockAccumulator{Type: "thinking"}
		index := s.thinkingIndex
		events = append(events, &llm.Event{
			Type:  llm.EventTypeContentBlockStart,
			Index: &index,
			ContentBlock: &llm.EventContentBlock{
				Type:     llm.ContentTypeThinking,
				Metadata: metadata.Clone(),
			},
		})
	}
	return events
}

func (s *StreamIterator) appendOpenRouterReasoningDetails(
	events []*llm.Event,
	raw json.RawMessage,
	includeVisibleText bool,
) ([]*llm.Event, error) {
	var details []json.RawMessage
	if err := json.Unmarshal(raw, &details); err != nil {
		return nil, err
	}
	if len(details) == 0 {
		return events, nil
	}
	for _, detail := range details {
		s.openRouterReasoningDetails = append(
			s.openRouterReasoningDetails,
			append(json.RawMessage(nil), detail...),
		)
	}
	events = s.ensureThinkingBlock(events, nil)
	if includeVisibleText {
		if text := reasoningDetailsText(raw); text != "" {
			events = s.appendThinkingEvents(events, text, nil)
		}
	}
	return events, nil
}

func (s *StreamIterator) flushOpenRouterReasoningDetails(events []*llm.Event) ([]*llm.Event, error) {
	if len(s.openRouterReasoningDetails) == 0 || s.thinkingIndex < 0 {
		return events, nil
	}
	fullDetails, err := json.Marshal(s.openRouterReasoningDetails)
	if err != nil {
		return nil, err
	}
	s.openRouterReasoningDetails = nil
	index := s.thinkingIndex
	return append(events, &llm.Event{
		Type:  llm.EventTypeContentBlockDelta,
		Index: &index,
		Delta: &llm.EventDelta{
			Type: llm.EventDeltaTypeMetadata,
			Metadata: llm.ProviderMetadata{
				openRouterReasoningDetailsMetadataKey: string(fullDetails),
			},
		},
	}), nil
}

func (s *StreamIterator) appendTextEvents(events []*llm.Event, text string) []*llm.Event {
	if text == "" {
		return events
	}
	// Apply and clear prefill if there is one.
	if s.prefill != "" {
		if !strings.HasPrefix(text, s.prefill) && !strings.HasPrefix(s.prefill, text) {
			text = s.prefill + text
		}
		s.prefill = ""
	}
	// If this is a new text block, stop any open blocks and start it.
	if s.textIndex < 0 {
		events = s.closeOpenBlocks(events)
		s.textIndex = s.nextBlockIndex
		s.nextBlockIndex++
		s.contentBlocks[s.textIndex] = &ContentBlockAccumulator{Type: "text"}
		events = append(events, &llm.Event{
			Type:         llm.EventTypeContentBlockStart,
			Index:        &s.textIndex,
			ContentBlock: &llm.EventContentBlock{Type: llm.ContentTypeText},
		})
	}
	return append(events, &llm.Event{
		Type:  llm.EventTypeContentBlockDelta,
		Index: &s.textIndex,
		Delta: &llm.EventDelta{
			Type: llm.EventDeltaTypeText,
			Text: text,
		},
	})
}

// closeOpenBlocks appends a content_block_stop for every content block and
// tool call still open, in block-index order, and marks each complete so it
// closes exactly once.
func (s *StreamIterator) closeOpenBlocks(events []*llm.Event) []*llm.Event {
	indexes := make([]int, 0, len(s.contentBlocks)+len(s.toolCalls))
	for index, block := range s.contentBlocks {
		if !block.IsComplete {
			block.IsComplete = true
			indexes = append(indexes, index)
		}
	}
	for index, toolCall := range s.toolCalls {
		if !toolCall.IsComplete {
			toolCall.IsComplete = true
			indexes = append(indexes, index)
		}
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		stopIndex := index
		events = append(events, &llm.Event{
			Type:  llm.EventTypeContentBlockStop,
			Index: &stopIndex,
		})
	}
	return events
}

// flushFinalEvents returns the deferred message_delta and message_stop events,
// stamping the message_delta with the most recent usage. Returns nil if there
// are no deferred events (or they were already flushed).
func (s *StreamIterator) flushFinalEvents() []*llm.Event {
	if s.terminated || len(s.finalEvents) == 0 {
		return nil
	}
	s.terminated = true
	events := s.finalEvents
	s.finalEvents = nil
	for _, event := range events {
		if event.Type == llm.EventTypeMessageDelta && event.Usage != nil {
			*event.Usage = s.llmUsage()
		}
	}
	return events
}

// endStream returns the terminal events for a stream that has signaled its end
// with [DONE] or EOF. Final events deferred by a finish_reason are flushed with
// the latest usage. When no finish_reason ever arrived, every block still open
// is closed and a message_delta (carrying usage, with no stop reason) and
// message_stop are synthesized, so consumers always receive a balanced block
// lifecycle in the content_block_stop → message_delta → message_stop order the
// other providers emit. Returns nil once the stream has terminated, or when no
// message was ever started.
func (s *StreamIterator) endStream() []*llm.Event {
	if events := s.flushFinalEvents(); len(events) > 0 {
		return events
	}
	if s.terminated || s.eventCount == 0 {
		return nil
	}
	s.terminated = true
	events := s.closeOpenBlocks(nil)
	usage := s.llmUsage()
	return append(events,
		&llm.Event{
			Type:  llm.EventTypeMessageDelta,
			Delta: &llm.EventDelta{},
			Usage: &usage,
		},
		&llm.Event{Type: llm.EventTypeMessageStop},
	)
}

func (s *StreamIterator) llmUsage() llm.Usage {
	usage := s.usage.toLLMUsage()
	applyReportedUsageCost(s.usage, &usage, s.responseModel, s.reportedCostCurrency)
	if s.reportedCostCurrency != "" && !s.usage.present {
		usage.Cost = nil
		usage.CostEstimateUnavailable = true
	} else if s.reportedCostCurrency != "" && usage.Cost == nil {
		llm.PopulateCost(s.responseModel, false, &usage)
		if usage.Cost == nil {
			usage.CostEstimateUnavailable = true
		}
	}
	return usage
}

func (s *StreamIterator) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.body.Close() })
	return err
}

func (s *StreamIterator) Err() error {
	return s.err
}
