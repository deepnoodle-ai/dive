package llm

import (
	"context"
)

type LLM interface {
	// Name of the LLM provider
	Name() string

	// Generate a response from the LLM by passing messages.
	Generate(ctx context.Context, opts ...Option) (*Response, error)
}

type StreamingLLM interface {
	LLM

	// Stream starts a streaming response from the LLM by passing messages.
	// The caller should call Close on the returned Stream when done.
	Stream(ctx context.Context, opts ...Option) (StreamIterator, error)
}

type StreamIterator interface {
	// Next advances the stream to the next event. It returns false when the stream
	// is complete or if an error occurs. The caller should check Err() after Next
	// returns false to distinguish between normal completion and errors.
	Next() bool

	// Event returns the current event in the stream. It should only be called
	// after a successful call to Next.
	Event() *Event

	// Err returns any error that occurred while reading from the stream.
	// It should be checked after Next returns false.
	Err() error

	// Close closes the stream and releases any associated resources.
	Close() error
}
