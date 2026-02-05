package dive

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/deepnoodle-ai/dive/llm"
)

// CreateResponseOptions contains configuration for LLM generations.
//
// This struct holds all the options that can be passed to Agent.CreateResponse.
// Options are typically set using the With* functions rather than directly
// modifying this struct.
type CreateResponseOptions struct {
	// Messages contains the input messages for this generation. These are
	// appended to any existing session messages before sending to the LLM.
	Messages []*llm.Message

	// EventCallback is invoked for each response item during generation.
	// Callbacks include messages, tool calls, and tool results.
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

// randomInt returns a random integer as a string
func randomInt() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%d", n)
}
