package llm

import (
	"encoding/json"
)

// Response is the generated response from an LLM. Matches the Anthropic
// response format documented here:
// https://docs.anthropic.com/en/api/messages#response-content
//
// In Dive, all LLM provider implementations must transform their responses into
// this type.
type Response struct {
	ID                string                     `json:"id"`
	Model             string                     `json:"model"`
	Role              Role                       `json:"role"`
	Content           []Content                  `json:"content"`
	StopReason        string                     `json:"stop_reason"`
	StopSequence      *string                    `json:"stop_sequence,omitempty"`
	StopDetails       *StopDetails               `json:"stop_details,omitempty"`
	Type              string                     `json:"type"`
	Usage             Usage                      `json:"usage"`
	ContextManagement *ContextManagementResponse `json:"context_management,omitempty"`
	// InputTransformations lists changes the provider made to the request
	// input before the model read it. Anthropic reports thinking blocks it
	// dropped or let through despite a mismatch, when the
	// thinking-binding-controls beta is on.
	InputTransformations []InputTransformation `json:"input_transformations,omitempty"`
}

// InputTransformation describes one change the provider made to the request
// input. Ignore entries whose Type or Reason you don't recognize; providers add
// values over time.
type InputTransformation struct {
	// Type is "thinking_dropped" (the model did not read the block) or
	// "thinking_mismatch_allowed" (the block failed a check the provider did
	// not enforce, and the model read it).
	Type string `json:"type"`
	// Path locates the block in the request as sent, such as
	// "messages.3.content.0". Indexes can differ from the caller's messages,
	// since providers drop empty messages before sending.
	Path string `json:"path,omitempty"`
	// Reason is "model_binding_mismatch" (the block came from another model)
	// or "prefix_binding_mismatch" (something before the block changed).
	Reason string `json:"reason,omitempty"`
}

// StopDetails provides additional structured detail about why generation
// stopped. It is populated alongside StopReason — most notably on refusals,
// where Type categorizes the kind of refusal so applications can route the
// user to the right next step.
type StopDetails struct {
	// Type categorizes the stop reason (e.g. the refusal category).
	Type string `json:"type,omitempty"`
}

// ContextManagementResponse contains information about applied context edits.
// This struct is populated if the API performed any context editing (e.g., clearing tool results).
type ContextManagementResponse struct {
	// AppliedEdits is a list of edits that were applied to the context.
	AppliedEdits []AppliedContextEdit `json:"applied_edits,omitempty"`

	// OriginalInputTokens is the token count before any context editing occurred.
	// This is useful for calculating token savings.
	OriginalInputTokens int `json:"original_input_tokens,omitempty"`
}

// AppliedContextEdit describes a context edit that was performed.
type AppliedContextEdit struct {
	// Type is the strategy type identifier (e.g., "clear_tool_uses_20250919").
	Type string `json:"type"`

	// ClearedToolUses is the number of tool use/result pairs that were cleared.
	// Populated for "clear_tool_uses_20250919" strategy.
	ClearedToolUses int `json:"cleared_tool_uses,omitempty"`

	// ClearedThinkingTurns is the number of thinking turns that were cleared.
	// Populated for "clear_thinking_20251015" strategy.
	ClearedThinkingTurns int `json:"cleared_thinking_turns,omitempty"`

	// ClearedInputTokens is the number of input tokens that were removed from the context.
	ClearedInputTokens int `json:"cleared_input_tokens,omitempty"`
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
func (r *Response) ToolCalls() []*ToolUseContent {
	var toolCalls []*ToolUseContent
	for _, content := range r.Content {
		if toolUse, ok := content.(*ToolUseContent); ok {
			toolCalls = append(toolCalls, &ToolUseContent{
				ID:          toolUse.ID,   // e.g. "toolu_01A09q90qw90lq917835lq9"
				Name:        toolUse.Name, // tool name e.g. "get_weather"
				ToolsetName: toolUse.ToolsetName,
				Input:       toolUse.Input, // tool call input JSON
				Metadata:    toolUse.Metadata.Clone(),
			})
		}
	}
	return toolCalls
}

// UnmarshalJSON implements custom unmarshaling for Response to properly handle
// the polymorphic Content field.
func (r *Response) UnmarshalJSON(data []byte) error {
	type tempResponse struct {
		ID                   string                     `json:"id"`
		Model                string                     `json:"model"`
		Role                 Role                       `json:"role"`
		Content              []json.RawMessage          `json:"content"`
		StopReason           string                     `json:"stop_reason"`
		StopSequence         *string                    `json:"stop_sequence,omitempty"`
		StopDetails          *StopDetails               `json:"stop_details,omitempty"`
		Type                 string                     `json:"type"`
		Usage                Usage                      `json:"usage"`
		ContextManagement    *ContextManagementResponse `json:"context_management,omitempty"`
		InputTransformations []InputTransformation      `json:"input_transformations,omitempty"`
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
	r.StopDetails = tmp.StopDetails
	r.Type = tmp.Type
	r.Usage = tmp.Usage
	r.ContextManagement = tmp.ContextManagement
	r.InputTransformations = tmp.InputTransformations

	// Process each content item
	r.Content = make([]Content, 0, len(tmp.Content))
	for _, rawContent := range tmp.Content {
		content, err := UnmarshalContent(rawContent)
		if err != nil {
			return err
		}
		r.Content = append(r.Content, content)
	}
	return nil
}
