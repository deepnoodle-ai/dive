package google

import (
	"context"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"google.golang.org/genai"
)

type StreamIterator struct {
	ctx          context.Context
	chat         *genai.Chat
	parts        []genai.Part
	model        string
	responseID   string
	contentIndex int

	// Streaming state
	streamSeq    iter.Seq2[*genai.GenerateContentResponse, error]
	streamNext   func() (*genai.GenerateContentResponse, error, bool)
	streamStop   func()
	currentEvent *llm.Event
	err          error
	done         bool
	started      bool

	// Event generation state
	messageStartSent      bool
	contentBlockStartSent bool
	contentBlockStopSent  bool

	mu sync.Mutex
}

func NewStreamIterator(ctx context.Context, chat *genai.Chat, parts []genai.Part, model string) *StreamIterator {
	return &StreamIterator{
		ctx:          ctx,
		chat:         chat,
		parts:        parts,
		model:        model,
		responseID:   fmt.Sprintf("google_%s_%d", model, time.Now().UnixNano()),
		contentIndex: 0,
	}
}

func (s *StreamIterator) Next() bool {
	s.mu.Lock()

	// If already done, return false
	if s.done {
		s.mu.Unlock()
		return false
	}

	// If not started, start the stream
	if !s.started {
		s.started = true
		if err := s.startStream(); err != nil {
			s.err = err
			s.done = true
			s.mu.Unlock()
			return false
		}
	}

	// Send message start event first
	if !s.messageStartSent {
		s.messageStartSent = true
		s.currentEvent = &llm.Event{
			Type: llm.EventTypeMessageStart,
			Message: &llm.Response{
				ID:      s.responseID,
				Type:    "message",
				Role:    llm.Assistant,
				Model:   s.model,
				Content: []llm.Content{},
			},
		}
		s.mu.Unlock()
		return true
	}

	// Send content block start event
	if !s.contentBlockStartSent {
		s.contentBlockStartSent = true
		s.currentEvent = &llm.Event{
			Type:  llm.EventTypeContentBlockStart,
			Index: &s.contentIndex,
			ContentBlock: &llm.EventContentBlock{
				Type: llm.ContentTypeText,
			},
		}
		s.mu.Unlock()
		return true
	}

	// Check if context is cancelled
	select {
	case <-s.ctx.Done():
		s.err = s.ctx.Err()
		s.done = true
		s.mu.Unlock()
		return false
	default:
	}

	// Read from stream iterator
	if s.streamNext != nil {
		response, err, hasMore := s.streamNext()
		if err != nil {
			s.err = err
			s.done = true
			s.mu.Unlock()
			return false
		}

		if !hasMore {
			// Stream ended, send final events
			result := s.sendFinalEvents()
			s.mu.Unlock()
			return result
		}

		// Process the response
		if response != nil {
			text := response.Text()
			if text != "" {
				s.currentEvent = &llm.Event{
					Type:  llm.EventTypeContentBlockDelta,
					Index: &s.contentIndex,
					Delta: &llm.EventDelta{
						Type: llm.EventDeltaTypeText,
						Text: text,
					},
				}
				s.mu.Unlock()
				return true
			}
		}

		// If no text, continue to next iteration (release lock first)
		s.mu.Unlock()
		return s.Next()
	}

	// Should not reach here
	s.done = true
	s.mu.Unlock()
	return false
}

func (s *StreamIterator) startStream() error {
	// Validate preconditions
	if s.chat == nil {
		return fmt.Errorf("chat is nil - streaming cannot proceed")
	}
	if len(s.parts) == 0 {
		return fmt.Errorf("no parts to send - streaming cannot proceed")
	}

	// Start the stream using the iterator pattern
	s.streamSeq = s.chat.SendMessageStream(s.ctx, s.parts...)

	// Create a next function from the iterator
	s.streamNext, s.streamStop = iter.Pull2(s.streamSeq)

	return nil
}

func (s *StreamIterator) sendFinalEvents() bool {
	// Send content block stop event first
	if !s.contentBlockStopSent {
		s.contentBlockStopSent = true
		s.currentEvent = &llm.Event{
			Type:  llm.EventTypeContentBlockStop,
			Index: &s.contentIndex,
		}
		return true
	}

	// Then send message stop event
	if !s.done {
		s.done = true
		s.currentEvent = &llm.Event{
			Type: llm.EventTypeMessageStop,
		}
		return true
	}

	// All events sent
	return false
}

func (s *StreamIterator) Event() *llm.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentEvent
}

func (s *StreamIterator) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *StreamIterator) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
	if s.streamStop != nil {
		s.streamStop()
	}
	return nil
}
