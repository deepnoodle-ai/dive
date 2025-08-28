package dive

import (
	"context"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

type ResponseItemType string

const (
	ResponseItemTypeMessage        ResponseItemType = "message"
	ResponseItemTypeToolCall       ResponseItemType = "tool_call"
	ResponseItemTypeToolCallResult ResponseItemType = "tool_call_result"
)

// ResponseItem contains either a message, tool call, or tool result. Multiple
// items may be generated in response to a single prompt.
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

	// Usage contains token usage information, if applicable
	Usage *llm.Usage `json:"usage,omitempty"`
}

// Response represents the output from an Agent's response generation.
type Response struct {
	// ID is a unique identifier for this response
	ID string `json:"id,omitempty"`

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

// ResponseStream is a generic interface for streaming responses
type ResponseStream interface {
	// Next advances the stream to the next item
	Next(ctx context.Context) bool

	// Event returns the current event in the stream
	Event() *ResponseEvent

	// Err returns any error encountered while streaming
	Err() error

	// Close releases any resources associated with the stream
	Close() error
}
