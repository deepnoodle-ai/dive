package dive

import (
	"fmt"
	"sync/atomic"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/google/uuid"
)

// notRunToolCallResult answers a call the turn ended before it started.
func notRunToolCallResult(call *llm.ToolUseContent) *ToolCallResult {
	return &ToolCallResult{
		ID:     call.ID,
		Name:   call.Name,
		Input:  call.Input,
		Result: NewToolResultError(ToolCallNotRunText),
		Error:  ErrToolCallNotRun,
	}
}

// unknownToolCallResult answers a call that was running when the turn ended
// and had not reported. handle, when set, is where its result arrives.
func unknownToolCallResult(call *llm.ToolUseContent, handle *BackgroundTaskHandle) *ToolCallResult {
	return &ToolCallResult{
		ID:               call.ID,
		Name:             call.Name,
		Input:            call.Input,
		Result:           NewToolResultError(ToolCallUnknownText),
		Error:            ErrToolCallUnknown,
		BackgroundHandle: handle,
	}
}

// closedToolBatch is a tool batch that stopped before it finished, with
// every call answered by what is known about it.
type closedToolBatch struct {
	// results answers every call in call order: with its own result when
	// the tool returned one, else with a not-run or unknown result.
	results []*ToolCallResult

	// completed are the calls' own results, which carry the hooks'
	// AdditionalContext and reminders.
	completed []*ToolCallResult

	// records says what is known about each call, in call order.
	records []ToolCallRecord

	// reconcile is set when a call with an unknown result is not read-only.
	reconcile bool

	// items are the tool_call and tool_call_result items the calls still
	// owe, so that every call has both.
	items []*ResponseItem
}

// closeToolBatch answers every call of a batch that stopped early. A call
// whose tool returned keeps its result. A call that was running, or that
// suspended in a turn that is not suspending, is unknown: it may have taken
// effect. Any other call never started.
func closeToolBatch(toolCalls []*llm.ToolUseContent, toolsByName map[string]Tool, batch *toolBatchResult) *closedToolBatch {
	closed := &closedToolBatch{}
	for i, call := range toolCalls {
		outcome := batch.Outcomes[i]
		var result *ToolCallResult
		var state ToolCallState
		switch {
		case outcome.Running != nil || outcome.Pending != nil:
			state = ToolCallStateUnknown
			result = unknownToolCallResult(call, outcome.Running)
			if !readOnlyTool(call, toolsByName) {
				closed.reconcile = true
			}
		case outcome.Result != nil:
			state = ToolCallStateCompleted
			result = outcome.Result
			closed.completed = append(closed.completed, result)
		default:
			state = ToolCallStateNotStarted
			result = notRunToolCallResult(call)
		}
		closed.results = append(closed.results, result)
		closed.records = append(closed.records, ToolCallRecord{ID: call.ID, Name: call.Name, State: state})
		if !outcome.announced {
			closed.items = append(closed.items, &ResponseItem{
				Type:     ResponseItemTypeToolCall,
				ToolCall: call,
			})
		}
		// A suspended call reported its suspension; its answer is new.
		if !outcome.reported || outcome.Pending != nil {
			closed.items = append(closed.items, &ResponseItem{
				Type:           ResponseItemTypeToolCallResult,
				ToolCallResult: result,
			})
		}
	}
	return closed
}

// readOnlyTool reports whether a call's tool is annotated ReadOnlyHint.
func readOnlyTool(call *llm.ToolUseContent, toolsByName map[string]Tool) bool {
	tool, ok := toolsByName[call.Name]
	if !ok {
		return false
	}
	ann := tool.Annotations()
	return ann != nil && ann.ReadOnlyHint
}

// startBackgroundTask replaces the result of a tool that returned a
// BackgroundResult with its started message, and records the task's handle,
// which it returns. It returns nil for any other result.
func startBackgroundTask(hctx *HookContext, call *llm.ToolUseContent, result *ToolCallResult) *BackgroundTaskHandle {
	if result == nil || result.Result == nil || result.Result.Background == nil {
		return nil
	}
	bg := result.Result.Background
	handle := &BackgroundTaskHandle{
		TaskID:      bg.id,
		ToolUseID:   call.ID,
		Description: bg.description,
		Done:        bg.done,
	}
	if hctx.backgroundTaskStarted != nil {
		hctx.backgroundTaskStarted(handle)
	}
	result.Result = NewToolResultText(backgroundStartedMessage(bg.description, bg.id))
	return handle
}

// keepUnhooked records the result of a call whose tool returned after its
// batch stopped. No PostToolUse hook runs for it, since the turn is ending.
// A background result becomes its started message and handle; a suspension
// becomes a pending call.
func keepUnhooked(hctx *HookContext, call *llm.ToolUseContent, preHctx *HookContext, result *ToolCallResult, outcome *toolCallOutcome) {
	if result.Result != nil && result.Result.Suspend != nil {
		outcome.Result = nil
		outcome.Pending = toPendingToolCall(call, result.Result.Suspend)
		return
	}
	if handle := startBackgroundTask(hctx, call, result); handle != nil {
		handle.hookCtx = preHctx
		result.BackgroundHandle = handle
	}
	outcome.Result = result
}

// parallelToolResult is a result sent by a parallel tool goroutine.
type parallelToolResult struct {
	index  int
	result *ToolCallResult
}

// The states of a call in a parallel batch. The goroutine that moves a call
// from callPending to callRunning starts it; a batch that stops first moves
// it to callAbandoned, and it never starts.
const (
	callPending int32 = iota
	callRunning
	callAbandoned
)

// stopParallelBatch settles a parallel batch that stops before every call
// has reported. It does not wait for a running tool. Results already in ch
// are kept, without PostToolUse hooks. A call that has not started is left
// not started, and never starts. A call still running gets a background task
// handle, recorded on the turn, and a forwarder that outlives the batch
// delivers its result to the handle when the tool returns.
func (a *Agent) stopParallelBatch(
	hctx *HookContext,
	toolCalls []*llm.ToolUseContent,
	preps []toolCallPrep,
	states []atomic.Int32,
	batch *toolBatchResult,
	ch <-chan parallelToolResult,
	remaining int,
) {
drain:
	for remaining > 0 {
		select {
		case landed := <-ch:
			remaining--
			i := landed.index
			keepUnhooked(hctx, toolCalls[i], preps[i].preHctx, landed.result, &batch.Outcomes[i])
		default:
			break drain
		}
	}

	late := map[int]chan *ToolResult{}
	for i, call := range toolCalls {
		outcome := &batch.Outcomes[i]
		if outcome.Result != nil || outcome.Pending != nil || preps[i].denied {
			continue
		}
		if states[i].CompareAndSwap(callPending, callAbandoned) {
			continue
		}
		done := make(chan *ToolResult, 1)
		handle := &BackgroundTaskHandle{
			TaskID:      uuid.New().String(),
			ToolUseID:   call.ID,
			Description: fmt.Sprintf("%s call still running when its turn ended", call.Name),
			Done:        done,
			hookCtx:     preps[i].preHctx,
		}
		if hctx.backgroundTaskStarted != nil {
			hctx.backgroundTaskStarted(handle)
		}
		outcome.Running = handle
		late[i] = done
	}
	if len(late) > 0 {
		go forwardLateResults(ch, late)
	}
}

// forwardLateResults moves the result of each call still running when its
// batch stopped from the batch channel to the call's handle. Every result
// still to come on ch belongs to one of these calls.
func forwardLateResults(ch <-chan parallelToolResult, late map[int]chan *ToolResult) {
	for range len(late) {
		landed := <-ch
		if done, ok := late[landed.index]; ok {
			deliverLateResult(landed.result, done)
		}
	}
}

// deliverLateResult sends the result a call returned after its turn ended
// to done. A background task's result is its eventual one.
func deliverLateResult(result *ToolCallResult, done chan<- *ToolResult) {
	switch {
	case result == nil || result.Result == nil:
		done <- NewToolResultError("The call returned no result.")
	case result.Result.Background != nil:
		go func() { done <- <-result.Result.Background.done }()
	case result.Result.Suspend != nil:
		done <- NewToolResultError("The call asked for input after its turn ended; the request was dropped.")
	default:
		done <- result.Result
	}
}

// closedToolResultMessage is the tool_result message of a stopped batch:
// every call's answer in call order, then the hooks' AdditionalContext.
func closedToolResultMessage(closed *closedToolBatch) *llm.Message {
	msg := llm.NewToolResultMessage(getToolResultContent(closed.results)...)
	for _, tc := range getAdditionalContextContent(closed.completed) {
		msg.Content = append(msg.Content, tc)
	}
	msg.Content = toolResultsBeforeAuxiliaryContent(msg.Content)
	return msg
}

// resumedBatchRecords records every call of a suspended batch whose resume
// stopped while running its not-started calls, in call order. The calls
// that closed does not cover completed before the suspension, or the caller
// supplied their results.
func resumedBatchRecords(rs *resumeState, closed *closedToolBatch) []ToolCallRecord {
	byID := make(map[string]ToolCallRecord, len(closed.records))
	for _, record := range closed.records {
		byID[record.ID] = record
	}
	var records []ToolCallRecord
	for _, call := range toolUseContents(rs.AssistantToolUse) {
		record, ok := byID[call.ID]
		if !ok {
			record = ToolCallRecord{ID: call.ID, Name: call.Name, State: ToolCallStateCompleted}
		}
		records = append(records, record)
	}
	return records
}
