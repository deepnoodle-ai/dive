package dive

import (
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
)

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

	// Extension holds optional data from experimental packages.
	// The concrete type depends on the ResponseItemType.
	Extension any `json:"extension,omitempty"`

	// Usage contains token usage information, if applicable
	Usage *llm.Usage `json:"usage,omitempty"`
}

// Response represents the output from an Agent's response generation.
type Response struct {
	// Model represents the model that generated the response
	Model string `json:"model,omitempty"`

	// Items contains the individual response items including
	// messages, tool calls, and tool results.
	Items []*ResponseItem `json:"items,omitempty"`

	// Usage contains token usage information
	Usage *llm.Usage `json:"usage,omitempty"`

	// CreatedAt is the timestamp when this response was created
	CreatedAt time.Time `json:"created_at,omitempty"`

	// FinishedAt is the timestamp when this response was completed
	FinishedAt *time.Time `json:"finished_at,omitempty"`
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
