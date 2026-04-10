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
	// transitions into a suspended state. The Suspended field carries the
	// same pending/completed lists as Response. Stream consumers should
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
type PendingToolCall struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Prompt   string          `json:"prompt,omitempty"`
	Metadata map[string]any  `json:"metadata,omitempty"`
}

// CompletedToolCall describes a tool call that ran to completion in the same
// iteration where a sibling suspended. Informational — the result is already
// persisted in the session.
type CompletedToolCall struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Result *ToolResult     `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// SuspendedItem is the payload of a ResponseItemTypeSuspended stream event.
type SuspendedItem struct {
	PendingToolCalls   []*PendingToolCall   `json:"pending_tool_calls,omitempty"`
	CompletedToolCalls []*CompletedToolCall `json:"completed_tool_calls,omitempty"`
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

	// Suspended is set on a ResponseItemTypeSuspended item.
	Suspended *SuspendedItem `json:"suspended,omitempty"`

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

	// PendingToolCalls is populated when Status == ResponseStatusSuspended.
	// Each entry is a tool call awaiting an external result, which the caller
	// must supply on a subsequent CreateResponse via WithToolResults.
	PendingToolCalls []*PendingToolCall `json:"pending_tool_calls,omitempty"`

	// CompletedToolCalls lists tool calls that ran alongside the suspending
	// sibling(s) in the suspended iteration. Only populated when
	// Status == ResponseStatusSuspended. For the terminal iteration only.
	CompletedToolCalls []*CompletedToolCall `json:"completed_tool_calls,omitempty"`
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
