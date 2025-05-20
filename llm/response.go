package llm

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
				ID:    toolUse.ID,            // e.g. "toolu_01A09q90qw90lq917835lq9"
				Name:  toolUse.Name,          // tool name e.g. "get_weather"
				Input: string(toolUse.Input), // tool call input (JSON as text)
			})
		}
	}
	return toolCalls
}
