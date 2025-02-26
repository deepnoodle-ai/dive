package dive

import (
	"context"
	"sync"
)

var _ Stream = &DiveStream{}

type DiveStream struct {
	events  chan *StreamEvent
	results chan *TaskResult
	done    chan struct{} // Signal channel for shutdown
}

func NewDiveStream() *DiveStream {
	return &DiveStream{
		events:  make(chan *StreamEvent, 64),
		results: make(chan *TaskResult, 64),
		done:    make(chan struct{}),
	}
}

func (s *DiveStream) Events() <-chan *StreamEvent {
	return s.events
}

func (s *DiveStream) Results() <-chan *TaskResult {
	return s.results
}

func (s *DiveStream) Close() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}
}

// DiveStreamPublisher is a helper for sending events, results, and errors to a
// DiveStream
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

// SendEvent sends an event to the stream's events channel
func (p *DiveStreamPublisher) SendEvent(ctx context.Context, event *StreamEvent) bool {
	select {
	case p.stream.events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

// SendResult sends a task result to the stream's results channel
func (p *DiveStreamPublisher) SendResult(ctx context.Context, result *TaskResult) bool {
	select {
	case p.stream.results <- result:
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
		close(p.stream.results)
	})
}
