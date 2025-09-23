package dive

import (
	"context"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/gofrs/uuid/v5"
)

// Type aliases for easy access to LLM types
type (
	Schema   = schema.Schema
	Property = schema.Property
)

// Agent represents an intelligent AI entity that can autonomously use tools to
// process information while responding to chat messages.
type Agent interface {

	// Name of the Agent
	Name() string

	// CreateResponse creates a new Response from the Agent
	CreateResponse(ctx context.Context, opts ...Option) (*Response, error)
}

// Options contains configuration for LLM generations.
type Options struct {
	ThreadID      string
	UserID        string
	Messages      []*llm.Message
	EventCallback EventCallback
}

// EventCallback is a function called with each item produced while an agent
// is using tools or generating a response.
type EventCallback func(ctx context.Context, item *ResponseItem) error

// Option is a type signature for defining new LLM generation options.
type Option func(*Options)

// Apply invokes any supplied options. Used internally in Dive.
func (o *Options) Apply(opts []Option) {
	for _, opt := range opts {
		opt(o)
	}
}

// WithThreadID associates the given conversation thread ID with a generation.
// This appends the new messages to any previous messages belonging to this thread.
func WithThreadID(threadID string) Option {
	return func(opts *Options) {
		opts.ThreadID = threadID
	}
}

// WithUserID associates the given user ID with a generation, indicating what
// person is the speaker in the conversation.
func WithUserID(userID string) Option {
	return func(opts *Options) {
		opts.UserID = userID
	}
}

// WithMessage specifies a single message to be used in the generation.
func WithMessage(message *llm.Message) Option {
	return func(opts *Options) {
		opts.Messages = []*llm.Message{message}
	}
}

// WithMessages specifies the messages to be used in the generation.
func WithMessages(messages ...*llm.Message) Option {
	return func(opts *Options) {
		opts.Messages = messages
	}
}

// WithInput specifies a simple text input string to be used in the generation.
// This is a convenience wrapper that creates a single user message.
func WithInput(input string) Option {
	return func(opts *Options) {
		opts.Messages = []*llm.Message{llm.NewUserTextMessage(input)}
	}
}

// WithEventCallback specifies a callback function that will be invoked for each
// item generated during response creation.
func WithEventCallback(callback EventCallback) Option {
	return func(opts *Options) {
		opts.EventCallback = callback
	}
}

// NewID returns a new UUID
func NewID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id.String()
}
