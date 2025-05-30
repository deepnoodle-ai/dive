package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
)

type ServerSentEventsReader[T any] struct {
	body   io.ReadCloser
	reader *bufio.Reader
	err    error
}

func NewServerSentEventsReader[T any](stream io.ReadCloser) *ServerSentEventsReader[T] {
	return &ServerSentEventsReader[T]{
		body:   stream,
		reader: bufio.NewReader(stream),
	}
}

func (s *ServerSentEventsReader[T]) Err() error {
	return s.err
}

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
