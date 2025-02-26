package dive

import (
	"context"
	"sync"
)

var _ Stream = &DiveStream{}

type DiveStream struct {
	events chan *StreamEvent
	done   chan struct{} // Signal channel for shutdown
}

func NewDiveStream() *DiveStream {
	return &DiveStream{
		events: make(chan *StreamEvent, 64),
		done:   make(chan struct{}),
	}
}

func (s *DiveStream) Channel() <-chan *StreamEvent {
	return s.events
}

func (s *DiveStream) Close() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}
}

// DiveStreamPublisher is a helper for sending events to a DiveStream
type DiveStreamPublisher struct {
	stream    *DiveStream
	closeOnce sync.Once
}

func NewDiveStreamPublisher(stream *DiveStream) *DiveStreamPublisher {
	return &DiveStreamPublisher{
		stream:    stream,
		closeOnce: sync.Once{},
	}
}

// Send sends an event to the stream's events channel
func (p *DiveStreamPublisher) Send(ctx context.Context, event *StreamEvent) bool {
	select {
	case p.stream.events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close the publisher. After closing, the publisher must NOT call any of the
// Send methods again. Doing so would result in a panic.
func (p *DiveStreamPublisher) Close() {
	p.closeOnce.Do(func() {
		close(p.stream.events)
	})
}
