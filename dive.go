package dive

import (
	"context"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
	"github.com/gofrs/uuid/v5"
)

// OutputFormat defines the desired output format for a Task
type OutputFormat string

const (
	OutputFormatText     OutputFormat = "text"
	OutputFormatMarkdown OutputFormat = "markdown"
	OutputFormatJSON     OutputFormat = "json"
)

// Type aliases for easy access to LLM types
type (
	Schema   = schema.Schema
	Property = schema.Property
)

// Agent represents an intelligent AI entity that can autonomously execute tasks,
// respond to chat messages, and process information. Agents can work with documents,
// generate responses using LLMs, and interact with users through natural language.
// They may have specialized capabilities depending on their implementation.
type Agent interface {

	// Name of the Agent
	Name() string

	// IsSupervisor indicates whether the Agent can assign work to other Agents
	IsSupervisor() bool

	// SetEnvironment sets the runtime Environment to which this Agent belongs
	SetEnvironment(env Environment) error

	// CreateResponse creates a new Response from the Agent
	CreateResponse(ctx context.Context, opts ...Option) (*Response, error)

	// StreamResponse streams a new Response from the Agent
	StreamResponse(ctx context.Context, opts ...Option) (ResponseStream, error)
}

// Environment is a container for running Agents and Workflows. Interactivity
// between Agents is generally scoped to a single Environment.
type Environment interface {

	// Name of the Environment
	Name() string

	// Agents returns the list of all Agents belonging to this Environment
	Agents() []Agent

	// AddAgent adds an Agent to this Environment
	AddAgent(agent Agent) error

	// GetAgent returns the Agent with the given name, if found
	GetAgent(name string) (Agent, error)

	// DocumentRepository returns the DocumentRepository for this Environment
	DocumentRepository() DocumentRepository

	// ThreadRepository returns the ThreadRepository for this Environment
	ThreadRepository() ThreadRepository

	// Confirmer returns the Confirmer for this Environment
	Confirmer() Confirmer
}

// Options contains configuration for LLM generations.
type Options struct {
	ThreadID      string
	UserID        string
	Messages      []*llm.Message
	EventCallback EventCallback
}

// EventCallback is a function that processes streaming events during response generation.
type EventCallback func(ctx context.Context, event *ResponseEvent) error

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
// event generated during response creation.
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
