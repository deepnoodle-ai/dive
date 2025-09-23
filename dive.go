package dive

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
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
	CreateResponse(ctx context.Context, opts ...CreateResponseOption) (*Response, error)
}

// CreateResponseOptions contains configuration for LLM generations.
type CreateResponseOptions struct {
	ThreadID      string
	UserID        string
	Messages      []*llm.Message
	EventCallback EventCallback
}

// EventCallback is a function called with each item produced while an agent
// is using tools or generating a response.
type EventCallback func(ctx context.Context, item *ResponseItem) error

// CreateResponseOption is a type signature for defining new LLM generation options.
type CreateResponseOption func(*CreateResponseOptions)

// Apply invokes any supplied options. Used internally in Dive.
func (o *CreateResponseOptions) Apply(opts []CreateResponseOption) {
	for _, opt := range opts {
		opt(o)
	}
}

// WithThreadID associates the given conversation thread ID with a generation.
// This appends the new messages to any previous messages belonging to this thread.
func WithThreadID(threadID string) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.ThreadID = threadID
	}
}

// WithUserID associates the given user ID with a generation, indicating what
// person is the speaker in the conversation.
func WithUserID(userID string) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.UserID = userID
	}
}

// WithMessage specifies a single message to be used in the generation.
func WithMessage(message *llm.Message) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.Messages = []*llm.Message{message}
	}
}

// WithMessages specifies the messages to be used in the generation.
func WithMessages(messages ...*llm.Message) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.Messages = messages
	}
}

// WithInput specifies a simple text input string to be used in the generation.
// This is a convenience wrapper that creates a single user message.
func WithInput(input string) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.Messages = []*llm.Message{llm.NewUserTextMessage(input)}
	}
}

// WithEventCallback specifies a callback function that will be invoked for each
// item generated during response creation.
func WithEventCallback(callback EventCallback) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.EventCallback = callback
	}
}

// newID returns a new unique identifier with format "agent-<randomnum>"
func newID() string {
	return fmt.Sprintf("agent-%s", randomInt())
}

// randomInt returns a random integer as a string
func randomInt() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%d", n)
}
