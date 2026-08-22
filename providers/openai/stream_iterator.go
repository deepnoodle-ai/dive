package openai

import (
	"fmt"
	"io"
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/openai/openai-go/v3/responses"
)

// StreamSource is an interface that both the real OpenAI stream and mocks can implement
type StreamSource interface {
	Next() bool
	Current() responses.ResponseStreamEventUnion
	Err() error
	Close() error
}

type openaiStreamIterator struct {
	sdkStream    StreamSource
	config       *llm.Config
	err          error
	currentEvent *llm.Event
	eventQueue   []*llm.Event

	responseID    string
	responseModel string
	finalUsage    *llm.Usage

	// Accumulators and state for current item being processed
	// Keyed by OutputIndex (from OpenAI events)
	outputItemsState map[int]*outputItemState

	eventCount int
	closeOnce  sync.Once
	isClosed   bool
}

type outputItemState struct {
	OutputIndex int
	ItemID      string // ID of the output item (e.g., fc_xxxx, msg_xxxx)
	ItemType    string // E.g., "function_call", "message"
	IsComplete  bool

	// For function_call
	ToolCallName      string
	ToolCallID        string // The 'call_id' (e.g. call_xxxx)
	ToolArgumentsJson string

	// SummaryStreamedByIndex tracks which reasoning summary parts were streamed
	// incrementally (keyed by SummaryIndex), so the OutputItemDone handler
	// only emits fallback events for summary parts that were not already streamed.
	SummaryStreamedByIndex       map[int]bool
	ReasoningTextStreamedByIndex map[int]bool
	ReasoningStarted             bool
	ReasoningStopped             bool

	// For message with text/reasoning content parts
	// Keyed by ContentIndex
	ContentParts map[int]*contentPartState

	// Phase is the phase OpenAI assigned to a message item. It may be absent
	// when the item is added and only appear on the done event, so the done
	// event is treated as authoritative.
	Phase string
}

// hasTextPart reports whether this item streamed an output_text part, meaning
// the accumulator holds a text content block at this item's index.
func (s *outputItemState) hasTextPart() bool {
	for _, part := range s.ContentParts {
		if part.PartType == "output_text" {
			return true
		}
	}
	return false
}

type contentPartState struct {
	ContentIndex int
	PartID       string // ID of the content part, if available
	PartType     string // E.g., "output_text", "reasoning"
	Text         string // Accumulated text for output_text or reasoning
	IsComplete   bool
}

func newOpenAIStreamIterator(sdkStream StreamSource, config *llm.Config) *openaiStreamIterator {
	return &openaiStreamIterator{
		sdkStream:        sdkStream,
		config:           config,
		outputItemsState: make(map[int]*outputItemState),
		eventQueue:       make([]*llm.Event, 0),
	}
}

// Next advances to the next event in the stream. Returns false when the stream
// is complete or an error occurs.
func (s *openaiStreamIterator) Next() bool {
	// If we have events in the queue, use the first one
	if len(s.eventQueue) > 0 {
		s.currentEvent = s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return true
	}

	if s.isClosed {
		return false
	}

	// Try to get the next event from the SDK stream
	if !s.sdkStream.Next() {
		// Stream ended, check for error
		if err := s.sdkStream.Err(); err != nil {
			if err != io.EOF {
				s.err = err
			}
		}
		s.Close()

		// Process any final events in queue
		if len(s.eventQueue) > 0 {
			s.currentEvent = s.eventQueue[0]
			s.eventQueue = s.eventQueue[1:]
			return true
		}
		return false
	}

	// Process the OpenAI event
	rawEvent := s.sdkStream.Current()
	events, err := s.processOpenAIEvent(rawEvent)
	if err != nil {
		s.err = err
		s.Close()
		return false
	}

	// Add events to queue
	s.eventQueue = append(s.eventQueue, events...)

	// Return the first event if we have any
	if len(s.eventQueue) > 0 {
		s.currentEvent = s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return true
	}

	// Continue to next iteration to get more events
	return s.Next()
}

// Event returns the current event. Should only be called after a successful Next().
func (s *openaiStreamIterator) Event() *llm.Event {
	return s.currentEvent
}

// Err returns any error that occurred while reading from the stream.
func (s *openaiStreamIterator) Err() error {
	return s.err
}

// Close closes the stream and releases any associated resources.
func (s *openaiStreamIterator) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.isClosed = true
		if s.sdkStream != nil {
			err = s.sdkStream.Close()
		}
	})
	return err
}

// processOpenAIEvent converts OpenAI stream events to Dive events
func (s *openaiStreamIterator) processOpenAIEvent(event responses.ResponseStreamEventUnion) ([]*llm.Event, error) {
	var diveEvents []*llm.Event

	switch data := event.AsAny().(type) {
	case responses.ResponseCreatedEvent:
		s.responseID = data.Response.ID
		s.responseModel = string(data.Response.Model)
		s.eventCount++
		diveEvents = append(diveEvents, &llm.Event{
			Type: llm.EventTypeMessageStart,
			Message: &llm.Response{
				ID:    s.responseID,
				Type:  "message",
				Role:  llm.Assistant,
				Model: s.responseModel,
			},
		})

	case responses.ResponseOutputItemAddedEvent:
		outputIdx := int(data.OutputIndex)
		s.outputItemsState[outputIdx] = &outputItemState{
			OutputIndex:  outputIdx,
			ItemID:       data.Item.ID,
			ItemType:     data.Item.Type,
			ContentParts: make(map[int]*contentPartState),
		}

		if data.Item.Type == "message" {
			s.outputItemsState[outputIdx].Phase = string(data.Item.Phase)
		}

		if data.Item.Type == "function_call" {
			fnCall := data.Item.AsFunctionCall()
			s.outputItemsState[outputIdx].ToolCallName = fnCall.Name
			s.outputItemsState[outputIdx].ToolCallID = fnCall.CallID

			diveEvents = append(diveEvents, &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &outputIdx,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeToolUse,
					ID:   fnCall.CallID,
					Name: fnCall.Name,
				},
			})
		}

	case responses.ResponseContentPartAddedEvent:
		outputIdx := int(data.OutputIndex)
		contentIdx := int(data.ContentIndex)
		itemState, ok := s.outputItemsState[outputIdx]
		if !ok {
			return nil, fmt.Errorf("received content part for unknown output index %d", outputIdx)
		}

		partState := &contentPartState{
			ContentIndex: contentIdx,
			PartType:     data.Part.Type,
		}
		itemState.ContentParts[contentIdx] = partState

		var diveContentType llm.ContentType
		switch data.Part.Type {
		case "output_text":
			diveContentType = llm.ContentTypeText
		case "reasoning":
			diveContentType = llm.ContentTypeThinking
		default:
			// Skip unknown part types for now
			return diveEvents, nil
		}

		contentBlock := &llm.EventContentBlock{Type: diveContentType}
		if diveContentType == llm.ContentTypeText {
			// Surface the phase to live consumers as soon as it is known. When
			// OpenAI only labels the message on the done event, the metadata
			// delta emitted there fills it in.
			contentBlock.Metadata = openAIPhaseMetadata(itemState.Phase)
		}
		diveEvents = append(diveEvents, &llm.Event{
			Type:         llm.EventTypeContentBlockStart,
			Index:        &outputIdx,
			ContentBlock: contentBlock,
		})

	case responses.ResponseTextDeltaEvent:
		outputIdx := int(data.OutputIndex)
		contentIdx := int(data.ContentIndex)
		itemState := s.outputItemsState[outputIdx]
		if itemState == nil || itemState.ContentParts[contentIdx] == nil {
			return nil, fmt.Errorf("received text delta for unknown output/content index %d/%d", outputIdx, contentIdx)
		}
		itemState.ContentParts[contentIdx].Text += data.Delta

		diveEvents = append(diveEvents, &llm.Event{
			Type:  llm.EventTypeContentBlockDelta,
			Index: &outputIdx,
			Delta: &llm.EventDelta{
				Type: llm.EventDeltaTypeText,
				Text: data.Delta,
			},
		})

	case responses.ResponseFunctionCallArgumentsDeltaEvent:
		outputIdx := int(data.OutputIndex)
		itemState, ok := s.outputItemsState[outputIdx]
		if !ok || itemState.ItemType != "function_call" {
			return nil, fmt.Errorf("received function call arguments delta for non-function-call item at index %d", outputIdx)
		}
		itemState.ToolArgumentsJson += data.Delta

		diveEvents = append(diveEvents, &llm.Event{
			Type:  llm.EventTypeContentBlockDelta,
			Index: &outputIdx,
			Delta: &llm.EventDelta{
				Type:        llm.EventDeltaTypeInputJSON,
				PartialJSON: data.Delta,
			},
		})

	case responses.ResponseReasoningSummaryPartAddedEvent:
		// This indicates the start of reasoning summary content.
		if data.Part.Type == "summary_text" {
			outputIdx := int(data.OutputIndex)
			summaryIdx := int(data.SummaryIndex)

			// Mark the existing reasoning item state so OutputItemDone skips duplicate emission.
			itemState, ok := s.outputItemsState[outputIdx]
			if ok {
				if itemState.SummaryStreamedByIndex == nil {
					itemState.SummaryStreamedByIndex = make(map[int]bool)
				}
				itemState.SummaryStreamedByIndex[summaryIdx] = true
			} else {
				// Create state if it doesn't exist yet.
				itemState = &outputItemState{
					OutputIndex: outputIdx,
					ItemID:      data.ItemID,
					ItemType:    "reasoning",
					SummaryStreamedByIndex: map[int]bool{
						summaryIdx: true,
					},
					ContentParts: make(map[int]*contentPartState),
				}
				s.outputItemsState[outputIdx] = itemState
			}
			itemState.ContentParts[summaryIdx] = &contentPartState{
				ContentIndex: summaryIdx,
				PartType:     "thinking",
				Text:         data.Part.Text,
			}

			if !itemState.ReasoningStarted {
				itemState.ReasoningStarted = true
				diveEvents = append(diveEvents, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &outputIdx,
					ContentBlock: &llm.EventContentBlock{
						Type:     llm.ContentTypeThinking,
						ID:       data.ItemID,
						Thinking: data.Part.Text,
					},
				})
			} else if data.Part.Text != "" {
				diveEvents = append(diveEvents, thinkingDeltaEvent(outputIdx, summarySeparator(summaryIdx)+data.Part.Text))
			} else if summaryIdx > 0 {
				diveEvents = append(diveEvents, thinkingDeltaEvent(outputIdx, summarySeparator(summaryIdx)))
			}
		}

	case responses.ResponseReasoningSummaryTextDeltaEvent:
		outputIdx := int(data.OutputIndex)
		summaryIdx := int(data.SummaryIndex)

		// Get or create the state for reasoning summary.
		itemState, exists := s.outputItemsState[outputIdx]
		if !exists {
			itemState = &outputItemState{
				OutputIndex: outputIdx,
				ItemID:      data.ItemID,
				ItemType:    "reasoning",
				SummaryStreamedByIndex: map[int]bool{
					summaryIdx: true,
				},
				ContentParts: map[int]*contentPartState{
					summaryIdx: {ContentIndex: summaryIdx, PartType: "thinking"},
				},
			}
			s.outputItemsState[outputIdx] = itemState
		} else {
			if itemState.SummaryStreamedByIndex == nil {
				itemState.SummaryStreamedByIndex = make(map[int]bool)
			}
			itemState.SummaryStreamedByIndex[summaryIdx] = true
		}
		if itemState.ContentParts[summaryIdx] == nil {
			itemState.ContentParts[summaryIdx] = &contentPartState{
				ContentIndex: summaryIdx,
				PartType:     "thinking",
			}
		}

		if !itemState.ReasoningStarted {
			itemState.ReasoningStarted = true
			idxStart := outputIdx
			diveEvents = append(diveEvents, &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &idxStart,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeThinking,
					ID:   data.ItemID,
				},
			})
		}

		// Accumulate the text.
		if partState, ok := itemState.ContentParts[summaryIdx]; ok {
			partState.Text += data.Delta
		}

		diveEvents = append(diveEvents, thinkingDeltaEvent(outputIdx, data.Delta))

	case responses.ResponseReasoningSummaryPartDoneEvent:
		outputIdx := int(data.OutputIndex)
		summaryIdx := int(data.SummaryIndex)
		itemState := s.outputItemsState[outputIdx]
		if itemState == nil {
			itemState = &outputItemState{
				OutputIndex:            outputIdx,
				ItemID:                 data.ItemID,
				ItemType:               "reasoning",
				SummaryStreamedByIndex: make(map[int]bool),
				ContentParts:           make(map[int]*contentPartState),
			}
			s.outputItemsState[outputIdx] = itemState
		}
		if itemState.SummaryStreamedByIndex == nil {
			itemState.SummaryStreamedByIndex = make(map[int]bool)
		}
		if !itemState.SummaryStreamedByIndex[summaryIdx] {
			if !itemState.ReasoningStarted {
				itemState.ReasoningStarted = true
				diveEvents = append(diveEvents, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &outputIdx,
					ContentBlock: &llm.EventContentBlock{
						Type: llm.ContentTypeThinking,
						ID:   data.ItemID,
					},
				})
			}
			diveEvents = append(diveEvents,
				thinkingDeltaEvent(outputIdx, summarySeparator(summaryIdx)+data.Part.Text))
		}
		itemState.SummaryStreamedByIndex[summaryIdx] = true
		if partState, ok := itemState.ContentParts[summaryIdx]; ok {
			partState.IsComplete = true
			partState.Text = data.Part.Text
		}

	case responses.ResponseReasoningTextDeltaEvent:
		outputIdx := int(data.OutputIndex)
		contentIdx := int(data.ContentIndex)
		itemState := s.outputItemsState[outputIdx]
		if itemState == nil {
			itemState = &outputItemState{
				OutputIndex:                  outputIdx,
				ItemID:                       data.ItemID,
				ItemType:                     "reasoning",
				SummaryStreamedByIndex:       make(map[int]bool),
				ReasoningTextStreamedByIndex: make(map[int]bool),
				ContentParts:                 make(map[int]*contentPartState),
			}
			s.outputItemsState[outputIdx] = itemState
		}
		if itemState.ReasoningTextStreamedByIndex == nil {
			itemState.ReasoningTextStreamedByIndex = make(map[int]bool)
		}
		firstReasoningText := len(itemState.ReasoningTextStreamedByIndex) == 0
		itemState.ReasoningTextStreamedByIndex[contentIdx] = true
		if !itemState.ReasoningStarted {
			itemState.ReasoningStarted = true
			diveEvents = append(diveEvents, &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &outputIdx,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeThinking,
					ID:   data.ItemID,
				},
			})
		} else if firstReasoningText && len(itemState.SummaryStreamedByIndex) > 0 {
			diveEvents = append(diveEvents, thinkingDeltaEvent(outputIdx, "\n\n"))
		}
		diveEvents = append(diveEvents, thinkingDeltaEvent(outputIdx, data.Delta))

	case responses.ResponseReasoningTextDoneEvent:
		outputIdx := int(data.OutputIndex)
		contentIdx := int(data.ContentIndex)
		itemState := s.outputItemsState[outputIdx]
		if itemState == nil {
			itemState = &outputItemState{
				OutputIndex:                  outputIdx,
				ItemID:                       data.ItemID,
				ItemType:                     "reasoning",
				SummaryStreamedByIndex:       make(map[int]bool),
				ReasoningTextStreamedByIndex: make(map[int]bool),
				ContentParts:                 make(map[int]*contentPartState),
			}
			s.outputItemsState[outputIdx] = itemState
		}
		if itemState.ReasoningTextStreamedByIndex == nil {
			itemState.ReasoningTextStreamedByIndex = make(map[int]bool)
		}
		if !itemState.ReasoningTextStreamedByIndex[contentIdx] {
			if !itemState.ReasoningStarted {
				itemState.ReasoningStarted = true
				diveEvents = append(diveEvents, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &outputIdx,
					ContentBlock: &llm.EventContentBlock{
						Type: llm.ContentTypeThinking,
						ID:   data.ItemID,
					},
				})
			}
			separator := ""
			if contentIdx > 0 || len(itemState.SummaryStreamedByIndex) > 0 {
				separator = "\n\n"
			}
			diveEvents = append(diveEvents, thinkingDeltaEvent(outputIdx, separator+data.Text))
		}
		itemState.ReasoningTextStreamedByIndex[contentIdx] = true

	case responses.ResponseTextDoneEvent:
		outputIdx := int(data.OutputIndex)
		contentIdx := int(data.ContentIndex)
		if item, ok := s.outputItemsState[outputIdx]; ok {
			if part, ok2 := item.ContentParts[contentIdx]; ok2 {
				part.Text = data.Text
				part.IsComplete = true
				diveEvents = append(diveEvents, &llm.Event{
					Type:  llm.EventTypeContentBlockStop,
					Index: &outputIdx,
				})
			}
		}

	case responses.ResponseFunctionCallArgumentsDoneEvent:
		outputIdx := int(data.OutputIndex)
		if item, ok := s.outputItemsState[outputIdx]; ok && item.ItemType == "function_call" {
			item.ToolArgumentsJson = data.Arguments
		}

	case responses.ResponseOutputItemDoneEvent:
		outputIdx := int(data.OutputIndex)

		// Handle reasoning items that arrive complete (without incremental summary events).
		// Only emit fallback events for summary parts that were NOT already streamed.
		if data.Item.Type == "reasoning" {
			itemState := s.outputItemsState[outputIdx]
			reasoning := data.Item.AsReasoning()
			if itemState == nil {
				itemState = &outputItemState{
					OutputIndex:                  outputIdx,
					ItemID:                       reasoning.ID,
					ItemType:                     "reasoning",
					SummaryStreamedByIndex:       make(map[int]bool),
					ReasoningTextStreamedByIndex: make(map[int]bool),
					ContentParts:                 make(map[int]*contentPartState),
				}
				s.outputItemsState[outputIdx] = itemState
			}
			if !itemState.ReasoningStarted &&
				(len(reasoning.Summary) > 0 || len(reasoning.Content) > 0 || reasoning.EncryptedContent != "") {
				itemState.ReasoningStarted = true
				diveEvents = append(diveEvents, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &outputIdx,
					ContentBlock: &llm.EventContentBlock{
						Type: llm.ContentTypeThinking,
						ID:   reasoning.ID,
					},
				})
			}
			preferReasoningText := len(reasoning.Content) > 0 && len(itemState.SummaryStreamedByIndex) == 0
			for i, part := range reasoning.Summary {
				if preferReasoningText {
					break
				}
				if itemState.SummaryStreamedByIndex[i] {
					continue // already streamed incrementally
				}
				diveEvents = append(diveEvents, thinkingDeltaEvent(outputIdx, summarySeparator(i)+part.Text))
			}
			for i, part := range reasoning.Content {
				if itemState.ReasoningTextStreamedByIndex[i] {
					continue
				}
				separator := ""
				if i > 0 || len(itemState.SummaryStreamedByIndex) > 0 {
					separator = "\n\n"
				}
				diveEvents = append(diveEvents, thinkingDeltaEvent(outputIdx, separator+part.Text))
			}
			metadata, err := openAIReasoningMetadata(
				reasoningSummaryTexts(reasoning),
				reasoningContentTexts(reasoning),
			)
			if err != nil {
				return nil, err
			}
			if len(metadata) > 0 {
				diveEvents = append(diveEvents, &llm.Event{
					Type:  llm.EventTypeContentBlockDelta,
					Index: &outputIdx,
					Delta: &llm.EventDelta{
						Type:     llm.EventDeltaTypeMetadata,
						Metadata: metadata,
					},
				})
			}
			if reasoning.EncryptedContent != "" {
				idxSignature := outputIdx
				diveEvents = append(diveEvents, &llm.Event{
					Type:  llm.EventTypeContentBlockDelta,
					Index: &idxSignature,
					Delta: &llm.EventDelta{
						Type:      llm.EventDeltaTypeSignature,
						Signature: reasoning.EncryptedContent,
					},
				})
			}
			if itemState.ReasoningStarted && !itemState.ReasoningStopped {
				itemState.ReasoningStopped = true
				idxStop := outputIdx
				diveEvents = append(diveEvents, &llm.Event{
					Type:  llm.EventTypeContentBlockStop,
					Index: &idxStop,
				})
			}
		}

		// The done event carries the authoritative phase for a message item:
		// OpenAI may omit it when the item is added and only label it here.
		if data.Item.Type == "message" {
			if itemState, ok := s.outputItemsState[outputIdx]; ok {
				if phase := string(data.Item.Phase); phase != "" {
					itemState.Phase = phase
				}
				metadata := openAIPhaseMetadata(itemState.Phase)
				if metadata != nil && itemState.hasTextPart() {
					idxPhase := outputIdx
					diveEvents = append(diveEvents, &llm.Event{
						Type:  llm.EventTypeContentBlockDelta,
						Index: &idxPhase,
						Delta: &llm.EventDelta{
							Type:     llm.EventDeltaTypeMetadata,
							Metadata: metadata,
						},
					})
				}
			}
		}

		if item, ok := s.outputItemsState[outputIdx]; ok && !item.IsComplete {
			item.IsComplete = true
			if item.ItemType == "function_call" {
				idxFnStop := outputIdx
				diveEvents = append(diveEvents, &llm.Event{
					Type:  llm.EventTypeContentBlockStop,
					Index: &idxFnStop,
				})
			}
		}

	case responses.ResponseCompletedEvent:
		inputTokens, cacheReadInputTokens, cacheCreationInputTokens := normalizeInputTokens(
			data.Response.Usage.InputTokens,
			data.Response.Usage.InputTokensDetails.CachedTokens,
			data.Response.Usage.InputTokensDetails.CacheWriteTokens,
		)
		s.finalUsage = &llm.Usage{
			InputTokens:              inputTokens,
			OutputTokens:             int(data.Response.Usage.OutputTokens),
			CacheReadInputTokens:     cacheReadInputTokens,
			CacheCreationInputTokens: cacheCreationInputTokens,
			ReasoningTokens:          int(data.Response.Usage.OutputTokensDetails.ReasoningTokens),
		}
		stopReason := determineStopReason(&data.Response)

		diveEvents = append(diveEvents, &llm.Event{
			Type: llm.EventTypeMessageDelta,
			Delta: &llm.EventDelta{
				StopReason: stopReason,
			},
			Usage: s.finalUsage,
		})
		diveEvents = append(diveEvents, &llm.Event{Type: llm.EventTypeMessageStop})
		s.isClosed = true

	case responses.ResponseFailedEvent:
		s.err = fmt.Errorf("stream failed: code=%s, message=%s", data.Response.Error.Code, data.Response.Error.Message)
		s.isClosed = true

	case responses.ResponseIncompleteEvent:
		inputTokens, cacheReadInputTokens, cacheCreationInputTokens := normalizeInputTokens(
			data.Response.Usage.InputTokens,
			data.Response.Usage.InputTokensDetails.CachedTokens,
			data.Response.Usage.InputTokensDetails.CacheWriteTokens,
		)
		s.finalUsage = &llm.Usage{
			InputTokens:              inputTokens,
			OutputTokens:             int(data.Response.Usage.OutputTokens),
			CacheReadInputTokens:     cacheReadInputTokens,
			CacheCreationInputTokens: cacheCreationInputTokens,
			ReasoningTokens:          int(data.Response.Usage.OutputTokensDetails.ReasoningTokens),
		}
		stopReason := determineStopReason(&data.Response)

		diveEvents = append(diveEvents, &llm.Event{
			Type: llm.EventTypeMessageDelta,
			Delta: &llm.EventDelta{
				StopReason: stopReason,
			},
			Usage: s.finalUsage,
		})
		diveEvents = append(diveEvents, &llm.Event{Type: llm.EventTypeMessageStop})
		s.isClosed = true

	case responses.ResponseErrorEvent:
		s.err = fmt.Errorf("stream error event: code=%s, message=%s, param=%s", data.Code, data.Message, data.Param)
		s.isClosed = true

	default:
		// Log unhandled event types if necessary
		// For now, just ignore unhandled events
	}

	return diveEvents, nil
}

func thinkingDeltaEvent(outputIdx int, thinking string) *llm.Event {
	return &llm.Event{
		Type:  llm.EventTypeContentBlockDelta,
		Index: &outputIdx,
		Delta: &llm.EventDelta{
			Type:     llm.EventDeltaTypeThinking,
			Thinking: thinking,
		},
	}
}

func summarySeparator(summaryIdx int) string {
	if summaryIdx > 0 {
		return "\n\n"
	}
	return ""
}

func reasoningSummaryTexts(reasoning responses.ResponseReasoningItem) []string {
	texts := make([]string, 0, len(reasoning.Summary))
	for _, part := range reasoning.Summary {
		texts = append(texts, part.Text)
	}
	return texts
}

func reasoningContentTexts(reasoning responses.ResponseReasoningItem) []string {
	texts := make([]string, 0, len(reasoning.Content))
	for _, part := range reasoning.Content {
		texts = append(texts, part.Text)
	}
	return texts
}
