package openai

import (
	"fmt"
	"io"

	"github.com/diveagents/dive/llm"
)

// StreamIterator implements llm.StreamIterator for the Responses API
type StreamIterator struct {
	reader               *llm.ServerSentEventsReader[StreamEvent]
	body                 io.ReadCloser
	err                  error
	currentEvent         *llm.Event
	eventCount           int
	previousText         string
	hasStartedContent    bool
	nextContentIndex     int
	textContentIndex     int
	functionCallIndex    int
	hasFunctionCallStart bool
	eventQueue           []*llm.Event
}

// Event returns the current event. Guaranteed to be non-nil if called after
// Next() returns true.
func (s *StreamIterator) Event() *llm.Event {
	return s.currentEvent
}

// Err returns any error that occurred
func (s *StreamIterator) Err() error {
	return s.err
}

// Close the stream
func (s *StreamIterator) Close() error {
	if s.body != nil {
		err := s.body.Close()
		s.body = nil
		return err
	}
	return nil
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
			return false
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
	// response.incomplete (TODO?)
	var events []*llm.Event
	switch streamEvent.Type {
	case "response.in_progress":
		return nil // No event needed

	case "response.output_item.added":
		// Check if this is a function call
		if streamEvent.Item != nil && streamEvent.Item.Type == "function_call" {
			// Emit "content_block_start" for function call
			s.functionCallIndex = s.nextContentIndex
			s.nextContentIndex++
			s.hasFunctionCallStart = true
			return []*llm.Event{{
				Type:  llm.EventTypeContentBlockStart,
				Index: &s.functionCallIndex,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeToolUse,
					ID:   streamEvent.Item.ID,
					Name: streamEvent.Item.Name,
				},
			}}
		}
		return nil // No event needed for non-function-call items

	case "response.output_text.done":
		return nil // No event needed

	case "response.output_item.done":
		return nil // No event needed

	case "response.created":
		// Emit "message_start"
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

	case "response.content_part.added":
		// Emit "content_block_start"
		if !s.hasStartedContent {
			s.hasStartedContent = true
			s.textContentIndex = s.nextContentIndex
			s.nextContentIndex++
			initialText := ""
			if streamEvent.Part != nil {
				initialText = streamEvent.Part.Text
			}
			s.previousText = initialText
			return []*llm.Event{{
				Type:  llm.EventTypeContentBlockStart,
				Index: &s.textContentIndex,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeText,
					Text: initialText,
				},
			}}
		}

	case "response.output_text.delta":
		// Emit "content_block_delta"
		if delta := streamEvent.Delta; delta != "" {
			s.previousText += delta
			return []*llm.Event{{
				Type:  llm.EventTypeContentBlockDelta,
				Index: &s.textContentIndex,
				Delta: &llm.EventDelta{
					Type: llm.EventDeltaTypeText,
					Text: delta,
				},
			}}
		}

	case "response.content_part.done":
		// Emit "content_block_stop"
		return []*llm.Event{{
			Type:  llm.EventTypeContentBlockStop,
			Index: &s.textContentIndex,
		}}

	case "response.function_call_arguments.delta":
		// Emit "content_block_delta" with proper index
		if delta := streamEvent.Delta; delta != "" && s.hasFunctionCallStart {
			return []*llm.Event{{
				Type:  llm.EventTypeContentBlockDelta,
				Index: &s.functionCallIndex,
				Delta: &llm.EventDelta{
					Type:        llm.EventDeltaTypeInputJSON,
					PartialJSON: delta,
				},
			}}
		}

	case "response.function_call_arguments.done":
		// Emit "content_block_stop"
		if s.hasFunctionCallStart {
			return []*llm.Event{{
				Type:  llm.EventTypeContentBlockStop,
				Index: &s.functionCallIndex,
			}}
		}

	case "response.completed":
		// Emit both "message_delta" and "message_stop" events
		if response := streamEvent.Response; response != nil {
			usage := &llm.Usage{}
			if response.Usage != nil {
				usage.InputTokens = response.Usage.InputTokens
				usage.OutputTokens = response.Usage.OutputTokens
				if response.Usage.InputTokensDetails != nil {
					usage.CacheReadInputTokens = response.Usage.InputTokensDetails.CachedTokens
				}
			}

			// Determine stop_reason based on the response content and status
			stopReason := determineStopReason(response)

			events = append(events, &llm.Event{
				Type: llm.EventTypeMessageDelta,
				Delta: &llm.EventDelta{
					StopReason: stopReason,
				},
				Usage: usage,
			})
			// Immediately emit message_stop instead of deferring it
			events = append(events, &llm.Event{
				Type: llm.EventTypeMessageStop,
			})
		}
		return events

	default:
		fmt.Printf("unknown event: %s\n", streamEvent.Type)
	}
	return events
}
