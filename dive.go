package dive

import (
	"context"
	"errors"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
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

// SuspendableSession is an optional extension of Session. Sessions that
// implement it participate in suspend/resume. Sessions that do not implement
// it cannot be resumed — any tool returning SuspendResult against such a
// session causes CreateResponse to return an error.
type SuspendableSession interface {
	Session

	// Suspended reports whether the session is currently awaiting external
	// tool results.
	Suspended() bool

	// PendingCalls returns the tool calls awaiting external results,
	// including the Prompt and Metadata the tool attached to its
	// SuspendResult. Only meaningful when Suspended() is true.
	PendingCalls() []session.PendingCall

	// LastEventMessageCount returns the number of messages in the most
	// recent event, or 0 if the session has no events. The agent uses this
	// to compute the boundary between the suspended turn and the rest of
	// the conversation history during resume.
	LastEventMessageCount() int

	// SaveSuspendedTurn persists a partial turn whose final tool_result message
	// is missing one or more tool_result blocks. Sets Suspended=true.
	SaveSuspendedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage, pending []session.PendingCall) error

	// SaveResumedTurn replaces the last (suspended) event with a complete turn
	// and clears the suspended flag. Implementations must return an error
	// (typically session.ErrNotSuspended) if the session is not currently
	// suspended.
	SaveResumedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error

	// AbandonSuspension marks the session as no longer suspended and clears
	// the pending set. Used when resume completes without running generate,
	// or as a rollback path when OnSuspend hooks abort after the suspend was
	// already persisted.
	AbandonSuspension(ctx context.Context) error
}

// ErrNoSuspendedState is returned from CreateResponse when WithToolResults is
// supplied but the session is not in a suspended state.
var ErrNoSuspendedState = errors.New("dive: session is not suspended")

// ErrUnknownPendingToolCall is returned when WithToolResults contains an ID
// that is not in the session's pending set.
var ErrUnknownPendingToolCall = errors.New("dive: unknown pending tool call id")

// ErrSessionNotSuspendable is returned when a tool returns a SuspendResult but
// the session does not implement SuspendableSession.
var ErrSessionNotSuspendable = errors.New("dive: session does not implement SuspendableSession; suspend/resume unavailable")

// ErrSuspendedSessionInput is returned when CreateResponse is called with new
// user input on a suspended session. Callers must use WithToolResults to
// resume the suspended turn first.
var ErrSuspendedSessionInput = errors.New("dive: session is suspended; use WithToolResults to resume")

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

	// ToolResults, when non-nil, indicates this is a resume call. Keys are
	// tool_call IDs from a prior suspended Response's PendingToolCalls; values
	// are the results the caller obtained out-of-band.
	ToolResults map[string]*ToolResult
}

// EventCallback is a function called with each item produced while an agent
// is using tools or generating a response. Callbacks may be invoked
// concurrently from multiple goroutines (e.g. parallel tool calls, tool
// streaming) — implementations must be safe for concurrent use.
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

// WithToolResults supplies externally-obtained tool results to resume a
// previously suspended agent. The keys are tool_call IDs taken from a prior
// Response.PendingToolCalls. The values are caller-constructed ToolResults;
// an IsError result flows through the PostToolUseFailure path as if the tool
// itself had failed.
//
// If the caller supplies results for only a subset of pending IDs, the agent
// stays suspended and returns a new suspended Response listing the remaining
// pending calls. If any supplied ID is not in the pending set, CreateResponse
// returns ErrUnknownPendingToolCall without mutating session state.
//
// Resume is not safe to call concurrently on the same session.
func WithToolResults(results map[string]*ToolResult) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.ToolResults = results
	}
}
