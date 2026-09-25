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

	// usageUnknown is set when a model call's usage could not be observed.
	usageUnknown bool

	// toolCalls records the batch in flight when the turn stopped, and
	// reconcile is set when one of its calls with an unknown result is not
	// read-only. owedItems are the tool_call and tool_call_result items its
	// calls still owe, which the exit emits.
	toolCalls []ToolCallRecord
	reconcile bool
	owedItems []*ResponseItem

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

// addPartialResponse records the usage of a model response that stopped
// before it finished. usageSeen reports whether the stream reported usage.
func (r *turnRecord) addPartialResponse(response *llm.Response, usageSeen bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelCalled = true
	if response != nil {
		r.usage.Add(&response.Usage)
	}
	if !usageSeen {
		r.usageUnknown = true
	}
	r.version++
}

// addBackgroundTask records the handle of a background task the turn started.
func (r *turnRecord) addBackgroundTask(handle *BackgroundTaskHandle) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backgroundTasks = append(r.backgroundTasks, handle)
	r.version++
}

// stopBatch records the batch in flight when the turn stopped, closed by
// closeToolBatch.
func (r *turnRecord) stopBatch(records []ToolCallRecord, closed *closedToolBatch) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.toolCalls = records
	r.reconcile = closed.reconcile
	r.owedItems = closed.items
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

	// priorUsage is the usage the suspended turn had accumulated before
	// this invocation resumed it, or nil.
	priorUsage *llm.Usage

	// syncedVersion is the record version the response was last synced at.
	syncedVersion int
}

// turnExitKind says how an invocation ends.
type turnExitKind int

const (
	turnExitCompleted turnExitKind = iota
	turnExitSuspended
	turnExitFailed
	turnExitStopped
)

// turnExit describes how an invocation ends; end acts on it.
type turnExit struct {
	kind turnExitKind

	// err is the failure of a failed exit.
	err error

	// suspension is the batch state of a suspended exit.
	suspension *suspendedSnapshot

	// outcome is the outcome of a stopped exit: a turn the model or its
	// provider stopped short, which ends without an error.
	outcome *TurnOutcome

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

// stoppedExit ends the invocation with a turn the model or its provider
// stopped short.
func stoppedExit(outcome *TurnOutcome) turnExit {
	return turnExit{kind: turnExitStopped, outcome: outcome}
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
	case turnExitStopped:
		return t.closeIncomplete(ctx, exit.outcome, nil)
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
// *GenerationError. The turn is closed and saved (closeIncomplete), unless
// IncompleteTurnOptions.Discard is set. A failed partial resume returns no
// response: the turn is still suspended.
func (t *turn) fail(ctx context.Context, err error) (*Response, error) {
	if t.partialResume {
		r := t.record
		r.mu.Lock()
		defer r.mu.Unlock()
		return nil, &GenerationError{
			Err:            err,
			Usage:          r.usage,
			OutputMessages: slices.Clone(r.output),
			Items:          slices.Clone(r.items),
		}
	}
	outcome := failureOutcome(err, t.record)
	if t.agent.incompleteTurns.Discard {
		return t.discardIncomplete(outcome, err)
	}
	return t.closeIncomplete(ctx, outcome, err)
}

// discardIncomplete ends a failed invocation as before v1.34
// (IncompleteTurnOptions.Discard): the output is left as the error found it,
// nothing is saved, and no further item is emitted.
func (t *turn) discardIncomplete(outcome *TurnOutcome, err error) (*Response, error) {
	t.syncResponse(true)
	r := t.record
	response := t.response
	response.Status = ResponseStatusIncomplete
	response.Suspension = nil
	if len(r.backgroundTasks) > 0 {
		response.BackgroundTasks = r.backgroundTasks
	}
	response.Turn = &Turn{
		Messages:    t.turnMessages(),
		Usage:       t.turnUsage(),
		Outcome:     outcome,
		Persistence: PersistenceNone,
	}
	r.mu.Lock()
	genErr := &GenerationError{
		Err:            err,
		Usage:          r.usage,
		OutputMessages: slices.Clone(r.output),
		Items:          slices.Clone(r.items),
		Response:       response,
	}
	r.mu.Unlock()
	return response, genErr
}

// closeIncomplete ends an incomplete invocation: err is the error that ended
// it, or nil for a turn the model stopped short. It closes the turn (every
// tool call answered, the outcome reminder last), runs the OnIncompleteTurn
// hooks, saves the turn, and emits turn_ended. Everything after the turn
// stopped runs on a context without its cancellation.
func (t *turn) closeIncomplete(ctx context.Context, outcome *TurnOutcome, err error) (*Response, error) {
	a, r, hctx, response := t.agent, t.record, t.hctx, t.response
	ctx = context.WithoutCancel(ctx)

	// Items the stopped batch owes, then the reminders queued while the
	// turn ended, which are recorded before the outcome reminder.
	t.emitOwedItems(ctx)
	t.recordPendingReminders()

	// Sync the response from the record unless it is current, so that the
	// edits a Stop hook made before a PostGeneration abort are kept, as a
	// completed turn keeps them.
	r.mu.Lock()
	stale := r.version != t.syncedVersion
	r.mu.Unlock()
	if stale || response.FinishedAt == nil {
		t.syncResponse(true)
	}

	// Answer every call still open, in the suspended turn on a resume and
	// in the output, and record the calls answered here.
	output := slices.Clone(response.OutputMessages)
	prefix := t.inputMessages
	var answered []*llm.ToolUseContent
	if t.rs != nil {
		prefix, answered = answerOpenToolCalls(t.rs.TurnMessages, outcome.ToolCalls)
	}
	output, answeredOutput := answerOpenToolCalls(output, outcome.ToolCalls)
	answered = append(answered, answeredOutput...)
	outcome.ToolCalls = withNotStartedRecords(outcome.ToolCalls, answered)
	t.emitAnswered(ctx, answered, outcome.ToolCalls)
	r.mu.Lock()
	response.Items = slices.Clone(r.items)
	r.mu.Unlock()
	response.FinishedAt = Ptr(time.Now())

	response.Status = ResponseStatusIncomplete
	response.Suspension = nil
	if len(r.backgroundTasks) > 0 {
		response.BackgroundTasks = r.backgroundTasks
	}
	response.OutputMessages = output
	turnRec := &Turn{
		Messages:    joinMessages(prefix, output),
		Usage:       t.turnUsage(),
		Outcome:     outcome,
		Persistence: PersistenceNone,
	}
	response.Turn = turnRec

	discard := false
	if len(a.hooks.OnIncompleteTurn) > 0 {
		hctx.Response = response
		hctx.OutputMessages = output
		hctx.Usage = r.usage
		hctx.Turn = turnRec
		for _, hook := range a.hooks.OnIncompleteTurn {
			decision, hookErr := hook(ctx, hctx)
			if hookErr != nil {
				t.logger.Error("incomplete turn hook error", "error", hookErr)
				continue
			}
			if decision != nil && decision.Discard {
				discard = true
			}
		}
		output = slices.Clone(hctx.OutputMessages)
		for _, delivery := range hctx.reminders.drainPending() {
			if delivery.recording == Recorded {
				output = append(output, NewReminderMessage(delivery.reminder))
			}
		}
		if turnRec.Outcome != nil {
			outcome = turnRec.Outcome
		}
	}
	output = append(output, NewReminderMessage(NewTurnOutcomeReminder(outcome)))
	response.OutputMessages = output
	turnRec.Messages = joinMessages(prefix, output)
	turnRec.Outcome = outcome

	var saveErr error
	if !discard {
		turnRec.Persistence, saveErr = t.save(ctx, turnRec.Messages)
	}
	t.emitTurnEnded(ctx)

	if err == nil {
		if saveErr != nil {
			return response, saveErr
		}
		return response, nil
	}
	if saveErr != nil {
		err = errors.Join(err, saveErr)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return response, &GenerationError{
		Err:            err,
		Usage:          r.usage,
		OutputMessages: output,
		Items:          slices.Clone(r.items),
		Response:       response,
	}
}

// recordPendingReminders records the reminders queued for the next model
// call that will not come: a recorded reminder joins the output, a
// model-only one is dropped.
func (t *turn) recordPendingReminders() {
	for _, delivery := range t.hctx.reminders.drainPending() {
		if delivery.recording == Recorded {
			t.record.addOutput(NewReminderMessage(delivery.reminder))
		}
	}
}

// emitAnswered emits a tool_call_result item for each call the closing of
// the turn answered, preceded by a tool_call item when the call was never
// announced, so that every call has both. A callback error is only logged.
func (t *turn) emitAnswered(ctx context.Context, answered []*llm.ToolUseContent, records []ToolCallRecord) {
	if len(answered) == 0 {
		return
	}
	r := t.record
	announced := map[string]bool{}
	r.mu.Lock()
	for _, item := range r.items {
		if item.Type == ResponseItemTypeToolCall && item.ToolCall != nil {
			announced[item.ToolCall.ID] = true
		}
	}
	r.mu.Unlock()
	unknown := map[string]bool{}
	for _, record := range records {
		if record.State == ToolCallStateUnknown {
			unknown[record.ID] = true
		}
	}
	var items []*ResponseItem
	for _, call := range answered {
		if !announced[call.ID] {
			items = append(items, &ResponseItem{Type: ResponseItemTypeToolCall, ToolCall: call})
		}
		result := notRunToolCallResult(call)
		if unknown[call.ID] {
			result = unknownToolCallResult(call, nil)
		}
		items = append(items, &ResponseItem{Type: ResponseItemTypeToolCallResult, ToolCallResult: result})
	}
	r.mu.Lock()
	r.owedItems = items
	r.mu.Unlock()
	t.emitOwedItems(ctx)
}

// save writes the turn to the session: a resume replaces the suspended turn
// (SaveResumedTurn) on a SuspendableSession, any other turn is appended
// (SaveTurn). ctx carries no cancellation; the write is bounded by
// IncompleteTurnOptions.SaveTimeout. The session is handed this
// invocation's usage alone, since it sums a resumed turn's usage itself.
func (t *turn) save(ctx context.Context, messages []*llm.Message) (PersistenceState, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), t.agent.incompleteTurns.SaveTimeout)
	defer cancel()
	usage := t.response.Usage
	var err error
	switch {
	case t.rs != nil && t.suspendable != nil:
		if err = t.suspendable.SaveResumedTurn(ctx, messages, usage); err != nil {
			err = fmt.Errorf("save resumed turn: %w", err)
		}
	case t.sess != nil:
		// On a resume, a plain session never saw the suspended turn (only
		// SuspendableSessions persist suspended turns), so this is the
		// first write for this turn. Append.
		if err = t.sess.SaveTurn(ctx, messages, usage); err != nil {
			err = fmt.Errorf("save turn: %w", err)
		}
	default:
		return PersistenceNone, nil
	}
	if err != nil {
		t.logger.Error("session save error", "error", err)
		return persistenceOf(err), err
	}
	return PersistenceSaved, nil
}

// turnUsage returns the turn's usage: this invocation's, plus what a resumed
// suspension had accumulated before it.
func (t *turn) turnUsage() *llm.Usage {
	r := t.record
	r.mu.Lock()
	usage := r.usage.Copy()
	r.mu.Unlock()
	if t.priorUsage != nil {
		usage.Add(t.priorUsage)
	}
	return usage
}

// joinMessages returns a new slice holding a then b.
func joinMessages(a, b []*llm.Message) []*llm.Message {
	out := make([]*llm.Message, 0, len(a)+len(b))
	out = append(out, a...)
	return append(out, b...)
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

// emitOwedItems emits the items the calls of a stopped batch still owe, so
// that every tool_call item has a tool_call_result. The turn is ending, so
// they go out on a context that is not cancelled, and a callback error is
// only logged: it must not replace the error that ended the turn.
func (t *turn) emitOwedItems(ctx context.Context) {
	r := t.record
	r.mu.Lock()
	items := r.owedItems
	r.owedItems = nil
	r.mu.Unlock()
	ctx = context.WithoutCancel(ctx)
	for _, item := range items {
		r.mu.Lock()
		r.items = append(r.items, item)
		r.version++
		r.mu.Unlock()
		if err := t.emit(ctx, item); err != nil {
			t.logger.Error("event callback error on a stopped tool batch", "error", err)
		}
	}
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

// finishCompleted runs PostGeneration hooks and saves a completed turn. A
// PostGeneration abort makes the turn incomplete, with the model's output
// kept. A save error is returned with the completed response, whose
// Turn.Persistence says whether the session may hold it.
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

	// On a resume, the session replaces the suspended event with the
	// combined turn (the suspended turn's messages plus the new output), and
	// Response.Suspension carries the final merged turn with no pending
	// calls, so stateless callers can flush it into their history in one
	// append without reconciling a stale partial tool_result.
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
		Usage:       t.turnUsage(),
		Persistence: PersistenceNone,
	}

	ctx = context.WithoutCancel(ctx)
	var saveErr error
	response.Turn.Persistence, saveErr = t.save(ctx, turnMsgs)
	t.emitTurnEnded(ctx)
	if saveErr != nil {
		return response, saveErr
	}
	return response, nil
}

// finishSuspended populates the suspended response, runs OnSuspend and
// PostGeneration hooks, persists the suspended turn (if a
// SuspendableSession is present), and emits the terminal suspended and
// turn_ended stream items. Hooks run before persistence so a hook abort
// leaves the session untouched — no compensation needed.
//
// A suspension is kept even when the context was cancelled by the time the
// agent reaches it: the suspending tool may already have dispatched its
// request, so the hooks and the write run on a context without the
// cancellation, and the invocation returns Suspended with a nil error.
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
	ctx = context.WithoutCancel(ctx)

	// Build the turn the caller will need on resume. For a generate-driven
	// suspend this is inputMessages + the assistant tool_use and any partial
	// tool_result. For a partial resume it is the existing turn plus any
	// tool_result updates captured in rs.
	turnMsgs := t.turnMessages()
	usage := t.turnUsage()

	response.Status = ResponseStatusSuspended
	response.Suspension = &SuspensionState{
		PendingToolCalls:   snap.PendingToolCalls,
		CompletedToolCalls: snap.CompletedToolCalls,
		TurnMessages:       turnMsgs,
		BatchHalted:        snap.BatchHalted,
		Usage:              usage,
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
					return t.abortSuspension(ctx, snap, turnMsgs, abortErr)
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
				if continuesSuspension {
					return t.fail(ctx, abortErr)
				}
				return t.abortSuspension(ctx, snap, turnMsgs, abortErr)
			}
			logger.Error("post-generation hook error", "error", err)
		}
	}

	response.Turn = &Turn{
		Messages:    turnMsgs,
		Usage:       usage,
		Suspension:  response.Suspension,
		Persistence: PersistenceNone,
	}

	// Persist the suspended turn only after hooks succeed, and only if the
	// caller opted into auto-persistence via a SuspendableSession. Plain
	// sessions and session-less callers rely on the Response.Suspension
	// payload to drive their own persistence.
	if t.suspendable != nil {
		saveCtx, cancel := context.WithTimeout(ctx, a.incompleteTurns.SaveTimeout)
		err := t.suspendable.SaveSuspendedTurn(saveCtx, turnMsgs, response.Usage, response.Suspension)
		cancel()
		if err != nil {
			err = fmt.Errorf("save suspended turn: %w", err)
			logger.Error("session save error", "error", err)
			if continuesSuspension {
				// A partial resume leaves the earlier suspension in place.
				return t.fail(ctx, err)
			}
			// The pending calls may not exist in the session. The caller
			// reloads it, and persists Response.Suspension itself only when
			// the session does not hold it.
			response.Turn.Persistence = persistenceOf(err)
			t.emitTurnEnded(ctx)
			return response, err
		}
		response.Turn.Persistence = PersistenceSaved
	}

	// Emit the terminal suspended stream item only after any persistence
	// succeeds and hooks have succeeded, so a stream consumer never sees a
	// suspended terminal for a call that ultimately returned an error. The
	// state is already decided, so a callback error is only logged.
	if !continuesSuspension {
		if err := t.emit(ctx, &ResponseItem{
			Type:       ResponseItemTypeSuspended,
			Suspension: response.Suspension,
		}); err != nil {
			logger.Error("suspended event callback error", "error", err)
		}
	}
	t.emitTurnEnded(ctx)
	return response, nil
}

// abortSuspension ends a new suspension that an OnSuspend or PostGeneration
// hook aborted. The turn is incomplete with hook_abort: the completed calls
// of the suspended batch keep their results, and the suspending calls are
// unknown, since a tool may dispatch its request before it suspends, and an
// earlier OnSuspend hook may have dispatched before a later one aborted.
// Calls the batch never reached are not started.
func (t *turn) abortSuspension(ctx context.Context, snap *suspendedSnapshot, turnMsgs []*llm.Message, abortErr error) (*Response, error) {
	var assistant *llm.Message
	for i := len(turnMsgs) - 1; i >= 0; i-- {
		if msg := turnMsgs[i]; msg.Role == llm.Assistant && hasToolUseContent(msg) {
			assistant = msg
			break
		}
	}
	pending := map[string]bool{}
	for _, call := range snap.PendingToolCalls {
		pending[call.ID] = true
	}
	answered := map[string]bool{}
	for _, msg := range turnMsgs {
		for _, c := range msg.Content {
			if result, ok := c.(*llm.ToolResultContent); ok {
				answered[result.ToolUseID] = true
			}
		}
	}
	_, toolsByName, _ := t.agent.resolveTools(ctx)
	var records []ToolCallRecord
	reconcile := false
	for _, call := range toolUseContents(assistant) {
		state := ToolCallStateNotStarted
		switch {
		case pending[call.ID]:
			state = ToolCallStateUnknown
			if !readOnlyTool(call, toolsByName) {
				reconcile = true
			}
		case answered[call.ID]:
			state = ToolCallStateCompleted
		}
		records = append(records, ToolCallRecord{ID: call.ID, Name: call.Name, State: state})
	}
	t.record.stopBatch(records, &closedToolBatch{reconcile: reconcile})
	return t.fail(ctx, abortErr)
}
