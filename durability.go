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

// checkDurability reports a session that cannot do what DurabilityOptions
// ask of it.
func (a *Agent) checkDurability(sess Session) error {
	if _, ok := sess.(TurnStore); a.durability.CheckpointSteps && !ok {
		return errors.New("dive: DurabilityOptions.CheckpointSteps needs a session that implements TurnStore")
	}
	if _, ok := sess.(SessionClaimer); a.durability.Claim && !ok {
		return errors.New("dive: DurabilityOptions.Claim needs a session that implements SessionClaimer")
	}
	return nil
}

// claimSession claims the session for the agent's owner and renews the claim
// every third of its time to live until the returned release function runs,
// which ends the claim. The returned context is cancelled, with a cause that
// wraps ErrSessionClaimed, when a renewal fails.
func (a *Agent) claimSession(ctx context.Context, claimer SessionClaimer) (context.Context, func(), error) {
	owner, ttl := a.durability.Owner, a.durability.ClaimTTL
	if err := claimer.ClaimSession(ctx, owner, ttl); err != nil {
		return ctx, nil, err
	}
	ctx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(max(ttl/3, time.Millisecond))
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if err := claimer.ClaimSession(ctx, owner, ttl); err != nil {
				if ctx.Err() == nil {
					a.logger.Error("session claim lost", "session_id", claimer.ID(), "error", err)
					cancel(fmt.Errorf("%w: the claim on session %s could not be renewed: %w", ErrSessionClaimed, claimer.ID(), err))
				}
				return
			}
		}
	}()
	release := func() {
		close(done)
		wg.Wait()
		releaseCtx, cancelRelease := context.WithTimeout(context.WithoutCancel(ctx), a.incompleteTurns.SaveTimeout)
		defer cancelRelease()
		if err := claimer.ReleaseSession(releaseCtx, owner); err != nil {
			a.logger.Error("session claim release error", "session_id", claimer.ID(), "error", err)
		}
		cancel(nil)
	}
	return ctx, release, nil
}

// recoverRunningTurn closes the running turn a step checkpoint left open,
// whose invocation ended without recording how, and records it in its
// place. It returns the snapshot reloaded after that write.
func (a *Agent) recoverRunningTurn(ctx context.Context, store TurnStore, snap *SessionSnapshot) (*SessionSnapshot, error) {
	open := snap.OpenTurn
	_, toolsByName, err := a.resolveTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("tool resolution error: %w", err)
	}
	outcome := &TurnOutcome{
		Reason: TurnReasonProcessExit,
		Error:  "the invocation running the turn ended without recording how it ended",
		Next:   defaultNext(TurnReasonProcessExit),
	}
	calls := map[string]*llm.ToolUseContent{}
	for _, msg := range open.Messages {
		for _, call := range toolUseContents(msg) {
			calls[call.ID] = call
		}
	}
	for _, record := range open.ToolCalls {
		if record.State != ToolCallStateRunning {
			continue
		}
		outcome.ToolCalls = append(outcome.ToolCalls, ToolCallRecord{ID: record.ID, Name: record.Name, State: ToolCallStateUnknown})
		call := calls[record.ID]
		if call == nil || !readOnlyTool(call, toolsByName) {
			outcome.Next = TurnNextReconcile
		}
	}
	messages := CloseTurn(open.Messages, outcome)
	recorded, _ := FindLatestTurnOutcome(messages)
	turn := &Turn{
		Schema:    TurnSchema,
		ID:        open.ID,
		Origin:    open.Origin,
		Status:    ResponseStatusIncomplete,
		Messages:  messages,
		Usage:     open.Usage,
		Outcome:   recorded,
		ToolCalls: turnToolCalls(messages, recorded, nil),
	}
	a.logger.Warn("closing a turn left running", "turn_id", open.ID, "next", recorded.Next)
	if _, err := store.CheckpointTurn(ctx, snap.Revision, turn); err != nil {
		return nil, fmt.Errorf("record the turn left running: %w", err)
	}
	snap, err = store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("session load error: %w", err)
	}
	return snap, nil
}

// stepBatch is the tool batch in flight, as step checkpoints record it.
type stepBatch struct {
	// merge is set for a resumed batch, whose results join the suspended
	// turn's tool_result message.
	merge bool

	// running are the calls started and not finished; results the calls
	// finished, in the order they finished.
	running map[string]bool
	results []*ToolCallResult
}

// stepBatchStart begins recording a tool batch for step checkpoints. merge
// is set for a resumed batch.
func (h *HookContext) stepBatchStart(merge bool) {
	t := h.steps
	if t == nil {
		return
	}
	t.stepMu.Lock()
	defer t.stepMu.Unlock()
	t.batch = &stepBatch{merge: merge, running: map[string]bool{}}
}

// stepBatchEnd ends the batch in flight: the caller records its results in
// the turn's messages.
func (h *HookContext) stepBatchEnd() {
	t := h.steps
	if t == nil {
		return
	}
	t.stepMu.Lock()
	defer t.stepMu.Unlock()
	t.batch = nil
}

// stepCallsStarting checkpoints calls as running before their tools are
// called. An error means the calls must not start.
func (h *HookContext) stepCallsStarting(ctx context.Context, calls ...*llm.ToolUseContent) error {
	t := h.steps
	if t == nil || len(calls) == 0 {
		return nil
	}
	t.stepMu.Lock()
	if t.batch != nil {
		for _, call := range calls {
			t.batch.running[call.ID] = true
		}
	}
	t.stepMu.Unlock()
	return t.checkpointStep(ctx)
}

// stepCallFinished checkpoints the result of a call as it arrives.
func (h *HookContext) stepCallFinished(ctx context.Context, call *llm.ToolUseContent, result *ToolCallResult) error {
	t := h.steps
	if t == nil || result == nil {
		return nil
	}
	t.stepMu.Lock()
	if t.batch != nil {
		delete(t.batch.running, call.ID)
		t.batch.results = append(t.batch.results, result)
	}
	t.stepMu.Unlock()
	return t.checkpointStep(ctx)
}

// stepCheckpoint checkpoints the turn as it stands, on a turn with step
// checkpoints.
func (h *HookContext) stepCheckpoint(ctx context.Context) error {
	if h.steps == nil {
		return nil
	}
	return h.steps.checkpointStep(ctx)
}

// checkpointStep records the turn as running: its messages so far, the
// results of the batch in flight, and the state of each call. It is bounded
// by IncompleteTurnOptions.SaveTimeout.
func (t *turn) checkpointStep(ctx context.Context) error {
	t.stepMu.Lock()
	defer t.stepMu.Unlock()
	messages := t.stepMessages()
	records := turnToolCalls(messages, nil, nil)
	if t.batch != nil {
		for i, record := range records {
			if t.batch.running[record.ID] {
				records[i].State = ToolCallStateRunning
			}
		}
	}
	rec := &Turn{
		Schema:    TurnSchema,
		ID:        t.turnID,
		Origin:    t.origin,
		Status:    ResponseStatusRunning,
		Messages:  messages,
		Usage:     t.turnUsage(),
		ToolCalls: records,
	}
	ctx, cancel := context.WithTimeout(ctx, t.agent.incompleteTurns.SaveTimeout)
	defer cancel()
	if err := t.checkpoint(ctx, rec); err != nil {
		return fmt.Errorf("step %w", err)
	}
	return nil
}

// stepMessages returns the turn's messages so far: those the invocation
// started from (the input, the suspended turn or the continued turn), its
// output, and the results of the batch in flight, which join the resumed
// turn's tool_result message or follow the output. Caller holds t.stepMu.
func (t *turn) stepMessages() []*llm.Message {
	var prefix []*llm.Message
	switch {
	case t.rs != nil:
		prefix = t.rs.TurnMessages
	case t.continued != nil:
		prefix = t.continued
	default:
		prefix = t.inputMessages
	}
	r := t.record
	r.mu.Lock()
	messages := joinMessages(prefix, r.output)
	r.mu.Unlock()
	if t.batch == nil || len(t.batch.results) == 0 {
		return messages
	}
	var content []llm.Content
	for _, c := range getToolResultContent(t.batch.results) {
		content = append(content, c)
	}
	for _, c := range getAdditionalContextContent(t.batch.results) {
		content = append(content, c)
	}
	if t.batch.merge && t.rs.ToolResultMessageIdx >= 0 {
		i := t.rs.ToolResultMessageIdx
		merged := *messages[i]
		merged.Content = toolResultsBeforeAuxiliaryContent(append(slices.Clone(merged.Content), content...))
		messages[i] = &merged
		return messages
	}
	return append(messages, &llm.Message{Role: llm.User, Content: toolResultsBeforeAuxiliaryContent(content)})
}
