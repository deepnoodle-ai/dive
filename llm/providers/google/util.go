package google

import (
	"encoding/json"
	"fmt"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
	"google.golang.org/genai"
)

// GoogleFunctionResponseContent is a special content type for handling tool results in Google format
type GoogleFunctionResponseContent struct {
	ToolUseID string `json:"tool_use_id"`
	Content   any    `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

func (c *GoogleFunctionResponseContent) Type() llm.ContentType {
	return "google_function_response"
}

// convertToolResultToFunctionResponse converts a GoogleFunctionResponseContent to a genai.Part with FunctionResponse
func convertToolResultToFunctionResponse(content *GoogleFunctionResponseContent) *genai.Part {
	// Extract function name from tool use ID (format: call_functionName_X)
	functionName := extractFunctionNameFromToolID(content.ToolUseID)

	// Convert the content to a map for the response
	var responseData map[string]any

	switch c := content.Content.(type) {
	case string:
		responseData = map[string]any{"result": c}
	case []*dive.ToolResultContent:
		// Combine all text results
		var textResults []string
		for _, toolResult := range c {
			if toolResult.Text != "" {
				textResults = append(textResults, toolResult.Text)
			}
		}
		if len(textResults) > 0 {
			responseData = map[string]any{"result": textResults}
		} else {
			responseData = map[string]any{"result": "No content"}
		}
	default:
		responseData = map[string]any{"result": fmt.Sprintf("%v", c)}
	}

	// Add error information if present
	if content.IsError {
		responseData["is_error"] = true
	}

	part := &genai.Part{
		FunctionResponse: &genai.FunctionResponse{
			Name:     functionName,
			Response: responseData,
		},
	}
	return part
}

// extractFunctionNameFromToolID extracts the function name from a tool use ID
// Expected format: call_functionName_X where X is a number
func extractFunctionNameFromToolID(toolID string) string {
	// Simple extraction - assumes format call_functionName_X
	if len(toolID) > 5 && toolID[:5] == "call_" {
		// Find the last underscore to separate the function name from the suffix
		lastUnderscore := -1
		for i := len(toolID) - 1; i >= 5; i-- {
			if toolID[i] == '_' {
				lastUnderscore = i
				break
			}
		}
		if lastUnderscore > 5 {
			return toolID[5:lastUnderscore]
		}
		// If no underscore found after "call_", return everything after "call_"
		return toolID[5:]
	}
	// Fallback: return the whole ID
	return toolID
}

// convertGoogleResponse converts a Google GenAI response to a Dive LLM response
func convertGoogleResponse(resp *genai.GenerateContentResponse) (*llm.Response, error) {
	if resp == nil || len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("empty response from Google GenAI")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil {
		return nil, fmt.Errorf("no content in response")
	}

	// Convert parts to Dive content
	var content []llm.Content
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			content = append(content, &llm.TextContent{Text: part.Text})
		} else if part.FunctionCall != nil {
			// Handle function calls - convert args to JSON
			args, err := json.Marshal(part.FunctionCall.Args)
			if err != nil {
				return nil, fmt.Errorf("error marshaling function call args: %w", err)
			}
			toolCallID := generateToolCallID(part.FunctionCall.Name)
			content = append(content, &llm.ToolUseContent{
				ID:    toolCallID,
				Name:  part.FunctionCall.Name,
				Input: json.RawMessage(args),
			})
		} else {
			// Handle other types as text (fallback)
			content = append(content, &llm.TextContent{Text: fmt.Sprintf("%v", part)})
		}
	}

	// Convert usage information
	var usage llm.Usage
	if resp.UsageMetadata != nil {
		usage = llm.Usage{
			InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
			OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
		}
	}

	diveResponse := &llm.Response{
		ID:      fmt.Sprintf("google_%d", candidate.Index),
		Model:   "gemini", // This would be set properly in a real implementation
		Role:    llm.Assistant,
		Content: content,
		Type:    "text",
		Usage:   usage,
	}

	// Set stop reason
	if candidate.FinishReason == genai.FinishReasonStop {
		diveResponse.StopReason = "stop"
	} else if candidate.FinishReason == genai.FinishReasonMaxTokens {
		diveResponse.StopReason = "max_tokens"
	} else {
		diveResponse.StopReason = "other"
	}

	return diveResponse, nil
}

// generateToolCallID generates a consistent ID for tool calls
func generateToolCallID(toolName string) string {
	return fmt.Sprintf("call_%s_%d", toolName, len(toolName))
}

// convertToolUseToFunctionCall converts a Dive ToolUseContent back to Google FunctionCall format
func convertToolUseToFunctionCall(toolUse *llm.ToolUseContent) *genai.Part {
	if toolUse == nil {
		return nil
	}

	// Parse the input JSON to a map
	var args map[string]any
	if len(toolUse.Input) > 0 {
		if err := json.Unmarshal(toolUse.Input, &args); err != nil {
			fmt.Printf("Error unmarshaling tool input: %v\n", err)
			args = map[string]any{}
		}
	} else {
		args = map[string]any{}
	}

	return &genai.Part{
		FunctionCall: &genai.FunctionCall{
			Name: toolUse.Name,
			Args: args,
		},
	}
}

// convertMessages converts Dive messages to the format expected by Google GenAI
func convertMessages(messages []*llm.Message) ([]*llm.Message, error) {
	messageCount := len(messages)
	if messageCount == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	for i, message := range messages {
		if len(message.Content) == 0 {
			return nil, fmt.Errorf("empty message detected (index %d)", i)
		}
	}

	// Convert roles from Dive to Google format and handle tool results
	converted := make([]*llm.Message, len(messages))
	for i, message := range messages {
		role := message.Role
		// Google uses "user" and "model" instead of "user" and "assistant"
		if role == llm.Assistant {
			role = "model"
		}

		// Handle tool result messages - they should be sent as user messages in Google
		convertedContent := make([]llm.Content, 0, len(message.Content))

		for _, content := range message.Content {
			if toolResult, ok := content.(*llm.ToolResultContent); ok {
				role = llm.User // Tool results are always sent as user messages

				// Convert ToolResultContent to a format that can be handled properly
				// We'll create a special GoogleFunctionResponseContent that gets converted later
				convertedContent = append(convertedContent, &GoogleFunctionResponseContent{
					ToolUseID: toolResult.ToolUseID,
					Content:   toolResult.Content,
					IsError:   toolResult.IsError,
				})
			} else {
				convertedContent = append(convertedContent, content)
			}
		}

		converted[i] = &llm.Message{
			Role:    role,
			Content: convertedContent,
		}
	}

	return converted, nil
}

// convertAnySchemaToGenAI converts any schema to Google GenAI schema format
func convertAnySchemaToGenAI(inputSchema any) *genai.Schema {
	if diveSchema, ok := inputSchema.(*schema.Schema); ok {
		return convertSchemaToGenAI(diveSchema)
	}
	return nil
}

// convertSchemaToGenAI converts a Dive schema to Google GenAI schema format
func convertSchemaToGenAI(diveSchema *schema.Schema) *genai.Schema {
	if diveSchema == nil {
		return nil
	}

	genaiSchema := &genai.Schema{
		Type:        genai.Type(diveSchema.Type),
		Description: diveSchema.Description,
	}

	// Convert properties
	if diveSchema.Properties != nil {
		genaiSchema.Properties = make(map[string]*genai.Schema)
		for name, prop := range diveSchema.Properties {
			genaiSchema.Properties[name] = convertPropertyToGenAI(prop)
		}
	}

	// Convert required fields
	if len(diveSchema.Required) > 0 {
		genaiSchema.Required = diveSchema.Required
	}

	return genaiSchema
}

// convertPropertyToGenAI converts a Dive schema property to Google GenAI schema format
func convertPropertyToGenAI(prop *schema.Property) *genai.Schema {
	if prop == nil {
		return nil
	}

	genaiSchema := &genai.Schema{
		Type:        genai.Type(prop.Type),
		Description: prop.Description,
	}

	// Handle enum values
	if len(prop.Enum) > 0 {
		enumValues := make([]string, len(prop.Enum))
		copy(enumValues, prop.Enum)
		genaiSchema.Enum = enumValues
	}

	// Handle array items
	if prop.Items != nil {
		genaiSchema.Items = convertPropertyToGenAI(prop.Items)
	}

	// Handle nested properties for object types
	if prop.Properties != nil {
		genaiSchema.Properties = make(map[string]*genai.Schema)
		for name, nestedProp := range prop.Properties {
			genaiSchema.Properties[name] = convertPropertyToGenAI(nestedProp)
		}
	}

	// Convert required fields
	if len(prop.Required) > 0 {
		genaiSchema.Required = prop.Required
	}

	return genaiSchema
}
