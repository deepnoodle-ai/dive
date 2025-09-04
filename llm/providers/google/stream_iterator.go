package google

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"google.golang.org/genai"
)

type StreamIterator struct {
	ctx          context.Context
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
	toolCallsSent         bool
	toolInputSent         bool
	isToolCall            bool
	pendingToolInput      json.RawMessage

	mu sync.Mutex
}

// NewStreamIteratorFromSeq creates a new StreamIterator from a streaming sequence
func NewStreamIteratorFromSeq(ctx context.Context, streamSeq iter.Seq2[*genai.GenerateContentResponse, error], model string) *StreamIterator {
	return &StreamIterator{
		ctx:          ctx,
		streamSeq:    streamSeq,
		model:        model,
		responseID:   fmt.Sprintf("google_%s_%d", model, time.Now().UnixNano()),
		contentIndex: 0,
	}
}

// NewStreamIterator creates a new StreamIterator (deprecated, use NewStreamIteratorFromSeq)
func NewStreamIterator(ctx context.Context, chat *genai.Chat, parts []genai.Part, model string) *StreamIterator {
	// Legacy constructor for backward compatibility
	// Start the stream using the chat API
	var streamSeq iter.Seq2[*genai.GenerateContentResponse, error]
	if chat != nil && len(parts) > 0 {
		streamSeq = chat.SendMessageStream(ctx, parts...)
	}
	return &StreamIterator{
		ctx:          ctx,
		streamSeq:    streamSeq,
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

	// Send content block start event (only for text, tool calls are handled differently)
	if !s.contentBlockStartSent && !s.isToolCall {
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

	// Send tool input as delta event if we have pending tool input
	if s.isToolCall && !s.toolInputSent && s.pendingToolInput != nil {
		s.toolInputSent = true
		s.currentEvent = &llm.Event{
			Type:  llm.EventTypeContentBlockDelta,
			Index: &s.contentIndex,
			Delta: &llm.EventDelta{
				Type:        llm.EventDeltaTypeInputJSON,
				PartialJSON: string(s.pendingToolInput),
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
			// Check for function calls first
			calls := response.FunctionCalls()
			if len(calls) > 0 && !s.toolCallsSent {
				s.toolCallsSent = true
				s.isToolCall = true

				// Send tool use event - only handle the first call for now
				call := calls[0]
				args, err := json.Marshal(call.Args)
				if err != nil {
					s.err = fmt.Errorf("error marshaling function call args: %w", err)
					s.done = true
					s.mu.Unlock()
					return false
				}

				// Store the tool input to be sent as a delta event
				s.pendingToolInput = json.RawMessage(args)

				s.currentEvent = &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &s.contentIndex,
					ContentBlock: &llm.EventContentBlock{
						Type: llm.ContentTypeToolUse,
						ID:   generateToolCallID(call.Name),
						Name: call.Name,
					},
				}
				s.mu.Unlock()
				return true
			}

			// Handle text content
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
	if s.streamSeq == nil {
		return fmt.Errorf("stream sequence is nil - streaming cannot proceed")
	}

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
