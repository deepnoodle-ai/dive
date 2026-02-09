package dive

import (
	"context"

	"github.com/deepnoodle-ai/dive/llm"
)

// Session provides persistent conversation state across multiple turns.
// The agent calls Messages before generation to load history, and SaveTurn
// after generation to persist new messages.
//
// Agents are stateless by default. Setting Session on AgentOptions or passing
// WithSession per-call enables automatic history loading and saving.
type Session interface {
	// ID returns a unique identifier for this session.
	ID() string

	// Messages returns the conversation history.
	Messages(ctx context.Context) ([]*llm.Message, error)

	// SaveTurn persists messages from a single turn.
	SaveTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error
}

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

	// Values contains arbitrary key-value pairs that are copied into
	// HookContext.Values before hooks run. This allows callers to pass
	// data to hooks (e.g. session IDs) through CreateResponse options.
	Values map[string]any

	// Session overrides AgentOptions.Session for this call.
	// Useful in server scenarios where one agent serves multiple sessions.
	Session Session
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

// WithValue sets a single key-value pair that will be available in
// HookContext.Values during generation. Multiple WithValue calls accumulate.
func WithValue(key string, value any) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		if opts.Values == nil {
			opts.Values = make(map[string]any)
		}
		opts.Values[key] = value
	}
}

// WithSession overrides the agent's default session for a single call.
// This is useful in server scenarios where one agent handles multiple sessions.
func WithSession(s Session) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.Session = s
	}
}
