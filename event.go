package dive

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
)

var ErrStreamClosed = errors.New("stream is closed")

// EventType is the type of event emitted by an Agent or Workflow.
type EventType string

const (
	EventTypeResponseCreated    EventType = "response.created"
	EventTypeResponseInProgress EventType = "response.in_progress"
	EventTypeResponseCompleted  EventType = "response.completed"
	EventTypeResponseFailed     EventType = "response.failed"
	EventTypeResponseToolCall   EventType = "response.tool_call"
	EventTypeResponseToolResult EventType = "response.tool_result"
	EventTypeResponseToolError  EventType = "response.tool_error"
	EventTypeLLMEvent           EventType = "llm.event"
	EventTypeError              EventType = "error"
)

func (t EventType) String() string {
	return string(t)
}

// ResponseEvent carries information about an event that occurred during a Dive
// LLM interaction.
type ResponseEvent struct {
	// Type of the event
	Type EventType `json:"type"`

	// Error is set if this Event corresponds to an error
	Error error `json:"error,omitempty"`

	// Item contains the item for item events
	Item *ResponseItem `json:"item,omitempty"`

	// Response contains the complete response for response completed events
	Response *Response `json:"response,omitempty"`
}

// EventPublisher is an interface used to send events to an EventStream.
// These functions are safe to call concurrently.
type EventPublisher interface {
	// Send sends an event to the stream
	Send(ctx context.Context, event *ResponseEvent) error

	// Close closes the publisher and the referenced stream. This signals to the
	// consumer that no more events will be sent.
	Close()
}

// ReadEventPayloads waits for and returns all events with a payload of the specified type.
// It will return an error if the context is canceled or if an error event is received.
func ReadEventPayloads[T any](ctx context.Context, stream ResponseStream) ([]T, error) {
	var results []T
	for stream.Next(ctx) {
		event := stream.Event()
		if event == nil {
			return results, fmt.Errorf("received nil event from stream")
		}
		if event.Error != nil {
			return results, fmt.Errorf("received error event from stream: %w", event.Error)
		}
		// This needs to be implemented by the caller based on the ResponseEvent structure
		// as ResponseEvent no longer has a generic Payload field
	}
	if err := stream.Err(); err != nil {
		return results, err
	}
	return results, nil
}

// ReadMessages returns all messages generated in an interaction. This may
// include both assistant messages and tool result messages.
func ReadMessages(ctx context.Context, stream ResponseStream) ([]*llm.Message, error) {
	var messages []*llm.Message
	for stream.Next(ctx) {
		event := stream.Event()
		if event == nil {
			return nil, fmt.Errorf("received nil event from stream")
		}
		if event.Error != nil {
			return nil, fmt.Errorf("received error event from stream: %w", event.Error)
		}
		if event.Type == EventTypeResponseCompleted {
			for _, item := range event.Response.Items {
				if item.Type == ResponseItemTypeMessage {
					messages = append(messages, item.Message)
				}
			}
			return messages, nil
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("stream ended without a response completed event")
}

type responseEventStream struct {
	ch        chan *ResponseEvent
	curr      *ResponseEvent
	err       error
	closed    bool
	pub       EventPublisher
	closeOnce sync.Once
}

// NewEventStream returns a new event stream and a publisher for the stream.
func NewEventStream() (ResponseStream, EventPublisher) {
	s := &responseEventStream{ch: make(chan *ResponseEvent, 16)}
	p := &eventPublisher{
		stream: s,
		done:   make(chan struct{}),
	}
	s.pub = p
	return s, p
}

func (s *responseEventStream) Next(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		s.closed = true
		s.err = ctx.Err()
		return false
	case event, ok := <-s.ch:
		if !ok {
			s.closed = true
			return false
		}
		s.curr = event
		return true
	}
}

func (s *responseEventStream) Event() *ResponseEvent {
	return s.curr
}

func (s *responseEventStream) Err() error {
	return s.err
}

func (s *responseEventStream) Close() error {
	s.closeOnce.Do(func() {
		s.closed = true
		s.pub.Close()
	})
	return nil
}

type eventPublisher struct {
	stream *responseEventStream
	done   chan struct{}
}

func (p *eventPublisher) Send(ctx context.Context, event *ResponseEvent) error {
	select {
	case <-p.done:
		return ErrStreamClosed
	default:
		select {
		case <-p.done:
			return ErrStreamClosed
		case <-ctx.Done():
			return ctx.Err()
		case p.stream.ch <- event:
			return nil
		}
	}
}

func (p *eventPublisher) Close() {
	select {
	case <-p.done:
		return
	default:
		close(p.done)
		close(p.stream.ch)
	}
}
