package dive

import (
	"errors"

	"github.com/deepnoodle-ai/dive/llm"
)

// SuspendForApprovalError is returned by a PreToolUse hook to pause the agent
// BEFORE a tool executes, pending external approval of the call — rather than
// allowing (nil) or denying (a plain error). The agent does NOT run the tool;
// it records the call as a PendingToolCall with AwaitingApproval set and
// suspends the turn (Response.Status == ResponseStatusSuspended), exactly like
// a tool that returned a SuspendResult.
//
// On resume the caller decides each gated call's fate with WithApprovals:
//
//   - approve (true): the tool is executed. The PreToolUse hook is told about
//     the approval via HookContext.Approved so it allows the call through
//     instead of gating again.
//   - reject (false): a denial result is injected and the tool never runs.
//
// Precedence within the PreToolUse chain: a plain denial from any hook beats an
// approval request (a hard "no" cannot be escalated into "ask a human"), and a
// HookAbortError still aborts the whole generation.
type SuspendForApprovalError struct {
	// Prompt is human-readable context describing what needs approval.
	// Surfaced on PendingToolCall.Prompt.
	Prompt string

	// Reason classifies the suspension. Surfaced on PendingToolCall.Reason.
	// Defaults to SuspendReasonAuth when empty (an approval gate is an
	// authorization step).
	Reason SuspendReason

	// Metadata is optional structured data surfaced on PendingToolCall.Metadata
	// (request IDs, approver hints, expiry, etc.). JSON-friendly values only.
	Metadata map[string]any
}

func (e *SuspendForApprovalError) Error() string {
	if e.Prompt != "" {
		return "tool call requires approval: " + e.Prompt
	}
	return "tool call requires approval"
}

// SuspendForApproval returns an error a PreToolUse hook can return to suspend
// the turn pending approval of the current tool call, instead of executing it.
// Pass nil for metadata if none is needed:
//
//	func gate(ctx context.Context, hctx *dive.HookContext) error {
//	    if hctx.Approved || !sensitive(hctx.Tool) {
//	        return nil // already approved on resume, or doesn't need a gate
//	    }
//	    return dive.SuspendForApproval("Approve running "+hctx.Tool.Name()+"?", nil)
//	}
func SuspendForApproval(prompt string, metadata map[string]any) error {
	return &SuspendForApprovalError{Prompt: prompt, Metadata: metadata}
}

// SuspendForApprovalWithReason is like SuspendForApproval but sets an explicit
// SuspendReason (defaults to SuspendReasonAuth when empty).
func SuspendForApprovalWithReason(prompt string, reason SuspendReason, metadata map[string]any) error {
	return &SuspendForApprovalError{Prompt: prompt, Reason: reason, Metadata: metadata}
}

// asSuspendForApproval reports whether err is, or wraps, a
// *SuspendForApprovalError and returns it when so.
func asSuspendForApproval(err error) (*SuspendForApprovalError, bool) {
	var e *SuspendForApprovalError
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// approvalSuspendResult synthesizes the ToolCallResult the execution loop uses
// to record a pre-execution approval gate. It deliberately reuses the regular
// suspend plumbing (finishToolCall -> toPendingToolCall); the internal approval
// flag is what makes the resulting PendingToolCall.AwaitingApproval true.
func approvalSuspendResult(toolCall *llm.ToolUseContent, e *SuspendForApprovalError) *ToolCallResult {
	reason := e.Reason
	if reason == "" {
		reason = SuspendReasonAuth
	}
	return &ToolCallResult{
		ID:    toolCall.ID,
		Name:  toolCall.Name,
		Input: toolCall.Input,
		Result: &ToolResult{
			Suspend: &SuspendResult{
				Prompt:   e.Prompt,
				Reason:   reason,
				Metadata: e.Metadata,
				approval: true,
			},
		},
	}
}

// Errors returned by a resume call that supplies WithApprovals.
var (
	// ErrApprovalNotPending is returned when WithApprovals references a tool
	// call that is not awaiting approval (it never went through an approval
	// gate). Use WithToolResults for input-required pendings instead.
	ErrApprovalNotPending = errors.New("dive: tool call is not awaiting approval")

	// ErrApprovalConflict is returned when the same tool call id is supplied in
	// both WithToolResults and WithApprovals.
	ErrApprovalConflict = errors.New("dive: tool call id supplied as both a result and an approval")
)

// WithApprovals resolves approval-gated tool calls on resume. Keys are
// tool_call IDs from a prior Response.Suspension.PendingToolCalls whose
// AwaitingApproval is true; a true value approves the call (the tool is
// executed) and a false value rejects it (a denial result is injected and the
// tool never runs).
//
// Combine it with WithToolResults / WithResume in a single resume call to
// satisfy a mix of input-required and approval-gated pendings. Pendings left
// unresolved keep the turn suspended (partial resume). Supplying an id that is
// unknown, not awaiting approval, or also present in WithToolResults returns an
// error without mutating session state.
func WithApprovals(approvals map[string]bool) CreateResponseOption {
	return func(opts *CreateResponseOptions) {
		opts.Approvals = approvals
	}
}
