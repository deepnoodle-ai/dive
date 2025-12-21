package anthropic

import (
	"io"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
)

// StreamIterator implements the llm.StreamIterator interface for Anthropic streaming responses
type StreamIterator struct {
	reader            *llm.ServerSentEventsReader[llm.Event]
	body              io.ReadCloser
	err               error
	currentEvent      *llm.Event
	prefill           string
	prefillClosingTag string
	closeOnce         sync.Once
}

// Next advances to the next event in the stream. Returns true if an event was
// successfully read, false when the stream is complete or an error occurs.
func (s *StreamIterator) Next() bool {
	for {
		event, ok := s.reader.Next()
		if !ok {
			s.err = s.reader.Err()
			s.Close()
			return false
		}
		processedEvent := s.processEvent(&event)
		if processedEvent != nil {
			s.currentEvent = processedEvent
			return true
		}
	}
}

// Event returns the current event. Should only be called after a successful Next().
func (s *StreamIterator) Event() *llm.Event {
	return s.currentEvent
}

// processEvent processes an Anthropic event and applies prefill logic if needed
func (s *StreamIterator) processEvent(event *llm.Event) *llm.Event {
	if event.Type == "" {
		return nil
	}

	// Apply prefill logic for the first text content block
	if s.prefill != "" && event.Type == llm.EventTypeContentBlockStart {
		if event.ContentBlock != nil && event.ContentBlock.Type == llm.ContentTypeText {
			// Add prefill to the beginning of the text
			if s.prefillClosingTag == "" || strings.Contains(event.ContentBlock.Text, s.prefillClosingTag) {
				event.ContentBlock.Text = s.prefill + event.ContentBlock.Text
				s.prefill = "" // Only apply prefill once
			}
		}
	}

	return event
}

func (s *StreamIterator) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.body.Close() })
	return err
}

func (s *StreamIterator) Err() error {
	return s.err
}
