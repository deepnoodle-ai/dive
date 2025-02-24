package llm

import (
	"context"
)

type LLM interface {
	// Generate a response from the LLM by passing messages.
	Generate(ctx context.Context, messages []*Message, opts ...Option) (*Response, error)

	// Stream a response from the LLM by passing messages.
	Stream(ctx context.Context, messages []*Message, opts ...Option) (Stream, error)

	// SupportsStreaming returns true if the LLM supports streaming.
	SupportsStreaming() bool
}
