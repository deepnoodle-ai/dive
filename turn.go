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
}

func newTurnRecord() *turnRecord {
	return &turnRecord{usage: &llm.Usage{}}
}

// collecting returns a callback that records each item and then forwards it
// to callback.
func (r *turnRecord) collecting(callback EventCallback) EventCallback {
	return func(ctx context.Context, item *ResponseItem) error {
		r.mu.Lock()
		r.items = append(r.items, item)
		r.mu.Unlock()
		return callback(ctx, item)
	}
}

// addOutput records a message the turn added to the conversation.
func (r *turnRecord) addOutput(msg *llm.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.output = append(r.output, msg)
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
}

// addBackgroundTask records the handle of a background task the turn started.
func (r *turnRecord) addBackgroundTask(handle *BackgroundTaskHandle) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backgroundTasks = append(r.backgroundTasks, handle)
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
	// not recorded; the terminal suspended item is the only such item.
	emit EventCallback

	// inputMessages is the turn's input: the caller's new messages and a
	// background-results message. Empty on a resume, whose input is rs.
	inputMessages []*llm.Message

	sess        Session
	suspendable SuspendableSession
	rs          *resumeState
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

	// withPartialWork wraps err in a *GenerationError that carries the
	// turn's output, items and usage so far.
	withPartialWork bool

	// suspension is the batch state of a suspended exit.
	suspension *suspendedSnapshot

	// continuesSuspension marks a partial resume: the turn was already
	// suspended and stays so, so OnSuspend hooks and the terminal suspended
	// item are skipped.
	continuesSuspension bool
}

func completedExit() turnExit {
	return turnExit{kind: turnExitCompleted}
}

func suspendedExit(snap *suspendedSnapshot, continuesSuspension bool) turnExit {
	return turnExit{kind: turnExitSuspended, suspension: snap, continuesSuspension: continuesSuspension}
}

// failedExit returns err as is.
func failedExit(err error) turnExit {
	return turnExit{kind: turnExitFailed, err: err}
}

// failedExitWithPartialWork returns err wrapped in a *GenerationError.
func failedExitWithPartialWork(err error) turnExit {
	return turnExit{kind: turnExitFailed, err: err, withPartialWork: true}
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
}

// end finishes the invocation: it builds the return for a failed exit, and
// runs the terminal hooks, saves the turn and builds the response for a
// completed or suspended one.
func (t *turn) end(ctx context.Context, exit turnExit) (*Response, error) {
	switch exit.kind {
	case turnExitFailed:
		if !exit.withPartialWork {
			return nil, exit.err
		}
		r := t.record
		r.mu.Lock()
		defer r.mu.Unlock()
		return nil, &GenerationError{
			Err:            exit.err,
			Usage:          r.usage,
			OutputMessages: slices.Clone(r.output),
			Items:          slices.Clone(r.items),
		}
	case turnExitSuspended:
		t.syncResponse(false)
		t.response.BackgroundTasks = t.record.backgroundTasks
		return t.finishSuspended(ctx, exit.suspension, exit.continuesSuspension)
	default:
		t.syncResponse(false)
		return t.finishCompleted(ctx)
	}
}

// finishCompleted runs PostGeneration hooks and saves a completed turn.
func (t *turn) finishCompleted(ctx context.Context) (*Response, error) {
	a, logger, hctx, response := t.agent, t.logger, t.hctx, t.response

	hctx.Response = response
	hctx.OutputMessages = response.OutputMessages
	hctx.Usage = response.Usage
	for _, hook := range a.hooks.PostGeneration {
		if err := hook(ctx, hctx); err != nil {
			// Check if this is a fatal abort error
			var abortErr *HookAbortError
			if errors.As(err, &abortErr) {
				abortErr.HookType = "PostGeneration"
				logger.Error("post-generation hook aborted", "error", abortErr)
				return nil, abortErr
			}
			// Regular errors are logged but don't affect the response
			logger.Error("post-generation hook error", "error", err)
		}
	}

	// Save session turn. On resume, replace the suspended event with the
	// combined turn (pre-suspend turn messages plus new output). Otherwise
	// append a new turn with input + output. Persistence failures are fatal:
	// returning a successful Response while the session is out of sync would
	// strand the caller with state that doesn't match disk.
	//
	// On a resume completion we also populate Response.Suspension with the
	// final merged turn snapshot (PendingToolCalls = nil) so stateless
	// callers can flush the turn into their local history in one append
	// without reconciling a stale partial tool_result from their saved
	// state.
	if rs := t.rs; rs != nil {
		turnMsgs := make([]*llm.Message, 0, len(rs.TurnMessages)+len(response.OutputMessages))
		turnMsgs = append(turnMsgs, rs.TurnMessages...)
		turnMsgs = append(turnMsgs, response.OutputMessages...)
		switch {
		case t.suspendable != nil:
			if err := t.suspendable.SaveResumedTurn(ctx, turnMsgs, response.Usage); err != nil {
				logger.Error("session save error", "error", err)
				return nil, fmt.Errorf("save resumed turn: %w", err)
			}
		case t.sess != nil:
			// Plain session: the suspend never hit SaveTurn (only
			// SuspendableSessions auto-persist suspended turns), so this
			// resume completion is the first write for this turn. Append.
			if err := t.sess.SaveTurn(ctx, turnMsgs, response.Usage); err != nil {
				logger.Error("session save error", "error", err)
				return nil, fmt.Errorf("save turn: %w", err)
			}
		}
		response.Suspension = &SuspensionState{
			CompletedToolCalls: rs.CompletedToolCalls(),
			TurnMessages:       turnMsgs,
		}
	} else if t.sess != nil {
		turnMessages := make([]*llm.Message, 0, len(t.inputMessages)+len(response.OutputMessages))
		turnMessages = append(turnMessages, t.inputMessages...)
		turnMessages = append(turnMessages, response.OutputMessages...)
		if err := t.sess.SaveTurn(ctx, turnMessages, response.Usage); err != nil {
			logger.Error("session save error", "error", err)
			return nil, fmt.Errorf("save turn: %w", err)
		}
	}

	response.Status = ResponseStatusCompleted
	if len(t.record.backgroundTasks) > 0 {
		response.BackgroundTasks = t.record.backgroundTasks
	}
	return response, nil
}

// finishSuspended populates the suspended response, runs OnSuspend and
// PostGeneration hooks, persists the suspended turn (if a
// SuspendableSession is present), and emits the terminal suspended stream
// item. Hooks run before persistence so a hook abort leaves the session
// untouched — no compensation needed.
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
	var turnMsgs []*llm.Message
	if t.rs != nil {
		turnMsgs = append(turnMsgs, t.rs.TurnMessages...)
		turnMsgs = append(turnMsgs, response.OutputMessages...)
	} else {
		turnMsgs = append(turnMsgs, t.inputMessages...)
		turnMsgs = append(turnMsgs, response.OutputMessages...)
	}

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
					return nil, abortErr
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
				return nil, abortErr
			}
			logger.Error("post-generation hook error", "error", err)
		}
	}

	// Persist the suspended turn only after hooks succeed, and only if the
	// caller opted into auto-persistence via a SuspendableSession. Plain
	// sessions and session-less callers rely on the Response.Suspension
	// payload to drive their own persistence.
	if t.suspendable != nil {
		if err := t.suspendable.SaveSuspendedTurn(ctx, turnMsgs, response.Usage, response.Suspension); err != nil {
			// If we can't persist the suspend, we must not return a
			// suspended Response with pending IDs that don't exist in the
			// session. Fail the call loudly.
			logger.Error("session save error", "error", err)
			return nil, fmt.Errorf("save suspended turn: %w", err)
		}
	}

	// Emit the terminal suspended stream item only after any persistence
	// succeeds and hooks have succeeded, so a stream consumer never sees a
	// suspended terminal for a call that ultimately returned an error.
	if !continuesSuspension {
		if err := t.emit(ctx, &ResponseItem{
			Type:       ResponseItemTypeSuspended,
			Suspension: response.Suspension,
		}); err != nil {
			return nil, err
		}
	}

	return response, nil
}
