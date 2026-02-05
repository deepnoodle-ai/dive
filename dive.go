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
	// SessionID is the conversation session identifier. If empty, a new session ID
	// will be auto-generated with the format "session-<random>". Use the same
	// SessionID across multiple calls to maintain conversation context.
	SessionID string

	// UserID identifies the user in the conversation. This is stored with the
	// session and can be used for multi-user scenarios.
	UserID string

	// Messages contains the input messages for this generation. These are
	// appended to any existing session messages before sending to the LLM.
	Messages []*llm.Message

	// EventCallback is invoked for each response item during generation.
	// The first callback will always be an InitEvent containing the SessionID.
	// Subsequent callbacks include messages, tool calls, and tool results.
	EventCallback EventCallback

	// Fork indicates whether to create a new session branching from an existing
	// session's history. When true and SessionID references an existing session,
	// a new session is created with a copy of the original's messages.
	// The original session remains unchanged.
	Fork bool
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

// WithSessionID associates the given conversation session ID with a generation.
//
// When a SessionRepository is configured on the agent, this enables multi-turn
// conversations by persisting and loading message history. If the session exists,
// previous messages are loaded; if not, a new session is created.
//
// If sessionID is empty, a new session ID will be auto-generated.
//
// Example:
//
//	// First message creates a new session
//	resp1, _ := agent.CreateResponse(ctx,
//	    dive.WithSessionID("conversation-123"),
//	    dive.WithInput("Hello"),
//	)
//
//	// Second message continues the conversation
//	resp2, _ := agent.CreateResponse(ctx,
//	    dive.WithSessionID("conversation-123"),
//	    dive.WithInput("Tell me more"),
//	)
func WithSessionID(sessionID string) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.SessionID = sessionID
	}
}

// WithResume resumes an existing conversation session by ID.
//
// This is functionally identical to WithSessionID but expresses clearer intent
// when the caller knows they are resuming an existing conversation rather than
// potentially creating a new one.
//
// The session's complete message history will be loaded from the SessionRepository
// and new messages will be appended to continue the conversation.
//
// Example:
//
//	// Resume a previously saved conversation
//	resp, _ := agent.CreateResponse(ctx,
//	    dive.WithResume(savedSessionID),
//	    dive.WithInput("Continue where we left off"),
//	)
func WithResume(sessionID string) CreateResponseOption {
	return WithSessionID(sessionID)
}

// WithFork creates a new session that branches from the resumed session's history.
//
// When fork is true and used with WithSessionID or WithResume, a new session is
// created containing a deep copy of all messages from the original session.
// The original session remains completely unchanged, allowing you to explore
// alternative conversation paths.
//
// This is useful for:
//   - Exploring different approaches from the same starting point
//   - Creating conversation branches for A/B testing
//   - Preserving a checkpoint while experimenting
//
// The forked session receives a new auto-generated ID, which is returned in
// Response.SessionID and emitted in the InitEvent callback.
//
// Example:
//
//	// Fork to try a different approach
//	resp, _ := agent.CreateResponse(ctx,
//	    dive.WithResume("original-session"),
//	    dive.WithFork(true),
//	    dive.WithInput("Let's try a completely different approach"),
//	)
//	// resp.SessionID is a new ID; "original-session" is unchanged
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

// newID returns a new unique identifier with format "agent-<randomnum>"
func newID() string {
	return fmt.Sprintf("agent-%s", randomInt())
}

// newSessionID returns a new unique session identifier with format "session-<randomnum>"
func newSessionID() string {
	return fmt.Sprintf("session-%s", randomInt())
}

// randomInt returns a random integer as a string
func randomInt() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%d", n)
}

// Session holds conversation state during generation.
// This is a minimal internal type. For persistent session storage,
// use PreGeneration/PostGeneration hooks with experimental/session.
type Session struct {
	// ID is the session identifier.
	ID string

	// Messages contains the conversation history.
	Messages []*llm.Message
}
