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

// StreamIterator adapts the genai streaming sequence to Dive's llm.Event
// stream, mirroring the event semantics of the Anthropic and OpenAI
// iterators: content blocks get 0-based contiguous indices, parallel function
// calls are each surfaced as their own tool_use block, and the final
// message_delta carries usage and the stop reason.
type StreamIterator struct {
	ctx        context.Context
	model      string
	responseID string

	// Streaming state
	streamSeq    iter.Seq2[*genai.GenerateContentResponse, error]
	streamNext   func() (*genai.GenerateContentResponse, error, bool)
	streamStop   func()
	currentEvent *llm.Event
	eventQueue   []*llm.Event
	err          error
	done         bool
	started      bool

	// Event generation state
	messageStartSent bool
	nextBlockIndex   int
	textBlockIndex   int // index of the open text block, or -1 if none
	usage            *genai.GenerateContentResponseUsageMetadata
	finishReason     genai.FinishReason

	mu sync.Mutex
}

// NewStreamIteratorFromSeq creates a new StreamIterator from a streaming sequence
func NewStreamIteratorFromSeq(ctx context.Context, streamSeq iter.Seq2[*genai.GenerateContentResponse, error], model string) *StreamIterator {
	return &StreamIterator{
		ctx:            ctx,
		streamSeq:      streamSeq,
		model:          model,
		responseID:     fmt.Sprintf("google_%s_%d", model, time.Now().UnixNano()),
		textBlockIndex: -1,
	}
}

func (s *StreamIterator) Next() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		// Drain queued events first
		if len(s.eventQueue) > 0 {
			s.currentEvent = s.eventQueue[0]
			s.eventQueue = s.eventQueue[1:]
			return true
		}

		if s.done {
			return false
		}

		// If not started, start the stream
		if !s.started {
			s.started = true
			if err := s.startStream(); err != nil {
				s.err = err
				s.done = true
				return false
			}
		}

		// Check if context is cancelled
		select {
		case <-s.ctx.Done():
			s.err = s.ctx.Err()
			s.done = true
			return false
		default:
		}

		response, err, hasMore := s.streamNext()
		if err != nil {
			s.err = wrapGoogleError(err)
			s.done = true
			return false
		}
		if !hasMore {
			// Stream ended: close any open block and emit the final
			// message_delta (usage + stop reason) and message_stop events.
			s.queueFinalEvents()
			s.done = true
			continue
		}
		if err := s.processChunk(response); err != nil {
			s.err = err
			s.done = true
			return false
		}
	}
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

// processChunk converts a single streamed response chunk into events on the
// event queue.
func (s *StreamIterator) processChunk(response *genai.GenerateContentResponse) error {
	if response == nil {
		return nil
	}

	if response.ResponseID != "" {
		s.responseID = response.ResponseID
	}

	// Send message start event first
	if !s.messageStartSent {
		s.messageStartSent = true
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
	}

	// Track the latest usage metadata; Gemini reports cumulative totals, the
	// last chunk carrying the final values. Emitted once on message_delta.
	if response.UsageMetadata != nil {
		s.usage = response.UsageMetadata
	}

	if len(response.Candidates) == 0 {
		return nil
	}
	candidate := response.Candidates[0]
	if candidate.FinishReason != "" {
		s.finishReason = candidate.FinishReason
	}
	if candidate.Content == nil {
		return nil
	}

	for _, part := range candidate.Content.Parts {
		switch {
		case part.FunctionCall != nil:
			if err := s.queueFunctionCall(part); err != nil {
				return err
			}
		case part.Text != "":
			if part.Thought {
				// Thought-summary parts are not surfaced (matching the
				// previous behavior based on response.Text(), which skips
				// them).
				continue
			}
			s.queueText(part.Text)
		}
	}
	return nil
}

// queueText emits a text delta, opening a new text content block if needed.
func (s *StreamIterator) queueText(text string) {
	if s.textBlockIndex < 0 {
		index := s.nextBlockIndex
		s.nextBlockIndex++
		s.textBlockIndex = index
		s.eventQueue = append(s.eventQueue, &llm.Event{
			Type:  llm.EventTypeContentBlockStart,
			Index: &index,
			ContentBlock: &llm.EventContentBlock{
				Type: llm.ContentTypeText,
			},
		})
	}
	index := s.textBlockIndex
	s.eventQueue = append(s.eventQueue, &llm.Event{
		Type:  llm.EventTypeContentBlockDelta,
		Index: &index,
		Delta: &llm.EventDelta{
			Type: llm.EventDeltaTypeText,
			Text: text,
		},
	})
}

// queueFunctionCall emits a complete tool_use content block for one function
// call. Gemini delivers each function call complete in a single part (there
// is no partial-JSON streaming), so the block starts, receives its full input
// as one delta, and stops immediately. Parallel calls each get their own
// sequential block index.
func (s *StreamIterator) queueFunctionCall(part *genai.Part) error {
	call := part.FunctionCall
	args, err := json.Marshal(call.Args)
	if err != nil {
		return fmt.Errorf("error marshaling function call args: %w", err)
	}

	// Close any open text block first
	s.closeTextBlock()

	// Gemini does not always populate FunctionCall.ID; synthesize a unique ID
	// when missing (matching convertGoogleResponse).
	toolCallID := call.ID
	if toolCallID == "" {
		toolCallID = generateToolCallID(call.Name)
	}

	index := s.nextBlockIndex
	s.nextBlockIndex++
	s.eventQueue = append(s.eventQueue,
		&llm.Event{
			Type:  llm.EventTypeContentBlockStart,
			Index: &index,
			ContentBlock: &llm.EventContentBlock{
				Type:     llm.ContentTypeToolUse,
				ID:       toolCallID,
				Name:     call.Name,
				Metadata: providerMetadataForGooglePart(part),
			},
		},
		&llm.Event{
			Type:  llm.EventTypeContentBlockDelta,
			Index: &index,
			Delta: &llm.EventDelta{
				Type:        llm.EventDeltaTypeInputJSON,
				PartialJSON: string(args),
			},
		},
		&llm.Event{
			Type:  llm.EventTypeContentBlockStop,
			Index: &index,
		},
	)
	return nil
}

// closeTextBlock emits a content_block_stop for the open text block, if any.
func (s *StreamIterator) closeTextBlock() {
	if s.textBlockIndex < 0 {
		return
	}
	index := s.textBlockIndex
	s.textBlockIndex = -1
	s.eventQueue = append(s.eventQueue, &llm.Event{
		Type:  llm.EventTypeContentBlockStop,
		Index: &index,
	})
}

// queueFinalEvents closes any open content block and emits message_delta
// (carrying usage and the stop reason) followed by message_stop.
func (s *StreamIterator) queueFinalEvents() {
	if !s.messageStartSent {
		// Stream ended without any chunks; nothing to finalize.
		return
	}
	s.closeTextBlock()

	delta := &llm.EventDelta{}
	if s.finishReason != "" {
		delta.StopReason = convertFinishReason(s.finishReason)
	}
	event := &llm.Event{
		Type:  llm.EventTypeMessageDelta,
		Delta: delta,
	}
	if s.usage != nil {
		usage := convertUsageMetadata(s.usage)
		event.Usage = &usage
	}
	s.eventQueue = append(s.eventQueue, event, &llm.Event{
		Type: llm.EventTypeMessageStop,
	})
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
