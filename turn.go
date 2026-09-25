package dive

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

// turnRecord accumulates what one CreateResponse invocation produces after
// the turn boundary: the resume phase, every generation loop and every
// Stop-hook continuation feed the same record, and every exit reads it.
//
// Items are appended under the mutex because parallel tool goroutines emit
// events concurrently with the main goroutine. The other fields are written
// by the main goroutine only, and are also guarded so that a snapshot taken
// while a batch unwinds is consistent.
type turnRecord struct {
	mu sync.Mutex

	// output is every message the turn added to the conversation, in order:
	// assistant responses, tool results, recorded reminders and Stop-hook
	// continuation reminders.
	output []*llm.Message

	// items is every event emitted to the caller's callback, in order,
	// except the terminal suspended item.
	items []*ResponseItem

	// usage is the sum over every model call in the turn.
	usage *llm.Usage

	// modelCalled reports whether the turn has called the model. A turn
	// that ends before its first model call reports no usage.
	modelCalled bool

	// stopReason and stopDetails are those of the last model response.
	stopReason  string
	stopDetails *llm.StopDetails

	// backgroundTasks are the handles of background tasks the generation
	// loop started.
	backgroundTasks []*BackgroundTaskHandle

	// callbackErr is the last error the caller's event callback returned,
	// and modelErr the last error a model call returned, with
	// modelStreamStarted set when its stream had delivered an event first.
	// An exit's error is classified by whether it is one of them.
	callbackErr        error
	modelErr           error
	modelStreamStarted bool

	// version counts changes to the record, so an exit can tell whether
	// the response is behind it.
	version int
}

// newTurnRecord returns an empty record with zero usage.
func newTurnRecord() *turnRecord {
	return &turnRecord{usage: &llm.Usage{}}
}

// collecting returns a callback that records each item and then forwards it
// to callback.
func (r *turnRecord) collecting(callback EventCallback) EventCallback {
	return func(ctx context.Context, item *ResponseItem) error {
		r.mu.Lock()
		r.items = append(r.items, item)
		r.version++
		r.mu.Unlock()
		err := callback(ctx, item)
		if err != nil {
			r.mu.Lock()
			r.callbackErr = err
			r.mu.Unlock()
		}
		return err
	}
}

// addOutput records a message the turn added to the conversation.
func (r *turnRecord) addOutput(msg *llm.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.output = append(r.output, msg)
	r.version++
}

// outputLen returns the number of output messages recorded so far.
func (r *turnRecord) outputLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.output)
}

// outputSince returns the output messages recorded after the first n.
func (r *turnRecord) outputSince(n int) []*llm.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.output[n:])
}

// addModelResponse records a model response's usage and stop reason. The
// response's message is recorded separately with addOutput.
func (r *turnRecord) addModelResponse(response *llm.Response) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelCalled = true
	r.usage.Add(&response.Usage)
	r.stopReason, r.stopDetails = response.StopReason, response.StopDetails
	r.version++
}

// addModelError records the error a model call returned. streamStarted
// reports whether its stream had delivered an event before it failed.
func (r *turnRecord) addModelError(err error, streamStarted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelErr, r.modelStreamStarted = err, streamStarted
}

// addBackgroundTask records the handle of a background task the turn started.
func (r *turnRecord) addBackgroundTask(handle *BackgroundTaskHandle) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backgroundTasks = append(r.backgroundTasks, handle)
	r.version++
}

// turn is one CreateResponse invocation after the turn boundary, which lies
// immediately before the PreGeneration hooks. Every exit after the boundary
// goes through end.
type turn struct {
	agent    *Agent
	logger   llm.Logger
	hctx     *HookContext
	response *Response
	record   *turnRecord

	// emit is the caller's event callback. Items passed to it directly are
	// not recorded; the terminal suspended and turn_ended items are the only
	// such items.
	emit EventCallback

	// inputMessages is the turn's input: the caller's new messages and a
	// background-results message. Empty on a resume, whose input is rs.
	inputMessages []*llm.Message

	sess        Session
	suspendable SuspendableSession
	rs          *resumeState

	// partialResume is set when the caller resumed a suspended turn with
	// results for some of its pending calls only. The turn stays
	// suspended, so a failure returns no Response.
	partialResume bool

	// syncedVersion is the record version the response was last synced at.
	syncedVersion int
}

// turnExitKind says how an invocation ends.
type turnExitKind int

const (
	turnExitCompleted turnExitKind = iota
	turnExitSuspended
	turnExitFailed
)

// turnExit describes how an invocation ends; end acts on it.
type turnExit struct {
	kind turnExitKind

	// err is the failure of a failed exit.
	err error

	// suspension is the batch state of a suspended exit.
	suspension *suspendedSnapshot

	// continuesSuspension marks a partial resume: the turn was already
	// suspended and stays so, so OnSuspend hooks and the terminal suspended
	// item are skipped.
	continuesSuspension bool
}

// completedExit ends the invocation with a completed turn.
func completedExit() turnExit {
	return turnExit{kind: turnExitCompleted}
}

// suspendedExit ends the invocation with a suspended turn.
func suspendedExit(snap *suspendedSnapshot, continuesSuspension bool) turnExit {
	return turnExit{kind: turnExitSuspended, suspension: snap, continuesSuspension: continuesSuspension}
}

// failedExit ends the invocation on err.
func failedExit(err error) turnExit {
	return turnExit{kind: turnExitFailed, err: err}
}

// syncResponse copies the record onto the response. FinishedAt is set when
// finished is true or when it has not been set yet.
func (t *turn) syncResponse(finished bool) {
	r := t.record
	r.mu.Lock()
	defer r.mu.Unlock()
	if finished || t.response.FinishedAt == nil {
		t.response.FinishedAt = Ptr(time.Now())
	}
	t.response.Items = slices.Clone(r.items)
	t.response.OutputMessages = r.output
	if r.modelCalled {
		t.response.Usage = r.usage
	}
	t.response.StopReason = r.stopReason
	t.response.StopDetails = r.stopDetails
	t.syncedVersion = r.version
}

// end finishes the invocation: it builds the incomplete response for a
// failed exit, and runs the terminal hooks, saves the turn and builds the
// response for a completed or suspended one. A completed exit follows the
// Stop hooks, which may have edited the response, so it is not synced from
// the record again.
func (t *turn) end(ctx context.Context, exit turnExit) (*Response, error) {
	switch exit.kind {
	case turnExitFailed:
		return t.fail(ctx, exit.err)
	case turnExitSuspended:
		t.syncResponse(false)
		t.response.BackgroundTasks = t.record.backgroundTasks
		return t.finishSuspended(ctx, exit.suspension, exit.continuesSuspension)
	default:
		// The response was synced when the last generate call returned, and
		// the Stop hooks since then may have edited it: keep their edits.
		return t.finishCompleted(ctx)
	}
}

// fail ends the invocation on err. The response becomes incomplete, with an
// outcome classifying err, and is returned with err wrapped in a
// *GenerationError. A failed partial resume returns no response: the turn
// is still suspended.
func (t *turn) fail(ctx context.Context, err error) (*Response, error) {
	r := t.record
	r.mu.Lock()
	genErr := &GenerationError{
		Err:            err,
		Usage:          r.usage,
		OutputMessages: slices.Clone(r.output),
		Items:          slices.Clone(r.items),
	}
	stale := r.version != t.syncedVersion
	r.mu.Unlock()
	if t.partialResume {
		return nil, genErr
	}

	response := t.response
	if stale || response.FinishedAt == nil {
		t.syncResponse(true)
	}
	response.Status = ResponseStatusIncomplete
	response.Suspension = nil
	if len(r.backgroundTasks) > 0 {
		response.BackgroundTasks = r.backgroundTasks
	}
	response.Turn = &Turn{
		Messages:    t.turnMessages(),
		Usage:       r.usage.Copy(),
		Outcome:     failureOutcome(err, r),
		Persistence: PersistenceNone,
	}
	genErr.Response = response
	t.emitTurnEnded(ctx)
	return response, genErr
}

// turnMessages returns the messages a session saves for this invocation:
// the suspended turn and the output on a resume, else the input and the
// output.
func (t *turn) turnMessages() []*llm.Message {
	output := t.response.OutputMessages
	var prefix []*llm.Message
	if t.rs != nil {
		prefix = t.rs.TurnMessages
	} else {
		prefix = t.inputMessages
	}
	messages := make([]*llm.Message, 0, len(prefix)+len(output))
	messages = append(messages, prefix...)
	return append(messages, output...)
}

// emitTurnEnded emits the terminal turn_ended item. The state it reports is
// already decided, so the item goes out on a context that is not cancelled
// and a callback error is only logged.
func (t *turn) emitTurnEnded(ctx context.Context) {
	item := &ResponseItem{Type: ResponseItemTypeTurnEnded, Turn: t.response.Turn}
	if err := t.emit(context.WithoutCancel(ctx), item); err != nil {
		t.logger.Error("turn ended event callback error", "error", err)
	}
}

// saveError records a failed session write on a completed or suspended
// response and returns err.
func (t *turn) saveError(err error) (*Response, error) {
	t.logger.Error("session save error", "error", err)
	t.response.Turn.Persistence = PersistenceUnknown
	return t.response, err
}

// finishCompleted runs PostGeneration hooks and saves a completed turn.
func (t *turn) finishCompleted(ctx context.Context) (*Response, error) {
	a, logger, hctx, response := t.agent, t.logger, t.hctx, t.response

	hctx.Response = response
	hctx.OutputMessages = t.record.output
	hctx.Usage = t.record.usage
	for _, hook := range a.hooks.PostGeneration {
		if err := hook(ctx, hctx); err != nil {
			// Check if this is a fatal abort error
			var abortErr *HookAbortError
			if errors.As(err, &abortErr) {
				abortErr.HookType = "PostGeneration"
				logger.Error("post-generation hook aborted", "error", abortErr)
				return t.fail(ctx, abortErr)
			}
			// Regular errors are logged but don't affect the response
			logger.Error("post-generation hook error", "error", err)
		}
	}

	// Save session turn. On resume, replace the suspended event with the
	// combined turn (pre-suspend turn messages plus new output). Otherwise
	// append a new turn with input + output. A save error is returned with
	// the completed response, whose Turn.Persistence says the session may
	// not hold it.
	//
	// On a resume completion we also populate Response.Suspension with the
	// final merged turn snapshot (PendingToolCalls = nil) so stateless
	// callers can flush the turn into their local history in one append
	// without reconciling a stale partial tool_result from their saved
	// state.
	turnMsgs := t.turnMessages()
	response.Status = ResponseStatusCompleted
	if len(t.record.backgroundTasks) > 0 {
		response.BackgroundTasks = t.record.backgroundTasks
	}
	if rs := t.rs; rs != nil {
		response.Suspension = &SuspensionState{
			CompletedToolCalls: rs.CompletedToolCalls(),
			TurnMessages:       turnMsgs,
		}
	}
	response.Turn = &Turn{
		Messages:    turnMsgs,
		Usage:       t.record.usage.Copy(),
		Persistence: PersistenceNone,
	}

	var saveErr error
	switch {
	case t.rs != nil && t.suspendable != nil:
		if err := t.suspendable.SaveResumedTurn(ctx, turnMsgs, response.Usage); err != nil {
			saveErr = fmt.Errorf("save resumed turn: %w", err)
		}
	case t.sess != nil:
		// On a resume, a plain session never saw the suspended turn (only
		// SuspendableSessions auto-persist suspended turns), so this resume
		// completion is the first write for this turn. Append.
		if err := t.sess.SaveTurn(ctx, turnMsgs, response.Usage); err != nil {
			saveErr = fmt.Errorf("save turn: %w", err)
		}
	default:
		t.emitTurnEnded(ctx)
		return response, nil
	}
	if saveErr != nil {
		resp, err := t.saveError(saveErr)
		t.emitTurnEnded(ctx)
		return resp, err
	}
	response.Turn.Persistence = PersistenceSaved
	t.emitTurnEnded(ctx)
	return response, nil
}

// finishSuspended populates the suspended response, runs OnSuspend and
// PostGeneration hooks, persists the suspended turn (if a
// SuspendableSession is present), and emits the terminal suspended and
// turn_ended stream items. Hooks run before persistence so a hook abort
// leaves the session untouched — no compensation needed.
//
// Suspension works without a session: when the session is nil or does not
// implement SuspendableSession, the Response.Suspension payload is still
// populated and returned to the caller, who is responsible for persisting
// history and state themselves.
//
// If continuesSuspension is true, OnSuspend hooks and the terminal stream
// item are skipped. This is used for pure partial resumes, which continue an
// existing suspension rather than announcing a new one.
func (t *turn) finishSuspended(ctx context.Context, snap *suspendedSnapshot, continuesSuspension bool) (*Response, error) {
	a, logger, hctx, response := t.agent, t.logger, t.hctx, t.response

	// Build the turn the caller will need on resume. For a generate-driven
	// suspend this is inputMessages + the assistant tool_use and any partial
	// tool_result. For a partial resume it is the existing turn plus any
	// tool_result updates captured in rs.
	turnMsgs := t.turnMessages()

	response.Status = ResponseStatusSuspended
	response.Suspension = &SuspensionState{
		PendingToolCalls:   snap.PendingToolCalls,
		CompletedToolCalls: snap.CompletedToolCalls,
		TurnMessages:       turnMsgs,
		BatchHalted:        snap.BatchHalted,
	}

	hctx.Response = response
	hctx.OutputMessages = response.OutputMessages
	hctx.Usage = response.Usage

	// Run OnSuspend hooks before PostGeneration and before persistence.
	// Aborting here leaves the session in its previous state.
	if !continuesSuspension {
		for _, hook := range a.hooks.OnSuspend {
			if err := hook(ctx, hctx); err != nil {
				var abortErr *HookAbortError
				if errors.As(err, &abortErr) {
					abortErr.HookType = "OnSuspend"
					logger.Error("on-suspend hook aborted", "error", abortErr)
					return t.fail(ctx, abortErr)
				}
				logger.Error("on-suspend hook error", "error", err)
			}
		}
	}

	// Run PostGeneration hooks (they see Status=Suspended). Still before
	// persistence so an abort cannot strand a saved suspended turn.
	for _, hook := range a.hooks.PostGeneration {
		if err := hook(ctx, hctx); err != nil {
			var abortErr *HookAbortError
			if errors.As(err, &abortErr) {
				abortErr.HookType = "PostGeneration"
				logger.Error("post-generation hook aborted", "error", abortErr)
				return t.fail(ctx, abortErr)
			}
			logger.Error("post-generation hook error", "error", err)
		}
	}

	response.Turn = &Turn{
		Messages:    turnMsgs,
		Usage:       t.record.usage.Copy(),
		Suspension:  response.Suspension,
		Persistence: PersistenceNone,
	}

	// Persist the suspended turn only after hooks succeed, and only if the
	// caller opted into auto-persistence via a SuspendableSession. Plain
	// sessions and session-less callers rely on the Response.Suspension
	// payload to drive their own persistence.
	if t.suspendable != nil {
		if err := t.suspendable.SaveSuspendedTurn(ctx, turnMsgs, response.Usage, response.Suspension); err != nil {
			err = fmt.Errorf("save suspended turn: %w", err)
			if continuesSuspension {
				// A partial resume leaves the earlier suspension in place.
				logger.Error("session save error", "error", err)
				return t.fail(ctx, err)
			}
			// The pending calls may not exist in the session. The caller
			// reloads it, and persists Response.Suspension itself only when
			// the session does not hold it.
			resp, err := t.saveError(err)
			t.emitTurnEnded(ctx)
			return resp, err
		}
		response.Turn.Persistence = PersistenceSaved
	}

	// Emit the terminal suspended stream item only after any persistence
	// succeeds and hooks have succeeded, so a stream consumer never sees a
	// suspended terminal for a call that ultimately returned an error. The
	// state is already decided, so a callback error is only logged.
	if !continuesSuspension {
		if err := t.emit(context.WithoutCancel(ctx), &ResponseItem{
			Type:       ResponseItemTypeSuspended,
			Suspension: response.Suspension,
		}); err != nil {
			logger.Error("suspended event callback error", "error", err)
		}
	}
	t.emitTurnEnded(ctx)
	return response, nil
}
