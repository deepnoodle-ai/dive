package google

import (
	"context"
	"fmt"
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
	"google.golang.org/genai"
)

type StreamIterator struct {
	ctx           context.Context
	chat          *genai.Chat
	parts         []genai.Part
	streamStarted bool
	iterator      func(func(*genai.GenerateContentResponse, error) bool)
	eventQueue    []*llm.Event
	currentEvent  *llm.Event
	err           error
	done          bool
	mu            sync.Mutex
	responseID    string
	model         string
	eventCount    int
	textBuffer    string
	contentIndex  int
}

func NewStreamIterator(ctx context.Context, chat *genai.Chat, parts []genai.Part, model string) *StreamIterator {
	return &StreamIterator{
		ctx:        ctx,
		chat:       chat,
		parts:      parts,
		model:      model,
		eventQueue: make([]*llm.Event, 0),
		responseID: fmt.Sprintf("google_%d", ctx.Value("requestID")),
	}
}

func (s *StreamIterator) Next() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Return queued events first
	if len(s.eventQueue) > 0 {
		s.currentEvent = s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return true
	}

	if s.done {
		return false
	}

	// Start streaming if not already started
	if !s.streamStarted {
		s.streamStarted = true
		s.startStream()
	}

	// Return next event from queue if available
	if len(s.eventQueue) > 0 {
		s.currentEvent = s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return true
	}

	return false
}

func (s *StreamIterator) startStream() {
	// Emit message start event
	s.eventCount++
	s.eventQueue = append(s.eventQueue, &llm.Event{
		Type: llm.EventTypeMessageStart,
		Message: &llm.Response{
			ID:      s.responseID,
			Type:    "message",
			Role:    llm.Assistant,
			Model:   s.model,
			Content: []llm.Content{},
		},
	})

	// Emit content block start event
	s.eventQueue = append(s.eventQueue, &llm.Event{
		Type:  llm.EventTypeContentBlockStart,
		Index: &s.contentIndex,
		ContentBlock: &llm.EventContentBlock{
			Type: llm.ContentTypeText,
		},
	})

	// Start streaming from Google
	go func() {
		defer func() {
			s.mu.Lock()
			// Emit content block stop
			s.eventQueue = append(s.eventQueue, &llm.Event{
				Type:  llm.EventTypeContentBlockStop,
				Index: &s.contentIndex,
			})
			// Emit message stop
			s.eventQueue = append(s.eventQueue, &llm.Event{
				Type: llm.EventTypeMessageStop,
			})
			s.done = true
			s.mu.Unlock()
		}()

		for result, err := range s.chat.SendMessageStream(s.ctx, s.parts...) {
			if err != nil {
				s.mu.Lock()
				s.err = err
				s.done = true
				s.mu.Unlock()
				return
			}

			text := result.Text()
			if text != "" {
				s.mu.Lock()
				s.eventQueue = append(s.eventQueue, &llm.Event{
					Type:  llm.EventTypeContentBlockDelta,
					Index: &s.contentIndex,
					Delta: &llm.EventDelta{
						Type: llm.EventDeltaTypeText,
						Text: text,
					},
				})
				s.textBuffer += text
				s.mu.Unlock()
			}
		}
	}()
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
	return nil
}
