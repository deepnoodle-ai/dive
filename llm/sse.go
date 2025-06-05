package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
)

// ServerSentEventsCallback is a callback that is called for each line of the
// server-sent events stream.
type ServerSentEventsCallback func(line string) error

// ServerSentEventsReader reads SSE events from a reader a decodes them into
// the parameter type T.
type ServerSentEventsReader[T any] struct {
	body        io.ReadCloser
	reader      *bufio.Reader
	err         error
	sseCallback ServerSentEventsCallback
}

// NewServerSentEventsReader creates a new ServerSentEventsReader.
func NewServerSentEventsReader[T any](stream io.ReadCloser) *ServerSentEventsReader[T] {
	return &ServerSentEventsReader[T]{
		body:   stream,
		reader: bufio.NewReader(stream),
	}
}

// WithSSECallback sets an optional callback that is called for each line of the
// server-sent events stream.
func (s *ServerSentEventsReader[T]) WithSSECallback(callback ServerSentEventsCallback) *ServerSentEventsReader[T] {
	s.sseCallback = callback
	return s
}

// Err returns the error that occurred while reading the SSE stream.
func (s *ServerSentEventsReader[T]) Err() error {
	return s.err
}

// Next reads the next event from the SSE stream.
func (s *ServerSentEventsReader[T]) Next() (T, bool) {
	var zero T
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				s.err = err
				return zero, false
			}
			return zero, false
		}

		// Fire callback if set
		if s.sseCallback != nil {
			if err := s.sseCallback(string(line)); err != nil {
				s.err = err
				return zero, false
			}
		}

		// Skip empty lines
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		// Remove "data: " prefix if present
		line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data: ")))

		// Check for stream end
		if bytes.Equal(line, []byte("[DONE]")) {
			return zero, false
		}

		// Skip non-JSON lines (like "event: " lines or other SSE metadata)
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}

		// Unmarshal then return the event
		var event T
		if err := json.Unmarshal(line, &event); err != nil {
			s.err = err
			return zero, false
		}
		return event, true
	}
}
