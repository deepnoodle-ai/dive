package dive

import (
	"encoding/json"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

// ResponseItemType represents the type of response item emitted during response generation.
//
// Response items are delivered via the EventCallback during CreateResponse. They provide
// real-time visibility into the agent's activity including initialization, messages,
// tool calls, and streaming events.
type ResponseItemType string

const (
	// ResponseItemTypeMessage indicates a complete message is available from the agent.
	// The Message field contains the full assistant message including any tool calls.
	ResponseItemTypeMessage ResponseItemType = "message"

	// ResponseItemTypeToolCall indicates a tool call is about to be executed.
	// The ToolCall field contains the tool name and input parameters.
	ResponseItemTypeToolCall ResponseItemType = "tool_call"

	// ResponseItemTypeToolCallResult indicates a tool call has completed.
	// The ToolCallResult field contains the tool output or error.
	ResponseItemTypeToolCallResult ResponseItemType = "tool_call_result"

	// ResponseItemTypeModelEvent indicates a streaming event from the LLM.
	// The Event field contains the raw LLM event for real-time UI updates.
	ResponseItemTypeModelEvent ResponseItemType = "model_event"

	// ResponseItemTypeToolStream indicates streaming output from a tool during execution.
	// The ToolStream field contains the tool call ID and a chunk of text.
	ResponseItemTypeToolStream ResponseItemType = "tool_stream"

	// ResponseItemTypeSuspended is a terminal item emitted when the agent
	// transitions into a suspended state. The Suspension field carries the
	// same SuspensionState as Response.Suspension. Stream consumers should
	// treat this as end-of-stream and then observe Response.Status.
	ResponseItemTypeSuspended ResponseItemType = "suspended"
)

// ResponseStatus indicates the terminal state of a CreateResponse call.
type ResponseStatus string

const (
	// ResponseStatusCompleted is the default: the agent finished normally.
	// An empty Status is treated as Completed for backward compatibility.
	ResponseStatusCompleted ResponseStatus = "completed"

	// ResponseStatusSuspended means one or more tool calls in the final
	// iteration returned SuspendResult. The agent has persisted the partial
	// turn to its session and expects a future CreateResponse call with
	// WithToolResults to supply the missing tool outputs.
	ResponseStatusSuspended ResponseStatus = "suspended"
)

// PendingToolCall describes a tool call awaiting an external result.
//
// # Metadata type fidelity
//
// Metadata values are round-tripped through JSON whenever a SuspensionState
// is deep-copied (during LoadSuspension, partial resume, etc.) or persisted
// to disk. After a round trip, numeric values come back as float64 and custom
// struct types become generic map[string]any. Tool authors attaching
// structured metadata should expect this loss of type fidelity and stick to
// JSON-friendly values.
type PendingToolCall struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Prompt   string          `json:"prompt,omitempty"`
	Reason   SuspendReason   `json:"reason,omitempty"`
	Metadata map[string]any  `json:"metadata,omitempty"`

	// AwaitingApproval is true when this call was paused by a PreToolUse
	// hook BEFORE the tool executed (an approval gate, via SuspendForApproval).
	// The tool has NOT run. Resume it with WithApprovals: approving re-runs the
	// tool, rejecting injects a denial. Input-required pendings (the tool
	// returned a SuspendResult) are resumed with WithToolResults instead.
	AwaitingApproval bool `json:\"awaiting_approval,omitempty\"`
}

// UnmarshalInput decodes the pending call's Input JSON into the given
// destination. Convenience for the common pattern of reading the original
// tool arguments when handling a suspend out-of-band.
func (p *PendingToolCall) UnmarshalInput(into any) error {
	if p == nil || len(p.Input) == 0 {
		return nil
	}
	return json.Unmarshal(p.Input, into)
}

// DecodePendingInput decodes the pending call's Input JSON into a value of
// type T. Generic wrapper around PendingToolCall.UnmarshalInput for call
// sites that prefer a return value over an out-parameter.
func DecodePendingInput[T any](p *PendingToolCall) (T, error) {
	var out T
	err := p.UnmarshalInput(&out)
	return out, err
}

// CompletedToolCall describes a tool call that ran to completion in the same
// iteration where a sibling suspended. Informational — the result is already
// present in the turn's message history (and in the session, when one is
// used). Exposed on SuspensionState so a SaaS UI can render "here's what we
// finished before pausing for your input" without reconstructing from the
// raw messages.
//
// Result and Error describe the same completion in two distinct ways:
//
//   - Result is the ToolResult the tool returned; it may have IsError set
//     if the tool reported a normal error outcome.
//   - Error is a Go-level error string, set only when the tool's Call method
//     itself returned a non-nil error (tool panic, transport failure, etc).
//     Empty whenever Result is non-nil.
type CompletedToolCall struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Result *ToolResult     `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// SuspensionState is the payload describing a suspended turn. It is
// populated on Response.Suspension whenever a CreateResponse call
// interacted with suspend/resume logic and is handed back to the agent via
// WithResume on the next call.
//
// # When Response.Suspension is populated
//
// SuspensionState is set on the Response in two situations:
//
//  1. Status == ResponseStatusSuspended. The agent has paused mid-turn and
//     at least one entry is in PendingToolCalls. TurnMessages carries the
//     in-progress turn snapshot.
//  2. Status == ResponseStatusCompleted on a call that resumed a previously
//     suspended turn. PendingToolCalls is nil (all pending work was
//     resolved). TurnMessages carries the final merged turn so stateless
//     callers can flush it into their local history in one step without
//     having to reconcile a stale partial tool_result from their saved
//     state.
//
// It is NOT populated on a plain completion that never involved suspend or
// resume.
//
// # Stateless caller pattern
//
// A caller managing history themselves holds two values: their pre-turn
// history and the most recent Response.Suspension (or nil). The pattern:
//
//	resp, _ := agent.CreateResponse(ctx,
//	    dive.WithMessages(history...),
//	    dive.WithResume(saved, toolResults), // omit if no active turn
//	)
//	if resp.Suspension != nil {
//	    if len(resp.Suspension.PendingToolCalls) > 0 {
//	        saved = resp.Suspension                 // still suspended
//	    } else {
//	        history = append(history, resp.Suspension.TurnMessages...)
//	        saved = nil                              // turn committed
//	    }
//	}
//
// SuspendableSession implementations persist this state on the caller's
// behalf so session-backed callers only need to pass WithToolResults.
type SuspensionState struct {
	// PendingToolCalls are the tool calls awaiting external results. On
	// resume the caller must supply a ToolResult for each via
	// WithResume/WithToolResults; partial resumes are allowed and remaining
	// entries stay pending. Nil on a resume-completed SuspensionState.
	PendingToolCalls []*PendingToolCall `json:"pending_tool_calls,omitempty"`

	// CompletedToolCalls are tool calls that ran to completion in the same
	// iteration as a suspending sibling (or have since been supplied via a
	// resume call). Informational: their results are already in TurnMessages.
	CompletedToolCalls []*CompletedToolCall `json:"completed_tool_calls,omitempty"`

	// TurnMessages is the complete set of messages that belong to the
	// in-progress (or just-completed) turn, in order, including the user
	// input that kicked it off and every assistant/tool_result message
	// produced across all iterations of this turn. The agent rebuilds the
	// full conversation on resume as preHistory + TurnMessages.
	//
	// On a resume-completed SuspensionState, TurnMessages reflects the
	// final merged view (including the last merged tool_result and the
	// final assistant message), so stateless callers can append it to
	// their pre-turn history in one operation.
	TurnMessages []*llm.Message `json:"turn_messages,omitempty"`
}

// ResponseItem contains either a message, tool call, tool result, or LLM event.
// Multiple items may be generated in response to a single prompt.
type ResponseItem struct {
	// Type of the response item
	Type ResponseItemType `json:"type,omitempty"`

	// Event is set if the response item is an event
	Event *llm.Event `json:"event,omitempty"`

	// Message is set if the response item is a message
	Message *llm.Message `json:"message,omitempty"`

	// ToolCall is set if the response item is a tool call
	ToolCall *llm.ToolUseContent `json:"tool_call,omitempty"`

	// ToolCallResult is set if the response item is a tool call result
	ToolCallResult *ToolCallResult `json:"tool_call_result,omitempty"`

	// ToolStream is set if the response item is streaming tool output
	ToolStream *ToolStreamEvent `json:"tool_stream,omitempty"`

	// Suspension is set on a ResponseItemTypeSuspended item. It mirrors
	// Response.Suspension.
	Suspension *SuspensionState `json:"suspension,omitempty"`

	// Extension holds optional data from experimental packages.
	// The concrete type depends on the ResponseItemType.
	Extension any `json:"extension,omitempty"`

	// Usage contains token usage information, if applicable
	Usage *llm.Usage `json:"usage,omitempty"`
}

// ToolStreamEvent contains a chunk of streaming output from a tool.
type ToolStreamEvent struct {
	ToolCallID string `json:"tool_call_id"`
	Text       string `json:"text"`
}

// Response represents the output from an Agent's response generation.
type Response struct {
	// Model represents the model that generated the response
	Model string `json:"model,omitempty"`

	// Items contains the individual response items including
	// messages, tool calls, and tool results.
	Items []*ResponseItem `json:"items,omitempty"`

	// OutputMessages contains the messages generated during this response.
	// This includes assistant messages and tool result messages in the order
	// they were produced. Use these messages to continue a multi-turn
	// conversation: pass the original input messages plus OutputMessages
	// plus a new user message on the next call.
	OutputMessages []*llm.Message `json:"output_messages,omitempty"`

	// Usage contains token usage information
	Usage *llm.Usage `json:"usage,omitempty"`

	// CreatedAt is the timestamp when this response was created
	CreatedAt time.Time `json:"created_at,omitempty"`

	// FinishedAt is the timestamp when this response was completed
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// Status is ResponseStatusCompleted for normal returns, or
	// ResponseStatusSuspended when at least one tool returned SuspendResult.
	// An empty Status means Completed (back-compat).
	Status ResponseStatus `json:"status,omitempty"`

	// Suspension carries the suspend/resume turn snapshot. It is populated:
	//
	//   - When Status == ResponseStatusSuspended (the agent is paused
	//     mid-turn): Pending and Completed tool calls plus the in-progress
	//     turn messages.
	//   - When Status == ResponseStatusCompleted on a call that resumed a
	//     previously suspended turn: PendingToolCalls is nil, TurnMessages
	//     holds the final merged turn. Stateless callers flush by
	//     appending TurnMessages to their pre-turn history.
	//
	// It is nil on plain completions that did not involve suspend or
	// resume. See the SuspensionState docs for the canonical stateless
	// usage pattern.
	Suspension *SuspensionState `json:"suspension,omitempty"`

	// BackgroundTasks is non-nil and non-empty when one or more tools
	// returned BackgroundResult during this turn. Each handle provides a Done
	// channel that delivers exactly one *ToolResult when the background
	// goroutine finishes. Status remains ResponseStatusCompleted — background
	// tasks do not change the response's terminal status.
	//
	// Use AwaitBackgroundTasks to block for all results, then pass them to
	// the next CreateResponse call via WithBackgroundResults. For the simple
	// interactive loop case, use ContinueWithBackground.
	//
	// Handles may be dropped without causing goroutine leaks: the background
	// goroutine sends to its buffered channel (cap 1) and exits regardless of
	// whether any caller reads the result.
	BackgroundTasks []*BackgroundTaskHandle `json:"-"`
}

// OutputText returns the text content from the last message in the response.
// If there are no messages or no text content, returns an empty string.
func (r *Response) OutputText() string {
	// Find the last message
	var lastMessage *llm.Message
	for _, item := range r.Items {
		if item.Type == ResponseItemTypeMessage && item.Message != nil {
			lastMessage = item.Message
		}
	}

	if lastMessage == nil {
		return ""
	}

	// Find the last text content
	for i := len(lastMessage.Content) - 1; i >= 0; i-- {
		content := lastMessage.Content[i]
		if textContent, ok := content.(*llm.TextContent); ok {
			return textContent.Text
		}
	}

	return ""
}

// ToolCallResults returns all tool call results from the response.
func (r *Response) ToolCallResults() []*ToolCallResult {
	var results []*ToolCallResult
	for _, item := range r.Items {
		if item.Type == ResponseItemTypeToolCallResult {
			results = append(results, item.ToolCallResult)
		}
	}
	return results
}
