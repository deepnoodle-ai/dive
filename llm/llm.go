package llm

import (
	"context"
)

type LLM interface {
	// Name of the LLM model
	Name() string

	// Generate a response from the LLM by passing messages.
	Generate(ctx context.Context, messages []*Message, opts ...Option) (*Response, error)
}

type StreamingLLM interface {
	LLM

	// Stream a response from the LLM by passing messages.
	Stream(ctx context.Context, messages []*Message, opts ...Option) (Stream, error)
}
