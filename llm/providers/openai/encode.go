package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diveagents/dive/llm"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"
)

func encodeMessages(messages []*llm.Message) ([]responses.ResponseInputItemUnionParam, error) {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(messages))
	for _, message := range messages {
		if len(message.Content) == 0 {
			continue
		}
		messageType, err := messageType(message)
		if err != nil {
			return nil, fmt.Errorf("error encoding message: %w", err)
		}
		switch messageType {
		case "assistant":
			outMessages, err := encodeAssistantMessage(message)
			if err != nil {
				return nil, fmt.Errorf("error encoding assistant message: %w", err)
			}
			items = append(items, outMessages...)
		case "user":
			outMessages, err := encodeUserMessage(message)
			if err != nil {
				return nil, fmt.Errorf("error encoding user message: %w", err)
			}
			items = append(items, outMessages...)
		case "tool_result":
			for _, c := range message.Content {
				toolResultContent, ok := c.(*llm.ToolResultContent)
				if !ok {
					return nil, fmt.Errorf("tool result mixed with other content")
				}
				outMessage, err := encodeToolResultContent(toolResultContent)
				if err != nil {
					return nil, fmt.Errorf("error encoding tool result message: %w", err)
				}
				items = append(items, *outMessage)
			}
		}
	}
	return items, nil
}

func messageType(message *llm.Message) (string, error) {
	if message.Role == llm.Assistant {
		return "assistant", nil
	}
	if message.Role != "" && message.Role != llm.User {
		return "", fmt.Errorf("unknown message role: %s", message.Role)
	}
	for _, c := range message.Content {
		if _, ok := c.(*llm.ToolResultContent); ok {
			return "tool_result", nil
		}
	}
	return "user", nil
}

func encodeAssistantMessage(message *llm.Message) ([]responses.ResponseInputItemUnionParam, error) {
	if message.Role != llm.Assistant {
		return nil, fmt.Errorf("message role is not assistant")
	}
	content := make([]responses.ResponseInputItemUnionParam, 0, len(message.Content))
	for _, c := range message.Content {
		encodedContent, err := encodeAssistantContent(c)
		if err != nil {
			return nil, fmt.Errorf("error encoding assistant content: %w", err)
		}
		content = append(content, encodedContent)
	}
	return content, nil
}

func encodeAssistantContent(content llm.Content) (responses.ResponseInputItemUnionParam, error) {
	switch c := content.(type) {
	case *llm.TextContent:
		return encodeAssistantTextContent(c)
	case *llm.RefusalContent:
		return encodeAssistantRefusalContent(c)
	case *llm.ImageContent:
		return encodeAssistantImageContent(c)
	case *llm.DocumentContent:
		return encodeAssistantDocumentContent(c)
	case *llm.FileContent:
		return encodeAssistantFileContent(c)
	case *llm.ToolUseContent:
		return encodeAssistantToolUseContent(c)
	case *llm.ToolResultContent:
		return encodeAssistantToolResultContent(c)
	case *llm.ServerToolUseContent:
		return encodeAssistantServerToolUseContent(c)
	case *llm.WebSearchToolResultContent:
		return encodeAssistantWebSearchToolResultContent(c)
	case *llm.ThinkingContent:
		return encodeAssistantThinkingContent(c)
	case *llm.RedactedThinkingContent:
		return encodeAssistantRedactedThinkingContent(c)
	case *llm.MCPToolUseContent:
		return encodeAssistantMCPToolUseContent(c)
	case *llm.MCPToolResultContent:
		return encodeAssistantMCPToolResultContent(c)
	case *llm.CodeExecutionToolResultContent:
		return encodeAssistantCodeExecutionToolResultContent(c)
	}
	return responses.ResponseInputItemUnionParam{}, fmt.Errorf("unsupported content type: %T", content)
}

func encodeAssistantTextContent(c *llm.TextContent) (responses.ResponseInputItemUnionParam, error) {
	content := []responses.ResponseOutputMessageContentUnionParam{
		{
			OfOutputText: &responses.ResponseOutputTextParam{
				Text: c.Text,
				Type: "output_text",
			},
		},
	}
	return responses.ResponseInputItemParamOfOutputMessage(content, "", ""), nil
}

func encodeAssistantImageContent(c *llm.ImageContent) (responses.ResponseInputItemUnionParam, error) {
	// The image content should have a generation ID, in reference to a previously generated image
	if c.Source == nil || c.Source.GenerationID == "" {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("image content has no generation id")
	}
	// Default status to "succeeded" if not specified
	status := c.Source.GenerationStatus
	if status == "" {
		status = "succeeded"
	}
	// Create an image generation call reference with empty result
	// This is used to reference a previously generated image
	return responses.ResponseInputItemParamOfImageGenerationCall(
		c.Source.GenerationID, // generation ID from the previous call
		"",                    // result left blank for references
		status,                // generation status
	), nil
}

func encodeAssistantToolUseContent(c *llm.ToolUseContent) (responses.ResponseInputItemUnionParam, error) {
	if c.Name == "" {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("tool use content name is empty")
	}
	return responses.ResponseInputItemParamOfFunctionCall(string(c.Input), c.ID, c.Name), nil
}

func encodeAssistantThinkingContent(c *llm.ThinkingContent) (responses.ResponseInputItemUnionParam, error) {
	summaryParam := responses.ResponseReasoningItemSummaryParam{
		Type: "summary_text",
		Text: c.Thinking,
	}
	reasoning := responses.ResponseReasoningItemParam{
		ID:               "", // Can be empty for assistant reasoning
		Summary:          []responses.ResponseReasoningItemSummaryParam{summaryParam},
		EncryptedContent: openai.String(c.Signature),
	}
	return responses.ResponseInputItemUnionParam{OfReasoning: &reasoning}, nil
}

func encodeAssistantRefusalContent(c *llm.RefusalContent) (responses.ResponseInputItemUnionParam, error) {
	content := []responses.ResponseOutputMessageContentUnionParam{
		{
			OfRefusal: &responses.ResponseOutputRefusalParam{
				Refusal: c.Text,
				Type:    "refusal",
			},
		},
	}
	return responses.ResponseInputItemParamOfOutputMessage(content, "", ""), nil
}

func encodeAssistantDocumentContent(c *llm.DocumentContent) (responses.ResponseInputItemUnionParam, error) {
	// Convert document content to a text message with document information
	var text strings.Builder
	if c.Title != "" {
		text.WriteString(fmt.Sprintf("Document: %s\n", c.Title))
	}
	if c.Context != "" {
		text.WriteString(fmt.Sprintf("Context: %s\n", c.Context))
	}
	if c.Source != nil && c.Source.Type == llm.ContentSourceTypeBase64 {
		text.WriteString("Document content available as base64 data")
	}

	content := []responses.ResponseOutputMessageContentUnionParam{
		{
			OfOutputText: &responses.ResponseOutputTextParam{
				Text: text.String(),
				Type: "output_text",
			},
		},
	}
	return responses.ResponseInputItemParamOfOutputMessage(content, "", ""), nil
}

func encodeAssistantFileContent(c *llm.FileContent) (responses.ResponseInputItemUnionParam, error) {
	// Convert file content to a text message with file information
	var text strings.Builder
	if c.Filename != "" {
		text.WriteString(fmt.Sprintf("File: %s\n", c.Filename))
	}
	if c.FileID != "" {
		text.WriteString(fmt.Sprintf("File ID: %s", c.FileID))
	} else if c.FileData != "" {
		text.WriteString("File data available")
	}

	content := []responses.ResponseOutputMessageContentUnionParam{
		{
			OfOutputText: &responses.ResponseOutputTextParam{
				Text: text.String(),
				Type: "output_text",
			},
		},
	}
	return responses.ResponseInputItemParamOfOutputMessage(content, "", ""), nil
}

func encodeAssistantToolResultContent(c *llm.ToolResultContent) (responses.ResponseInputItemUnionParam, error) {
	var output string
	switch content := c.Content.(type) {
	case string:
		output = content
	case []byte:
		output = string(content)
	default:
		resultJSON, err := json.Marshal(c.Content)
		if err != nil {
			return responses.ResponseInputItemUnionParam{}, fmt.Errorf("failed to marshal tool result: %v", err)
		}
		output = string(resultJSON)
	}

	param := responses.ResponseInputItemParamOfFunctionCallOutput(c.ToolUseID, output)
	return param, nil
}

func encodeAssistantServerToolUseContent(c *llm.ServerToolUseContent) (responses.ResponseInputItemUnionParam, error) {
	inputBytes, err := json.Marshal(c.Input)
	if err != nil {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("failed to marshal server tool input: %v", err)
	}
	return responses.ResponseInputItemParamOfFunctionCall(string(inputBytes), c.ID, c.Name), nil
}

func encodeAssistantWebSearchToolResultContent(c *llm.WebSearchToolResultContent) (responses.ResponseInputItemUnionParam, error) {
	// Encode as a web search call result with completed status
	return responses.ResponseInputItemParamOfWebSearchCall(c.ToolUseID, responses.ResponseFunctionWebSearchStatusCompleted), nil
}

func encodeAssistantRedactedThinkingContent(c *llm.RedactedThinkingContent) (responses.ResponseInputItemUnionParam, error) {
	reasoning := responses.ResponseReasoningItemParam{
		ID:               "",
		Summary:          []responses.ResponseReasoningItemSummaryParam{}, // Empty for redacted content
		EncryptedContent: openai.String(c.Data),
	}
	return responses.ResponseInputItemUnionParam{OfReasoning: &reasoning}, nil
}

func encodeAssistantMCPToolUseContent(c *llm.MCPToolUseContent) (responses.ResponseInputItemUnionParam, error) {
	// Since there's no direct MCP call function, encode as a regular function call
	return responses.ResponseInputItemParamOfFunctionCall(string(c.Input), c.ID, c.Name), nil
}

func encodeAssistantMCPToolResultContent(c *llm.MCPToolResultContent) (responses.ResponseInputItemUnionParam, error) {
	// Convert content chunks to output text
	var text strings.Builder
	for i, chunk := range c.Content {
		if i > 0 {
			text.WriteString("\n")
		}
		text.WriteString(chunk.Text)
	}

	output := text.String()
	if c.IsError {
		output = fmt.Sprintf("Error: %s", output)
	}

	// Encode as function call output since MCP function doesn't exist
	return responses.ResponseInputItemParamOfFunctionCallOutput(c.ToolUseID, output), nil
}

func encodeAssistantCodeExecutionToolResultContent(c *llm.CodeExecutionToolResultContent) (responses.ResponseInputItemUnionParam, error) {
	// Convert code execution result to function call output
	resultBytes, err := json.Marshal(c.Content)
	if err != nil {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("failed to marshal code execution result: %v", err)
	}
	return responses.ResponseInputItemParamOfFunctionCallOutput(c.ToolUseID, string(resultBytes)), nil
}

func encodeUserMessage(message *llm.Message) ([]responses.ResponseInputItemUnionParam, error) {
	if message.Role != llm.User {
		return nil, fmt.Errorf("message role is not user")
	}
	content := make([]responses.ResponseInputContentUnionParam, 0, len(message.Content))
	for _, c := range message.Content {
		encodedContent, err := encodeUserContent(c)
		if err != nil {
			return nil, fmt.Errorf("error encoding user content: %w", err)
		}
		content = append(content, *encodedContent)
	}
	var items []responses.ResponseInputItemUnionParam
	items = append(items, responses.ResponseInputItemParamOfInputMessage(content, "user"))
	for _, c := range message.Content {
		if thinking, ok := c.(*llm.ThinkingContent); ok {
			items = append(items, encodeReasoningContent(thinking))
		}
	}
	return items, nil
}

func encodeUserContent(content llm.Content) (*responses.ResponseInputContentUnionParam, error) {
	switch c := content.(type) {
	case *llm.TextContent:
		return encodeInputTextContent(c)
	case *llm.ImageContent:
		return encodeInputImageContent(c)
	case *llm.DocumentContent:
		return encodeInputDocumentContent(c)
	}
	return nil, fmt.Errorf("unsupported content type: %T", content)
}

func encodeInputTextContent(c *llm.TextContent) (*responses.ResponseInputContentUnionParam, error) {
	param := responses.ResponseInputContentParamOfInputText(c.Text)
	return &param, nil
}

func encodeReasoningContent(c *llm.ThinkingContent) responses.ResponseInputItemUnionParam {
	param := responses.ResponseReasoningItemParam{
		ID:               "",
		EncryptedContent: openai.String(c.Signature),
	}
	return responses.ResponseInputItemUnionParam{OfReasoning: &param}
}

func encodeInputImageContent(c *llm.ImageContent) (*responses.ResponseInputContentUnionParam, error) {
	if c.Source == nil {
		return nil, fmt.Errorf("image content source is required")
	}
	imageParam := responses.ResponseInputImageParam{
		Detail: responses.ResponseInputImageDetailAuto,
	}
	switch c.Source.Type {
	case llm.ContentSourceTypeBase64:
		if c.Source.MediaType == "" || c.Source.Data == "" {
			return nil, fmt.Errorf("media type and data are required for base64 image content")
		}
		// Create data URL for base64 image data
		dataURL := fmt.Sprintf("data:%s;base64,%s", c.Source.MediaType, c.Source.Data)
		imageParam.ImageURL = openai.String(dataURL)

	case llm.ContentSourceTypeFile:
		if c.Source.FileID == "" {
			return nil, fmt.Errorf("file ID is required for file-based image content")
		}
		imageParam.FileID = openai.String(c.Source.FileID)

	case llm.ContentSourceTypeURL:
		if c.Source.URL == "" {
			return nil, fmt.Errorf("URL is required for URL-based image content")
		}
		imageParam.ImageURL = openai.String(c.Source.URL)

	default:
		return nil, fmt.Errorf("unsupported content source type for image: %v", c.Source.Type)
	}
	return &responses.ResponseInputContentUnionParam{OfInputImage: &imageParam}, nil
}

func encodeInputDocumentContent(c *llm.DocumentContent) (*responses.ResponseInputContentUnionParam, error) {
	if c.Source == nil {
		return nil, fmt.Errorf("document content source is required")
	}
	var fileParam responses.ResponseInputFileParam

	// OpenAI requires a filename, so generate a default if one is not provided
	if c.Title != "" {
		fileParam.Filename = openai.String(c.Title)
	} else {
		fileParam.Filename = openai.String("document")
	}

	switch c.Source.Type {
	case llm.ContentSourceTypeBase64:
		if c.Source.MediaType == "" {
			return nil, fmt.Errorf("media type is required for base64 document content")
		}
		if c.Source.Data == "" {
			return nil, fmt.Errorf("data is required for base64 document content")
		}
		// Create data URL format expected by OpenAI
		dataURL := fmt.Sprintf("data:%s;base64,%s", c.Source.MediaType, c.Source.Data)
		fileParam.FileData = openai.String(dataURL)

	case llm.ContentSourceTypeFile:
		if c.Source.FileID == "" {
			return nil, fmt.Errorf("file ID is required for file-based document content")
		}
		fileParam.FileID = openai.String(c.Source.FileID)

	case llm.ContentSourceTypeURL:
		return nil, fmt.Errorf("url-based document content is not supported by openai")

	default:
		return nil, fmt.Errorf("unsupported content source type for document: %v", c.Source.Type)
	}
	return &responses.ResponseInputContentUnionParam{OfInputFile: &fileParam}, nil
}

func encodeToolResultContent(c *llm.ToolResultContent) (*responses.ResponseInputItemUnionParam, error) {
	if c.ToolUseID == "" {
		return nil, fmt.Errorf("tool use id is not set")
	}
	var output string

	// Handle different content types
	switch content := c.Content.(type) {
	case string:
		output = content
	case []byte:
		output = string(content)
	default:
		resultJSON, err := json.Marshal(c.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool result: %v", err)
		}
		output = string(resultJSON)
	}
	// Note the IsError field is unused here...

	param := responses.ResponseInputItemParamOfFunctionCallOutput(c.ToolUseID, output)
	return &param, nil
}
