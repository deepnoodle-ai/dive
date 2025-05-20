package llm

import (
	"encoding/json"
	"fmt"
)

// Response is the generated response from an LLM. Matches the Anthropic
// response format documented here:
// https://docs.anthropic.com/en/api/messages#response-content
//
// In Dive, all LLM provider implementations must transform their responses into
// this type.
type Response struct {
	ID           string    `json:"id"`
	Model        string    `json:"model"`
	Role         Role      `json:"role"`
	Content      []Content `json:"content"`
	StopReason   string    `json:"stop_reason"`
	StopSequence *string   `json:"stop_sequence,omitempty"`
	Type         string    `json:"type"`
	Usage        Usage     `json:"usage"`
}

// Message extracts and returns the message from the response.
func (r *Response) Message() *Message {
	return &Message{
		ID:      r.ID,
		Role:    r.Role,
		Content: r.Content,
	}
}

// ToolCalls extracts and returns all tool calls from the response.
func (r *Response) ToolCalls() []*ToolCall {
	var toolCalls []*ToolCall
	for _, content := range r.Content {
		if toolUse, ok := content.(*ToolUseContent); ok {
			toolCalls = append(toolCalls, &ToolCall{
				ID:    toolUse.ID,    // e.g. "toolu_01A09q90qw90lq917835lq9"
				Name:  toolUse.Name,  // tool name e.g. "get_weather"
				Input: toolUse.Input, // tool call input (JSON as text)
			})
		}
	}
	return toolCalls
}

// UnmarshalJSON implements custom unmarshaling for Response to properly handle
// the polymorphic Content field.
func (r *Response) UnmarshalJSON(data []byte) error {
	type tempResponse struct {
		ID           string            `json:"id"`
		Model        string            `json:"model"`
		Role         Role              `json:"role"`
		Content      []json.RawMessage `json:"content"`
		StopReason   string            `json:"stop_reason"`
		StopSequence *string           `json:"stop_sequence,omitempty"`
		Type         string            `json:"type"`
		Usage        Usage             `json:"usage"`
	}

	// Unmarshal JSON into the temporary struct
	var tmp tempResponse
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	// Copy all fields except Content
	r.ID = tmp.ID
	r.Model = tmp.Model
	r.Role = tmp.Role
	r.StopReason = tmp.StopReason
	r.StopSequence = tmp.StopSequence
	r.Type = tmp.Type
	r.Usage = tmp.Usage

	// Process each content item
	r.Content = make([]Content, 0, len(tmp.Content))
	for _, rawContent := range tmp.Content {
		content, err := UnmarshalContent(r.Role, rawContent)
		if err != nil {
			return err
		}
		r.Content = append(r.Content, content)
	}
	return nil
}

type contentTypeIndicator struct {
	Type ContentType `json:"type"`
}

// UnmarshalContent unmarshals the JSON of one content block into the
// appropriate concrete Content type.
func UnmarshalContent(role Role, data []byte) (Content, error) {
	// Extract the type field
	var ct contentTypeIndicator
	if err := json.Unmarshal(data, &ct); err != nil {
		return nil, err
	}
	// Create and unmarshal the appropriate concrete type
	var content Content
	switch ct.Type {
	case ContentTypeText:
		if role == Assistant {
			content = &AssistantTextContent{}
		} else {
			content = &TextContent{}
		}
	case ContentTypeImage:
		content = &ImageContent{}
	case ContentTypeDocument:
		content = &DocumentContent{}
	case ContentTypeToolUse:
		content = &ToolUseContent{}
	case ContentTypeToolResult:
		content = &ToolResultContent{}
	case ContentTypeThinking:
		content = &ThinkingContent{}
	case ContentTypeRedactedThinking:
		content = &RedactedThinkingContent{}
	case ContentTypeServerToolUse:
		content = &ServerToolUseContent{}
	case ContentTypeWebSearchToolResult:
		content = &WebSearchToolResultContent{}
	default:
		return nil, fmt.Errorf("unsupported content type: %s", ct.Type)
	}
	// Unmarshal into the concrete type
	if err := json.Unmarshal(data, content); err != nil {
		return nil, err
	}
	fmt.Printf("unmarshalled content: %+v\n", content)
	return content, nil
}
