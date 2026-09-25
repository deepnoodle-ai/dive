package dive

import (
	"context"
	"errors"
	"io"

	"github.com/deepnoodle-ai/dive/llm"
)

// Turn is the record of one turn as the invocation that returned it left it:
// the unit a session saves and a stateless caller appends to its history. It
// is set on Response.Turn for every status once the turn has begun, and on
// the terminal ResponseItemTypeTurnEnded item.
type Turn struct {
	// Messages is what a session saves for this invocation. For a fresh
	// turn it is the input and the output, including a synthetic
	// background-results message, which OutputMessages does not carry. For
	// a resume it is the whole suspended turn merged with this invocation's
	// output, since the session replaces the suspended event.
	Messages []*llm.Message `json:"messages"`

	// Usage is the usage of this invocation's model calls. It is never nil.
	Usage *llm.Usage `json:"usage,omitempty"`

	// Outcome is set when Status is ResponseStatusIncomplete.
	Outcome *TurnOutcome `json:"outcome,omitempty"`

	// Suspension is set when Status is ResponseStatusSuspended. It is the
	// same value as Response.Suspension.
	Suspension *SuspensionState `json:"suspension,omitempty"`

	// Persistence says whether a session recorded this state.
	Persistence PersistenceState `json:"persistence"`
}

// PersistenceState says whether a session recorded a turn.
type PersistenceState string

const (
	// PersistenceNone means nothing was saved: there is no session, the
	// session does not store this kind of state (a suspension on a session
	// that is not a SuspendableSession), or the turn is incomplete.
	PersistenceNone PersistenceState = "none"

	// PersistenceSaved means the session acknowledged the write.
	PersistenceSaved PersistenceState = "saved"

	// PersistenceUnknown means the write returned an error. The write may
	// still have landed, as when a file store fails after replacing the
	// file, so the caller reloads the session before acting on it.
	PersistenceUnknown PersistenceState = "unknown"
)

// TurnOutcome says how an incomplete turn stopped and what can continue it.
// It is set on Response.Turn.Outcome.
type TurnOutcome struct {
	// Reason says what stopped the turn.
	Reason TurnReason `json:"reason"`

	// Error is the error that ended the invocation, as text.
	Error string `json:"error,omitempty"`

	// Hook is the hook type ("PreToolUse", "Stop", ...) for
	// TurnReasonHookAbort.
	Hook string `json:"hook,omitempty"`

	// ToolCalls records every call of the batch in flight when the turn
	// stopped, in call order, with what is known about it. Empty when the
	// turn stopped between batches.
	ToolCalls []ToolCallRecord `json:"tool_calls,omitempty"`

	// Next says what the turn needs: another model call on the record as it
	// stands, reconciliation of a call with an unknown result first, or new
	// input because there is nothing to continue. Advisory: the application
	// decides.
	Next TurnNext `json:"next"`
}

// TurnReason says what stopped an incomplete turn.
type TurnReason string

const (
	// TurnReasonCanceled: the context was cancelled. The error wraps
	// context.Canceled.
	TurnReasonCanceled TurnReason = "canceled"

	// TurnReasonDeadline: the context's deadline passed, including
	// AgentOptions.ResponseTimeout. The error wraps context.DeadlineExceeded.
	TurnReasonDeadline TurnReason = "deadline"

	// TurnReasonProviderError: a model call failed, after the provider's
	// own retries, before its response began to stream.
	TurnReasonProviderError TurnReason = "provider_error"

	// TurnReasonStreamInterrupted: a model response stopped streaming
	// before it finished.
	TurnReasonStreamInterrupted TurnReason = "stream_interrupted"

	// TurnReasonHookAbort: a hook returned a *HookAbortError.
	TurnReasonHookAbort TurnReason = "hook_abort"

	// TurnReasonCallbackError: the event callback returned an error.
	TurnReasonCallbackError TurnReason = "callback_error"

	// TurnReasonError: any other error; TurnOutcome.Error says which.
	TurnReasonError TurnReason = "error"
)

// TurnNext says what an incomplete turn needs next.
type TurnNext string

const (
	// TurnNextContinue: another model call on the history as it stands.
	TurnNextContinue TurnNext = "continue"

	// TurnNextReconcile: a call with an unknown result must be checked
	// before the model is called again.
	TurnNextReconcile TurnNext = "reconcile"

	// TurnNextInput: nothing to continue; the next move is the user's.
	TurnNextInput TurnNext = "input"
)

// ToolCallRecord is what is known about one call of the batch in flight when
// a turn stopped.
type ToolCallRecord struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	State ToolCallState `json:"state"`
}

// ToolCallState is what is known about a tool call when its turn stopped.
type ToolCallState string

const (
	// ToolCallStateCompleted: a result is recorded, success or tool error.
	ToolCallStateCompleted ToolCallState = "completed"

	// ToolCallStateNotStarted: the call never started; it had no effect.
	ToolCallStateNotStarted ToolCallState = "not_started"

	// ToolCallStateUnknown: the call started and no result was recorded; it
	// may have taken effect.
	ToolCallStateUnknown ToolCallState = "unknown"
)

// failureOutcome classifies the error that ended an invocation. A context
// error wins, since whatever failed may have failed because of it; then a
// hook abort, the event callback, and the model call, identified by the
// errors the turn record saw them return.
func failureOutcome(err error, record *turnRecord) *TurnOutcome {
	outcome := &TurnOutcome{Reason: TurnReasonError, Error: err.Error()}
	var abortErr *HookAbortError
	record.mu.Lock()
	callbackErr, modelErr, streamStarted := record.callbackErr, record.modelErr, record.modelStreamStarted
	record.mu.Unlock()
	switch {
	case errors.Is(err, context.Canceled):
		outcome.Reason = TurnReasonCanceled
	case errors.Is(err, context.DeadlineExceeded):
		outcome.Reason = TurnReasonDeadline
	case errors.As(err, &abortErr):
		outcome.Reason = TurnReasonHookAbort
		outcome.Hook = abortErr.HookType
	case callbackErr != nil && errors.Is(err, callbackErr):
		outcome.Reason = TurnReasonCallbackError
	case modelErr != nil && errors.Is(err, modelErr):
		outcome.Reason = TurnReasonProviderError
		if streamStarted || errors.Is(err, io.ErrUnexpectedEOF) {
			outcome.Reason = TurnReasonStreamInterrupted
		}
	}
	outcome.Next = defaultNext(outcome.Reason)
	return outcome
}

// defaultNext is the Next an outcome with this reason advises.
func defaultNext(reason TurnReason) TurnNext {
	switch reason {
	case TurnReasonDeadline, TurnReasonProviderError, TurnReasonStreamInterrupted, TurnReasonCallbackError:
		return TurnNextContinue
	default:
		return TurnNextInput
	}
}
