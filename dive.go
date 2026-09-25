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

	// SaveTurn persists messages from a single turn. An incomplete turn is
	// saved the same way; its last message records the outcome (see
	// FindTurnOutcome). The agent calls it on a context without the
	// cancellation that may have ended the turn, bounded by
	// IncompleteTurnOptions.SaveTimeout. An error that wraps ErrSaveRejected
	// says nothing was written.
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

	// SaveResumedTurn replaces the last (suspended) event with the resumed
	// turn, completed or closed incomplete, and clears the stored suspension
	// state. Implementations should return an error if the session is not
	// currently suspended.
	SaveResumedTurn(ctx context.Context, messages []*llm.Message, usage *llm.Usage) error

	// CancelSuspension abandons a suspended turn, clearing the suspension
	// state and removing the partial turn from the session history.
	// Returns ErrNotSuspended if the session is not currently suspended.
	// After cancellation, the session is ready for a fresh turn as if the
	// suspended turn never happened.
	CancelSuspension(ctx context.Context) error
}

// TurnStore is an optional Session extension that stores turn records. The
// agent loads the history and the open turn from it at the start of every
// invocation (Load), and records the turn at the end (CheckpointTurn) in
// place of SaveTurn, SaveSuspendedTurn and SaveResumedTurn. An incomplete
// turn stays open, and a WithContinue invocation folds into it instead of
// adding a turn of its own. session.Session implements it.
//
// Load takes a context and returns an error, unlike LoadSuspension, so a
// remote store can implement it.
type TurnStore interface {
	Session

	// Load returns the history needed to build context, the open turn if
	// any (suspended or incomplete), and the session revision a later
	// checkpoint must carry.
	Load(ctx context.Context) (*SessionSnapshot, error)

	// CheckpointTurn records the turn's state and returns the new session
	// revision. A turn whose ID is the open turn's replaces it; any other
	// turn is added after it. It fails with an error that wraps
	// ErrRevisionConflict when the stored revision is not expectedRevision,
	// and the caller reloads. The revision is the session's, advanced by
	// every write (a checkpoint, a save, a compaction, a removal), so a
	// checkpoint fails when anything changed since the load. The store
	// keeps its own copy of turn.
	//
	// It is also how a caller imports a turn it holds, such as a
	// Response.Turn from another process, with the same conflict check.
	CheckpointTurn(ctx context.Context, expectedRevision uint64, turn *Turn) (uint64, error)
}

// SessionSnapshot is what TurnStore.Load returns.
type SessionSnapshot struct {
	// History is the conversation before the open turn: every completed
	// turn, and any incomplete turn later turns superseded, as the model
	// sees them.
	History []*llm.Message

	// OpenTurn is the last turn when it is suspended or incomplete, and nil
	// when it completed. Its Messages are closed; an incomplete turn's end
	// with the outcome reminder.
	OpenTurn *Turn

	// Revision is the session revision the snapshot was read at.
	Revision uint64
}

// ErrRevisionConflict is wrapped by the error of a TurnStore checkpoint, or of
// a ResumeRequest, whose expected revision or turn is no longer the stored
// one: the session changed since the caller read it. Reload and retry. A
// checkpoint refused with it wrote nothing: errors.Is(err, ErrSaveRejected)
// holds.
var ErrRevisionConflict error = &revisionConflictError{}

type revisionConflictError struct{}

func (*revisionConflictError) Error() string { return "dive: session revision conflict" }

func (*revisionConflictError) Is(target error) bool { return target == ErrSaveRejected }

// ErrConflictingToolResult is returned when a resume supplies a result for a
// call that already has a different one. It also matches
// ErrUnknownPendingToolCall, since the call is not pending.
var ErrConflictingToolResult = errors.New("dive: a different result was already accepted for this tool call")

// ErrUnreconciledToolCalls is returned when new input is given on a session
// whose last turn stopped with a call whose result is unknown (TurnOutcome.Next
// is TurnNextReconcile), and IncompleteTurnOptions.RequireReconcile is set.
// Continue the turn (WithContinue) or remove it first.
var ErrUnreconciledToolCalls = errors.New("dive: the last turn has tool calls with unknown results; continue it before giving new input")

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

// ErrSessionNotSuspended is returned when WithResume supplies an explicit
// SuspensionState but the attached SuspendableSession is not currently
// suspended. The completed resume would be persisted via SaveResumedTurn,
// which fails on a non-suspended session — so the mismatch is detected
// before generation rather than after tokens have been spent.
var ErrSessionNotSuspended = errors.New("dive: WithResume supplied but the attached session has no suspended turn to resume")

// CreateResponseOptions contains configuration for LLM generations.
//
// This struct holds all the options that can be passed to Agent.CreateResponse.
// Options are typically set using the With* functions rather than directly
// modifying this struct.
type CreateResponseOptions struct {
	// Messages contains the input messages for this generation. These are
	// appended to any existing session messages before sending to the LLM.
	Messages []*llm.Message

	// ModelOnlyReminders are appended at the conversation tail for this request
	// but excluded from OutputMessages and session persistence.
	ModelOnlyReminders []Reminder

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

	// PromptCacheKey routes requests from the same conversation to the same
	// provider prompt cache. When empty, the agent derives one from the active
	// Session or falls back to a key that is stable for this Agent instance.
	PromptCacheKey string

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

	// BackgroundHandles and BackgroundResults, when both non-nil, inject
	// completed background task results as a synthetic user message at the
	// start of the next generation. Set via WithBackgroundResults.
	BackgroundHandles []*BackgroundTaskHandle
	BackgroundResults map[string]*ToolResult

	// Continue asks for another invocation of the conversation as recorded,
	// with no new input. Set via WithContinue.
	Continue bool

	// ResumeTurnID and ExpectedRevision, when set, are checked against the
	// session's open turn and revision before a resume. Set via
	// WithResumeRequest.
	ResumeTurnID     string
	ExpectedRevision uint64

	// cancelSuspended closes the suspended turn without a model call. Set by
	// Agent.CancelSuspendedTurn.
	cancelSuspended bool
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

// WithModelOnlyReminder appends runtime context for this CreateResponse without
// including it in OutputMessages or session persistence.
func WithModelOnlyReminder(reminder Reminder) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.ModelOnlyReminders = append(opts.ModelOnlyReminders, reminder)
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

// WithPromptCacheKey sets the provider prompt-cache routing key for this call.
// Stateless applications that share one Agent across conversations should pass
// a stable, distinct key for each conversation. Session-backed agents derive a
// private key from Session.ID automatically.
func WithPromptCacheKey(key string) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.PromptCacheKey = key
	}
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

// ErrContinueWithInput is returned when WithContinue is combined with new
// input on a session, or with a resume.
var ErrContinueWithInput = errors.New("dive: WithContinue takes no new input and cannot resume a suspended turn")

// WithContinue asks for another invocation of the conversation as recorded,
// with no new input: the model is called on the history as it stands. It is
// how a stopped, failed or cut-off turn is picked up without rerunning any
// tool, since every tool result is already in the record. A "turn-continue"
// reminder says the user asked to continue from where the turn stopped:
// recorded before the new output when the history ends in an assistant
// message, so the request never ends in an assistant turn, which some models
// reject as a prefill, and model-only when it ends in an incomplete turn's
// outcome reminder.
//
// On a TurnStore session whose last turn is incomplete, the continuation
// folds into that turn: the model sees it without its outcome reminder, the
// turn keeps its ID, and it is saved again with the new output, so
// Response.Turn.Messages is the whole turn. On any other session, pass it
// without input: the continuation is saved as its own turn, and
// Response.Turn.Messages is its reminder, if recorded, and its output. A
// stateless caller passes its history with WithMessages; the messages are
// history, not input, and Response.Turn.Messages is appended to them. It returns ErrResumeRequired on a suspended
// session, which must be resumed first, ErrContinueWithInput with input on a
// session or with a resume, and an error when there is no history at all.
func WithContinue() CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.Continue = true
	}
}

// ResumeRequest resumes a suspended turn on a TurnStore session, naming the
// turn and the session revision the caller read it at, with the results it
// supplies. A stale turn or revision fails with ErrRevisionConflict before
// anything runs; the caller reloads the session and decides again.
type ResumeRequest struct {
	// TurnID is the suspended turn's Turn.ID (SuspensionState.TurnID).
	// Empty skips the check.
	TurnID string

	// ExpectedRevision is the session revision the caller read the
	// suspension at (Turn.Revision, SessionSnapshot.Revision). Zero skips
	// the check.
	ExpectedRevision uint64

	// ToolResults are the results for pending calls, as for
	// WithToolResults.
	ToolResults map[string]*ToolResult
}

// WithResumeRequest resumes the session's suspended turn with req's results,
// after checking that it is still the turn and revision the caller read. It
// needs a session that implements TurnStore. Supplying a result a previous
// resume already accepted is not an error, so a resume whose outcome was
// unknown can be sent again as it was.
func WithResumeRequest(req ResumeRequest) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.ToolResults = req.ToolResults
		opts.ResumeTurnID = req.TurnID
		opts.ExpectedRevision = req.ExpectedRevision
	}
}
