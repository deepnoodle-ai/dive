package dive

import (
	"context"
	"errors"

	"github.com/deepnoodle-ai/dive/llm"
)

// Session provides persistent conversation state across multiple turns.
// The agent calls Messages before generation to load history, and SaveTurn
// after generation to persist new messages.
//
// Agents are stateless by default. Setting Session on AgentOptions or passing
// WithSession per-call enables automatic history loading and saving.
//
// # Concurrency
//
// Agent.CreateResponse serializes calls that share a session ID using an
// in-process per-ID lock, so two goroutines (or two agents) calling
// CreateResponse on the same session will run one after the other rather
// than interleaving their Messages() reads and SaveTurn writes. This is a
// correctness guarantee — implementations are not expected to coordinate
// concurrent access themselves. It is an in-process guarantee only; for
// multi-process deployments that share a session, use a backend with its
// own serialization (e.g. a database with row locks) rather than
// FileStore, which is single-writer-per-session by design.
type Session interface {
	// ID returns a unique identifier for this session.
	ID() string

	// Messages returns the conversation history.
	Messages(ctx context.Context) ([]*llm.Message, error)

	// SaveTurn persists messages from a single turn.
	SaveTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error
}

// SuspendableSession is an optional extension of Session for callers who
// want suspend/resume state to be auto-persisted alongside the conversation
// history. A plain Session — or no session at all — also supports
// suspend/resume; in that case the caller manages the SuspensionState
// themselves via Response.Suspension and WithResume.
type SuspendableSession interface {
	Session

	// LoadSuspension returns the stored suspension state for this session,
	// or nil if the session is not currently suspended. The returned value
	// is a deep copy — mutations do not affect session internals.
	LoadSuspension() *SuspensionState

	// SaveSuspendedTurn persists a partial turn whose final tool_result
	// message is missing one or more tool_result blocks, together with the
	// SuspensionState that describes the pending work. Implementations
	// should store the state so a subsequent LoadSuspension returns an
	// equivalent value.
	SaveSuspendedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage, state *SuspensionState) error

	// SaveResumedTurn replaces the last (suspended) event with a completed
	// turn and clears the stored suspension state. Implementations should
	// return an error if the session is not currently suspended.
	SaveResumedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error

	// CancelSuspension abandons a suspended turn, clearing the suspension
	// state and removing the partial turn from the session history.
	// Returns ErrNotSuspended if the session is not currently suspended.
	// After cancellation, the session is ready for a fresh turn as if the
	// suspended turn never happened.
	CancelSuspension(ctx context.Context) error
}

// ErrNoSuspendedTurn is returned from CreateResponse when WithResume or
// WithToolResults is supplied but there is no suspended turn to resume
// (neither the session nor the options carry one).
var ErrNoSuspendedTurn = errors.New("dive: no suspended turn to resume")

// ErrUnknownPendingToolCall is returned when a resume call supplies tool
// results for an ID that is not in the pending set.
var ErrUnknownPendingToolCall = errors.New("dive: unknown pending tool call id")

// ErrInputOnSuspendedSession is returned when CreateResponse is called with
// new user input on a session-backed suspended session. Resume the turn
// first (via WithResume / WithToolResults); new input belongs in a fresh
// turn after the suspended one resolves.
var ErrInputOnSuspendedSession = errors.New("dive: session is suspended; resume the current turn before supplying new input")

// ErrResumeRequired is returned when CreateResponse is called on a suspended
// session without any explicit opt-in (no WithResume, no WithToolResults,
// no new input). Resume is explicit — the agent does not silently re-save
// an idle suspended turn.
var ErrResumeRequired = errors.New("dive: session is suspended; pass WithResume or WithToolResults to continue the turn")

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

	// ToolResults, when non-nil, supplies externally-obtained tool results
	// to a resume call. Keys are tool_call IDs from a prior suspended
	// Response's Suspension.PendingToolCalls; values are the results the
	// caller obtained out-of-band. Typically set via WithResume (stateless)
	// or WithToolResults (session-backed).
	ToolResults map[string]*ToolResult

	// Suspension, when non-nil, supplies a SuspensionState from a prior
	// suspended Response, used by stateless callers to resume a turn
	// without a SuspendableSession. The agent splices
	// Suspension.TurnMessages onto the pre-turn history passed via
	// WithMessages to reconstruct the full conversation. When a
	// SuspendableSession is in use this option overrides the session's
	// stored state (useful for cross-process handoff where the resumer
	// holds a newer snapshot than what was persisted). Typically set via
	// WithResume rather than assigned directly.
	Suspension *SuspensionState
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

// sessionCtxKey is the context-value key used to thread the active Session
// through CreateResponse so downstream LLM hooks (which receive
// llm.HookContext, not dive.HookContext) can correlate work to a chat
// thread without bespoke wiring.
type sessionCtxKey struct{}

// SessionFromContext returns the Session attached to ctx by CreateResponse,
// or nil if none. Useful for provider-level llm.Hooks that need the
// conversation ID — the agent installs the active session on ctx before
// invoking the model, so the same hook works for both stateful and
// stateless calls (returning nil in the stateless case).
func SessionFromContext(ctx context.Context) Session {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(sessionCtxKey{}).(Session)
	return s
}

// withSessionContext returns a derived ctx carrying sess. Internal — used by
// the agent to publish the active session to downstream LLM hooks.
func withSessionContext(ctx context.Context, sess Session) context.Context {
	if sess == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionCtxKey{}, sess)
}

// WithToolResults supplies externally-obtained tool results to resume a
// session-backed suspended agent. The keys are tool_call IDs taken from a
// prior Response.Suspension.PendingToolCalls. The values are
// caller-constructed ToolResults; an IsError result flows through the
// PostToolUseFailure path as if the tool itself had failed.
//
// Use WithToolResults when the agent has a SuspendableSession — the session
// supplies the SuspensionState automatically. Stateless callers, or callers
// doing cross-process handoff, should use WithResume instead (which bundles
// both the state and the tool results).
//
// If the caller supplies results for only a subset of pending IDs, the agent
// stays suspended and returns a new suspended Response listing the remaining
// pending calls. If any supplied ID is not in the pending set, CreateResponse
// returns ErrUnknownPendingToolCall without mutating session state.
//
// Concurrent CreateResponse calls on the same session (resume or otherwise)
// are serialized automatically by an in-process per-session lock keyed on
// Session.ID() — see the Session interface documentation.
func WithToolResults(results map[string]*ToolResult) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.ToolResults = results
	}
}

// WithResume supplies a prior SuspensionState together with the tool results
// that satisfy its pending calls, bundling the two values a stateless
// resume requires into a single option. The caller still passes their
// pre-turn history via WithMessages; the agent splices
// state.TurnMessages onto that history to reconstruct the full
// conversation.
//
// Partial resumes are allowed: pass results for a subset of the pending
// calls and the agent returns a new suspended Response listing the
// remaining pending. Pass a nil map of results to re-submit the state
// without advancing (rare — useful for cross-process handoff where the
// resumer just wants the agent to take authority over a newer snapshot).
//
// When a SuspendableSession is in use, WithResume overrides the session's
// stored state — useful when the caller holds a more recent snapshot.
// Session-backed callers who simply want to resume from the session's
// stored state should use WithToolResults instead.
//
// Supplying an ID that is not in the pending set returns
// ErrUnknownPendingToolCall without mutating session state.
func WithResume(state *SuspensionState, results map[string]*ToolResult) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.Suspension = state
		opts.ToolResults = results
	}
}
