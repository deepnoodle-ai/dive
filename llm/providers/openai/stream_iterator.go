package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/diveagents/dive/llm"
	"github.com/openai/openai-go/responses"
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

	// For message with text/reasoning content parts
	// Keyed by ContentIndex
	ContentParts map[int]*contentPartState
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

		diveEvents = append(diveEvents, &llm.Event{
			Type:  llm.EventTypeContentBlockStart,
			Index: &outputIdx,
			ContentBlock: &llm.EventContentBlock{
				Type: diveContentType,
			},
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
		// This indicates the start of reasoning summary content
		if data.Part.Type == "summary_text" {
			// We need to track this as a thinking content block
			// Use a special index for reasoning summary (e.g., -1 to distinguish from regular output items)
			summaryIndex := -1

			// Create state for the summary content
			s.outputItemsState[summaryIndex] = &outputItemState{
				OutputIndex:  summaryIndex,
				ItemID:       fmt.Sprintf("reasoning_summary_%d", s.eventCount),
				ItemType:     "reasoning_summary",
				ContentParts: make(map[int]*contentPartState),
			}

			// Add the content part state
			s.outputItemsState[summaryIndex].ContentParts[0] = &contentPartState{
				ContentIndex: 0,
				PartType:     "thinking",
				Text:         data.Part.Text,
			}

			diveEvents = append(diveEvents, &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &summaryIndex,
				ContentBlock: &llm.EventContentBlock{
					Type:     llm.ContentTypeThinking,
					Thinking: data.Part.Text,
				},
			})
		}

	case responses.ResponseReasoningSummaryTextDeltaEvent:
		// Handle incremental text for reasoning summary
		summaryIndex := -1

		// Get or create the state for reasoning summary
		itemState, exists := s.outputItemsState[summaryIndex]
		if !exists {
			// Create state if it doesn't exist (shouldn't happen normally)
			s.outputItemsState[summaryIndex] = &outputItemState{
				OutputIndex:  summaryIndex,
				ItemID:       fmt.Sprintf("reasoning_summary_%d", s.eventCount),
				ItemType:     "reasoning_summary",
				ContentParts: make(map[int]*contentPartState),
			}
			s.outputItemsState[summaryIndex].ContentParts[0] = &contentPartState{
				ContentIndex: 0,
				PartType:     "thinking",
			}
			itemState = s.outputItemsState[summaryIndex]

			// Emit a start event if we haven't already
			diveEvents = append(diveEvents, &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &summaryIndex,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeThinking,
				},
			})
		}

		// Accumulate the text
		if partState, ok := itemState.ContentParts[0]; ok {
			partState.Text += data.Delta
		}

		// Emit the delta event
		diveEvents = append(diveEvents, &llm.Event{
			Type:  llm.EventTypeContentBlockDelta,
			Index: &summaryIndex,
			Delta: &llm.EventDelta{
				Type:     llm.EventDeltaTypeThinking,
				Thinking: data.Delta,
			},
		})

	case responses.ResponseReasoningSummaryPartDoneEvent:
		// Handle completion of reasoning summary part
		summaryIndex := -1
		if itemState, exists := s.outputItemsState[summaryIndex]; exists {
			if partState, ok := itemState.ContentParts[0]; ok {
				partState.IsComplete = true
				partState.Text = data.Part.Text // Set final text
			}
		}

	case responses.ResponseReasoningSummaryDoneEvent:
		// Handle completion of entire reasoning summary
		summaryIndex := -1
		if itemState, exists := s.outputItemsState[summaryIndex]; exists {
			itemState.IsComplete = true
			diveEvents = append(diveEvents, &llm.Event{
				Type:  llm.EventTypeContentBlockStop,
				Index: &summaryIndex,
			})
		}

	case responses.ResponseReasoningDeltaEvent:
		outputIdx := int(data.OutputIndex)
		contentIdx := int(data.ContentIndex)

		itemState, itemOk := s.outputItemsState[outputIdx]
		if !itemOk {
			return nil, fmt.Errorf("reasoning delta for unknown output index %d", outputIdx)
		}

		partState, partOk := itemState.ContentParts[contentIdx]
		if !partOk {
			partState = &contentPartState{
				ContentIndex: contentIdx,
				PartType:     "thinking",
			}
			itemState.ContentParts[contentIdx] = partState

			diveEvents = append(diveEvents, &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &outputIdx,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeThinking,
				},
			})
		}

		// Handle delta that might be string or other type
		var reasoningTextChunk string
		if textChunk, ok := data.Delta.(string); ok {
			reasoningTextChunk = textChunk
		} else {
			jsonBytes, _ := json.Marshal(data.Delta)
			reasoningTextChunk = string(jsonBytes)
		}
		partState.Text += reasoningTextChunk

		diveEvents = append(diveEvents, &llm.Event{
			Type:  llm.EventTypeContentBlockDelta,
			Index: &outputIdx,
			Delta: &llm.EventDelta{
				Type:     llm.EventDeltaTypeThinking,
				Thinking: reasoningTextChunk,
			},
		})

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

	case responses.ResponseReasoningDoneEvent:
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

	case responses.ResponseOutputItemDoneEvent:
		outputIdx := int(data.OutputIndex)
		if item, ok := s.outputItemsState[outputIdx]; ok && !item.IsComplete {
			item.IsComplete = true
			if item.ItemType == "function_call" {
				diveEvents = append(diveEvents, &llm.Event{
					Type:  llm.EventTypeContentBlockStop,
					Index: &outputIdx,
				})
			}
		}

	case responses.ResponseCompletedEvent:
		s.finalUsage = &llm.Usage{
			InputTokens:  int(data.Response.Usage.InputTokens),
			OutputTokens: int(data.Response.Usage.OutputTokens),
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
		s.finalUsage = &llm.Usage{
			InputTokens:  int(data.Response.Usage.InputTokens),
			OutputTokens: int(data.Response.Usage.OutputTokens),
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

	// Switch event to zero-based indexing to match Anthropic's event behavior
	for _, event := range diveEvents {
		if event.Index != nil {
			*event.Index--
		}
	}

	return diveEvents, nil
}
