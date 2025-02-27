package dive

import (
	"context"
	"errors"
	"sync"
)

// ErrStreamClosed indicates that the stream has been closed.
var ErrStreamClosed = errors.New("stream closed")

// Confirm the Stream interface is implemented
var _ Stream = &DiveStream{}

// DiveStream is an implementation of the Stream interface. It is used to stream
// task events from a team or agent to a client.
type DiveStream struct {
	events chan *StreamEvent
	done   chan struct{} // Signal channel for shutdown
}

// NewDiveStream returns a new Stream. Typically, a stream is created for each
// task or group of tasks that is to be executed.
func NewDiveStream() *DiveStream {
	return &DiveStream{
		events: make(chan *StreamEvent, 16),
		done:   make(chan struct{}),
	}
}

// Channel returns a channel that can be used by the client to receive events.
func (s *DiveStream) Channel() <-chan *StreamEvent {
	return s.events
}

// Close is used by the client to indicate that it no longer wishes to receive
// events, even if the task is not yet done. Any publisher should monitor the
// done channel and stop sending events when it is closed.
func (s *DiveStream) Close() {
	select {
	case <-s.done:
		return
	default:
		close(s.done)
	}
}

// StreamPublisher is a helper for sending events to a Stream.
type StreamPublisher struct {
	stream    *DiveStream
	closeOnce sync.Once
	closed    bool
	mutex     sync.Mutex
}

func NewStreamPublisher(stream *DiveStream) *StreamPublisher {
	return &StreamPublisher{
		stream:    stream,
		closeOnce: sync.Once{},
	}
}

// Send sends an event to the stream's events channel
func (p *StreamPublisher) Send(ctx context.Context, event *StreamEvent) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		return ErrStreamClosed
	}

	// Send the event, as long as the stream is open and the context
	// hasn't been canceled.
	select {
	case <-p.stream.done:
		p.close()
		return ErrStreamClosed

	case <-ctx.Done():
		p.close()
		return ctx.Err()

	case p.stream.events <- event:
		return nil
	}
}

// Close the publisher and close the corresponding Stream. No more calls to Send
// should be made, however doing so will not cause a panic.
func (p *StreamPublisher) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.close()
}

func (p *StreamPublisher) close() {
	p.closeOnce.Do(func() {
		p.closed = true
		close(p.stream.events)
	})
}
