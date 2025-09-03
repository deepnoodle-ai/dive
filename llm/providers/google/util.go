package google

import (
	"encoding/json"
	"fmt"

	"github.com/deepnoodle-ai/dive/llm"
	"google.golang.org/genai"
)

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
			content = append(content, &llm.ToolUseContent{
				ID:    fmt.Sprintf("call_%s", part.FunctionCall.Name),
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

	// Convert roles from Dive to Google format
	converted := make([]*llm.Message, len(messages))
	for i, message := range messages {
		role := message.Role
		// Google uses "user" and "model" instead of "user" and "assistant"
		if role == llm.Assistant {
			role = "model"
		}

		converted[i] = &llm.Message{
			Role:    role,
			Content: message.Content,
		}
	}

	return converted, nil
}
