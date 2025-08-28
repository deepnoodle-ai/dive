package agent

import (
	"context"

	"github.com/deepnoodle-ai/dive"
)

// // ResponseEventStream interface for streaming response events
// type ResponseEventStream interface {
// 	dive.ResponseStream
// 	Publisher() dive.EventPublisher
// }

// // responseEventStream implementation
// type responseEventStream struct {
// 	events    chan *dive.ResponseEvent
// 	curr      *dive.ResponseEvent
// 	err       error
// 	publisher dive.EventPublisher
// 	closeOnce sync.Once
// 	mu        sync.Mutex
// 	closed    bool
// }

// // newResponseEventStream creates a new response event stream
// func newResponseEventStream() ResponseEventStream {
// 	s := &responseEventStream{
// 		events: make(chan *dive.ResponseEvent, 16),
// 	}
// 	s.publisher = &responseEventPublisher{stream: s}
// 	return s
// }

// func (s *responseEventStream) Next(ctx context.Context) bool {
// 	select {
// 	case <-ctx.Done():
// 		s.mu.Lock()
// 		s.err = ctx.Err()
// 		s.closed = true
// 		s.mu.Unlock()
// 		return false
// 	case event, ok := <-s.events:
// 		if !ok {
// 			s.mu.Lock()
// 			s.closed = true
// 			s.mu.Unlock()
// 			return false
// 		}
// 		s.curr = event
// 		return true
// 	}
// }

// func (s *responseEventStream) Event() *dive.ResponseEvent {
// 	return s.curr
// }

// func (s *responseEventStream) Err() error {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
// 	return s.err
// }

// func (s *responseEventStream) Close() error {
// 	s.closeOnce.Do(func() {
// 		s.mu.Lock()
// 		s.closed = true
// 		close(s.events)
// 		s.mu.Unlock()
// 	})
// 	return nil
// }

// func (s *responseEventStream) Publisher() dive.EventPublisher {
// 	return s.publisher
// }

// // responseEventPublisher implementation
// type responseEventPublisher struct {
// 	stream *responseEventStream
// }

// func (p *responseEventPublisher) Send(ctx context.Context, event *dive.ResponseEvent) error {
// 	p.stream.mu.Lock()
// 	closed := p.stream.closed
// 	p.stream.mu.Unlock()

// 	if closed {
// 		return dive.ErrStreamClosed
// 	}

// 	select {
// 	case <-ctx.Done():
// 		return ctx.Err()
// 	case p.stream.events <- event:
// 		return nil
// 	}
// }

// func (p *responseEventPublisher) Close() {
// 	p.stream.Close()
// }

type callbackPublisher struct {
	callback dive.EventCallback
}

func (p *callbackPublisher) Send(ctx context.Context, event *dive.ResponseEvent) error {
	return p.callback(ctx, event)
}

func (p *callbackPublisher) Close() {}

type nullEventPublisher struct{}

func (p *nullEventPublisher) Send(ctx context.Context, event *dive.ResponseEvent) error {
	return nil
}

func (p *nullEventPublisher) Close() {}
