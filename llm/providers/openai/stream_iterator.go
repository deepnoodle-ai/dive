package openai

import (
	"fmt"
	"io"
	"strings"

	"github.com/diveagents/dive/llm"
)

// StreamIterator implements llm.StreamIterator for the Responses API
type StreamIterator struct {
	reader            *llm.ServerSentEventsReader[StreamEvent]
	body              io.ReadCloser
	err               error
	currentEvent      *llm.Event
	eventCount        int
	previousText      string
	hasStartedContent bool
	hasEmittedStop    bool
	nextContentIndex  int
	textContentIndex  int
	eventQueue        []*llm.Event
}

func (s *StreamIterator) emitDone() bool {
	if !s.hasEmittedStop {
		// Emit message stop event before ending
		s.hasEmittedStop = true
		s.currentEvent = &llm.Event{Type: llm.EventTypeMessageStop}
		return true
	}
	return false
}

// Next advances to the next event in the stream
func (s *StreamIterator) Next() bool {
	// If we have events in the queue, return the next one
	if len(s.eventQueue) > 0 {
		s.currentEvent = s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return true
	}

	// Read the next event from the stream
	for {
		event, ok := s.reader.Next()
		if !ok {
			s.err = s.reader.Err()
			return s.emitDone()
		}
		events := s.convertStreamEvent(&event)
		if len(events) > 0 {
			// Return the first event and queue the rest
			s.currentEvent = events[0]
			if len(events) > 1 {
				s.eventQueue = append(s.eventQueue, events[1:]...)
			}
			return true
		}
	}
}

// convertStreamEvent converts a StreamEvent to a slice of llm.Event
func (s *StreamIterator) convertStreamEvent(streamEvent *StreamEvent) []*llm.Event {
	var events []*llm.Event

	switch streamEvent.Type {
	case "response.created":
		// First event - emit message start
		if streamEvent.Response != nil && s.eventCount == 0 {
			s.eventCount++
			return []*llm.Event{{
				Type: llm.EventTypeMessageStart,
				Message: &llm.Response{
					ID:      streamEvent.Response.ID,
					Type:    "message",
					Role:    llm.Assistant,
					Model:   streamEvent.Response.Model,
					Content: []llm.Content{},
					Usage:   llm.Usage{},
				},
			}}
		}

	case "response.in_progress":
		// Just status update, no events needed
		return nil

	case "response.output_item.added":
		// New output item added - this indicates a message is starting
		// We already emitted message_start, so no additional events needed
		return nil

	case "response.content_part.added":
		// New content part added - emit content_block_start
		if !s.hasStartedContent {
			s.hasStartedContent = true
			s.textContentIndex = s.nextContentIndex
			s.nextContentIndex++

			initialText := ""
			if streamEvent.Part != nil {
				initialText = streamEvent.Part.Text
			}
			s.previousText = initialText

			events = append(events, &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &s.textContentIndex,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeText,
					Text: initialText,
				},
			})
		}

	case "response.output_text.delta":
		// Text delta - emit content_block_delta
		if streamEvent.Delta != "" {
			events = append(events, &llm.Event{
				Type:  llm.EventTypeContentBlockDelta,
				Index: &s.textContentIndex,
				Delta: &llm.EventDelta{
					Type: llm.EventDeltaTypeText,
					Text: streamEvent.Delta,
				},
			})
			s.previousText += streamEvent.Delta
		}

	case "response.output_text.done":
		// Text is complete, but we don't emit content_block_stop yet
		// as the content part might not be done
		return nil

	case "response.content_part.done":
		// Content part is complete - emit content_block_stop
		events = append(events, &llm.Event{
			Type:  llm.EventTypeContentBlockStop,
			Index: &s.textContentIndex,
		})

	case "response.output_item.done":
		// Output item is complete - no specific event needed
		return nil

	case "response.completed":
		// Response is complete - emit message_stop and usage
		if streamEvent.Response != nil && streamEvent.Response.Usage != nil {
			events = append(events, &llm.Event{
				Type:  llm.EventTypeMessageDelta,
				Delta: &llm.EventDelta{}, // Empty delta is required for message delta events
				Usage: &llm.Usage{
					InputTokens:  streamEvent.Response.Usage.InputTokens,
					OutputTokens: streamEvent.Response.Usage.OutputTokens,
				},
			})
		}

		if !s.hasEmittedStop {
			s.hasEmittedStop = true
			events = append(events, &llm.Event{
				Type: llm.EventTypeMessageStop,
			})
		}

	case "response":
		// Legacy format - fall back to the original response-based processing
		return s.convertResponseBasedEvent(streamEvent)

	default:
		// For other event types, fall back to the original logic
		// This handles tool calls and other complex events
		if streamEvent.Response != nil {
			return s.convertResponseBasedEvent(streamEvent)
		}
	}

	return events
}

// convertResponseBasedEvent handles events that need to be processed based on response.output
// This is the fallback for complex events like tool calls and legacy "response" events
func (s *StreamIterator) convertResponseBasedEvent(streamEvent *StreamEvent) []*llm.Event {
	response := streamEvent.Response
	if response == nil {
		return nil
	}

	var events []*llm.Event

	// Emit message start event if this is the first event (for legacy format)
	if s.eventCount == 0 {
		s.eventCount++
		events = append(events, &llm.Event{
			Type: llm.EventTypeMessageStart,
			Message: &llm.Response{
				ID:      response.ID,
				Type:    "message",
				Role:    llm.Assistant,
				Model:   response.Model,
				Content: []llm.Content{},
				Usage:   llm.Usage{},
			},
		})
	}

	// Process ALL output items for content deltas
	for _, item := range response.Output {
		switch item.Type {
		case "message":
			// Handle text content deltas (legacy format)
			for _, content := range item.Content {
				if content.Type == "text" || content.Type == "output_text" {
					currentText := content.Text

					// If this is the first time we see content, emit a content block start event
					if !s.hasStartedContent {
						s.hasStartedContent = true
						s.previousText = currentText
						s.textContentIndex = s.nextContentIndex
						s.nextContentIndex++
						events = append(events, &llm.Event{
							Type:  llm.EventTypeContentBlockStart,
							Index: &s.textContentIndex,
							ContentBlock: &llm.EventContentBlock{
								Type: llm.ContentTypeText,
								Text: currentText,
							},
						})
					} else if currentText != s.previousText {
						// If the text has changed, emit a delta event with only the new text
						deltaText := currentText[len(s.previousText):]
						s.previousText = currentText
						events = append(events, &llm.Event{
							Type:  llm.EventTypeContentBlockDelta,
							Index: &s.textContentIndex,
							Delta: &llm.EventDelta{
								Type: llm.EventDeltaTypeText,
								Text: deltaText,
							},
						})
					}
				}
			}
		case "function_call":
			// Handle tool call events
			toolIndex := s.nextContentIndex
			s.nextContentIndex++
			events = append(events, &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &toolIndex,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeToolUse,
					ID:   item.CallID,
					Name: item.Name,
				},
			})
		case "image_generation_call":
			// Handle image generation events
			if item.Result != "" {
				imageIndex := s.nextContentIndex
				s.nextContentIndex++
				// For now, convert image to text content since EventContentBlock doesn't support images directly
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &imageIndex,
					ContentBlock: &llm.EventContentBlock{
						Type: llm.ContentTypeText,
						Text: fmt.Sprintf("[Generated image - base64 data: %d bytes]", len(item.Result)),
					},
				})
			}
		case "web_search_call":
			// Handle web search results
			if len(item.Results) > 0 {
				searchIndex := s.nextContentIndex
				s.nextContentIndex++
				var resultText strings.Builder
				resultText.WriteString("Web search results:\n")
				for _, result := range item.Results {
					resultText.WriteString(fmt.Sprintf("- %s: %s\n", result.Title, result.Description))
				}
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &searchIndex,
					ContentBlock: &llm.EventContentBlock{
						Type: llm.ContentTypeText,
						Text: resultText.String(),
					},
				})
			}
		case "mcp_call":
			// Handle MCP tool call events
			toolIndex := s.nextContentIndex
			s.nextContentIndex++
			events = append(events, &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &toolIndex,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeToolUse,
					ID:   item.ID,
					Name: item.Name,
				},
			})
		case "mcp_list_tools":
			// Handle MCP tool list events - emit as text content
			if len(item.Tools) > 0 {
				toolIndex := s.nextContentIndex
				s.nextContentIndex++
				var toolsText strings.Builder
				toolsText.WriteString(fmt.Sprintf("MCP server '%s' tools:\n", item.ServerLabel))
				for _, tool := range item.Tools {
					toolsText.WriteString(fmt.Sprintf("- %s\n", tool.Name))
				}
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &toolIndex,
					ContentBlock: &llm.EventContentBlock{
						Type: llm.ContentTypeText,
						Text: toolsText.String(),
					},
				})
			}
		case "mcp_approval_request":
			// Handle MCP approval request events - emit as text content
			toolIndex := s.nextContentIndex
			s.nextContentIndex++
			events = append(events, &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &toolIndex,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeText,
					Text: fmt.Sprintf("MCP approval required for tool '%s' on server '%s'", item.Name, item.ServerLabel),
				},
			})
		case "partial_image":
			// Handle partial image events for streaming image generation
			if item.Result != "" {
				imageIndex := s.nextContentIndex - 1 // Use the existing image index
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockDelta,
					Index: &imageIndex,
					Delta: &llm.EventDelta{
						Type:        llm.EventDeltaTypeText,
						Text:        fmt.Sprintf("[Partial image update - %d bytes]", len(item.Result)),
						PartialJSON: item.Result, // Store the partial image data in PartialJSON field
					},
				})
			}
		}
	}

	// If we have usage information, emit a message delta event (for legacy format completion)
	if response.Usage != nil && len(events) > 0 {
		events = append(events, &llm.Event{
			Type:  llm.EventTypeMessageDelta,
			Delta: &llm.EventDelta{}, // Empty delta is required for message delta events
			Usage: &llm.Usage{
				InputTokens:  response.Usage.InputTokens,
				OutputTokens: response.Usage.OutputTokens,
			},
		})
	}

	return events
}

// Event returns the current event
func (s *StreamIterator) Event() *llm.Event {
	return s.currentEvent
}

// Err returns any error that occurred
func (s *StreamIterator) Err() error {
	return s.err
}

// Close closes the stream
func (s *StreamIterator) Close() error {
	if s.body != nil {
		return s.body.Close()
	}
	return nil
}
