package dive

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/deepnoodle-ai/dive/llm"
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
//
// This struct holds all the options that can be passed to Agent.CreateResponse.
// Options are typically set using the With* functions rather than directly
// modifying this struct.
type CreateResponseOptions struct {
	// ThreadID is the conversation thread identifier. If empty, a new thread ID
	// will be auto-generated with the format "thread-<random>". Use the same
	// ThreadID across multiple calls to maintain conversation context.
	ThreadID string

	// UserID identifies the user in the conversation. This is stored with the
	// thread and can be used for multi-user scenarios.
	UserID string

	// Messages contains the input messages for this generation. These are
	// appended to any existing thread messages before sending to the LLM.
	Messages []*llm.Message

	// EventCallback is invoked for each response item during generation.
	// The first callback will always be an InitEvent containing the ThreadID.
	// Subsequent callbacks include messages, tool calls, and tool results.
	EventCallback EventCallback

	// Fork indicates whether to create a new thread branching from an existing
	// thread's history. When true and ThreadID references an existing thread,
	// a new thread is created with a copy of the original's messages.
	// The original thread remains unchanged.
	Fork bool

	// Compaction overrides the agent's compaction configuration for this request.
	// If nil, the agent's compaction configuration (if any) is used.
	Compaction *CompactionConfig
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
//
// When a ThreadRepository is configured on the agent, this enables multi-turn
// conversations by persisting and loading message history. If the thread exists,
// previous messages are loaded; if not, a new thread is created.
//
// If threadID is empty, a new thread ID will be auto-generated.
//
// Example:
//
//	// First message creates a new thread
//	resp1, _ := agent.CreateResponse(ctx,
//	    dive.WithThreadID("conversation-123"),
//	    dive.WithInput("Hello"),
//	)
//
//	// Second message continues the conversation
//	resp2, _ := agent.CreateResponse(ctx,
//	    dive.WithThreadID("conversation-123"),
//	    dive.WithInput("Tell me more"),
//	)
func WithThreadID(threadID string) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.ThreadID = threadID
	}
}

// WithResume resumes an existing conversation thread by ID.
//
// This is functionally identical to WithThreadID but expresses clearer intent
// when the caller knows they are resuming an existing conversation rather than
// potentially creating a new one.
//
// The thread's complete message history will be loaded from the ThreadRepository
// and new messages will be appended to continue the conversation.
//
// Example:
//
//	// Resume a previously saved conversation
//	resp, _ := agent.CreateResponse(ctx,
//	    dive.WithResume(savedThreadID),
//	    dive.WithInput("Continue where we left off"),
//	)
func WithResume(threadID string) CreateResponseOption {
	return WithThreadID(threadID)
}

// WithFork creates a new thread that branches from the resumed thread's history.
//
// When fork is true and used with WithThreadID or WithResume, a new thread is
// created containing a deep copy of all messages from the original thread.
// The original thread remains completely unchanged, allowing you to explore
// alternative conversation paths.
//
// This is useful for:
//   - Exploring different approaches from the same starting point
//   - Creating conversation branches for A/B testing
//   - Preserving a checkpoint while experimenting
//
// The forked thread receives a new auto-generated ID, which is returned in
// Response.ThreadID and emitted in the InitEvent callback.
//
// Example:
//
//	// Fork to try a different approach
//	resp, _ := agent.CreateResponse(ctx,
//	    dive.WithResume("original-thread"),
//	    dive.WithFork(true),
//	    dive.WithInput("Let's try a completely different approach"),
//	)
//	// resp.ThreadID is a new ID; "original-thread" is unchanged
func WithFork(fork bool) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.Fork = fork
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

// WithCompaction overrides the agent's compaction configuration for this request.
//
// Compaction automatically summarizes conversation history when token thresholds
// are exceeded, replacing the full message history with a concise summary.
// This helps manage context window limits for long-running conversations.
//
// Example:
//
//	resp, _ := agent.CreateResponse(ctx,
//	    dive.WithInput("Process all the files"),
//	    dive.WithCompaction(&dive.CompactionConfig{
//	        Enabled:               true,
//	        ContextTokenThreshold: 50000,
//	    }),
//	)
func WithCompaction(config *CompactionConfig) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.Compaction = config
	}
}

// newID returns a new unique identifier with format "agent-<randomnum>"
func newID() string {
	return fmt.Sprintf("agent-%s", randomInt())
}

// newThreadID returns a new unique thread identifier with format "thread-<randomnum>"
func newThreadID() string {
	return fmt.Sprintf("thread-%s", randomInt())
}

// randomInt returns a random integer as a string
func randomInt() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%d", n)
}
