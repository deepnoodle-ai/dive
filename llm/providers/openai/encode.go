package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
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

// findAndEncodeMCPPair looks for an MCPToolResultContent corresponding to the
// given MCPToolUseContent in the subsequent content items. If found, it encodes
// them as a single MCP call parameter and returns the parameter along with the
// index of the processed result. If no corresponding result is found, it encodes
// only the MCPToolUseContent.
func findAndEncodeMCPPair(
	mcpToolUse *llm.MCPToolUseContent,
	contentList []llm.Content,
	currentIndex int,
	processed map[int]bool,
) (responses.ResponseInputItemUnionParam, int, error) {
	mcpCallParam := &responses.ResponseInputItemMcpCallParam{
		ID:          mcpToolUse.ID,
		Arguments:   string(mcpToolUse.Input),
		Name:        mcpToolUse.Name,
		ServerLabel: mcpToolUse.ServerName,
	}
	pairedResultIndex := -1

	// Look for corresponding MCPToolResultContent
	for j := currentIndex + 1; j < len(contentList); j++ {
		if processed[j] {
			continue
		}
		if result, ok := contentList[j].(*llm.MCPToolResultContent); ok && result.ToolUseID == mcpToolUse.ID {
			var text strings.Builder
			for k, chunk := range result.Content {
				if k > 0 {
					text.WriteString("\n")
				}
				text.WriteString(chunk.Text)
			}
			if result.IsError {
				mcpCallParam.Error = openai.String(text.String())
			} else {
				mcpCallParam.Output = openai.String(text.String())
			}
			pairedResultIndex = j
			break
		}
	}
	return responses.ResponseInputItemUnionParam{OfMcpCall: mcpCallParam}, pairedResultIndex, nil
}

func encodeAssistantMessage(message *llm.Message) ([]responses.ResponseInputItemUnionParam, error) {
	if message.Role != llm.Assistant {
		return nil, fmt.Errorf("message role is not assistant")
	}
	encodedItems := make([]responses.ResponseInputItemUnionParam, 0, len(message.Content))

	// Track which content items we've already processed (for MCP pairing)
	processed := make(map[int]bool)

	for i, c := range message.Content {
		if processed[i] {
			continue // Skip if already processed
		}
		if mcpToolUse, ok := c.(*llm.MCPToolUseContent); ok {
			// Handle MCP tool use, potentially pairing it with a result
			mcpCallParam, pairedResultIndex, err := findAndEncodeMCPPair(mcpToolUse, message.Content, i, processed)
			if err != nil {
				return nil, fmt.Errorf("error encoding MCP pair for tool use ID %s: %w", mcpToolUse.ID, err)
			}
			encodedItems = append(encodedItems, mcpCallParam)
			processed[i] = true // Mark the MCPToolUseContent as processed
			if pairedResultIndex != -1 {
				processed[pairedResultIndex] = true // Mark the corresponding MCPToolResultContent as processed
			}
		} else if _, ok := c.(*llm.MCPToolResultContent); ok {
			// If we encounter an MCPToolResultContent that hasn't been processed
			// as part of a pair, it means it's an orphaned result. The current
			// OpenAI spec implies results are always paired with a preceding
			// use, so this case might indicate an issue or an unexpected message
			// structure. For now, we'll skip encoding it directly as it should
			// have been handled by findAndEncodeMCPPair.
			// If strict adherence to pairing is required, an error could be
			// returned here.
			processed[i] = true // Mark as processed to avoid re-evaluation
			continue
		} else {
			// Handle all other content types normally
			encodedContent, err := encodeAssistantContent(c)
			if err != nil {
				return nil, fmt.Errorf("error encoding assistant content type %T: %w", c, err)
			}
			encodedItems = append(encodedItems, encodedContent)
			processed[i] = true
		}
	}
	return encodedItems, nil
}

func encodeAssistantContent(content llm.Content) (responses.ResponseInputItemUnionParam, error) {
	switch c := content.(type) {
	case *llm.TextContent:
		return encodeAssistantTextContent(c)
	case *llm.ImageContent:
		return encodeAssistantImageContent(c)
	case *llm.ToolUseContent:
		return encodeAssistantToolUseContent(c)
	case *llm.ToolResultContent:
		return encodeAssistantToolResultContent(c)
	case *llm.ServerToolUseContent:
		return encodeAssistantServerToolUseContent(c)
	case *llm.ThinkingContent:
		return encodeAssistantThinkingContent(c)
	case *CodeInterpreterCallContent:
		return encodeAssistantCodeInterpreterCallContent(c)
	case *llm.RefusalContent:
		return encodeAssistantRefusalContent()
	case *llm.MCPToolUseContent:
		// MCP content is handled at the message level in encodeAssistantMessage
		return responses.ResponseInputItemUnionParam{},
			fmt.Errorf("MCPToolUseContent processing error")
	case *llm.MCPToolResultContent:
		// MCP content is handled at the message level in encodeAssistantMessage
		return responses.ResponseInputItemUnionParam{},
			fmt.Errorf("MCPToolResultContent processing error")
	}
	return responses.ResponseInputItemUnionParam{},
		fmt.Errorf("unsupported assistant content type: %T", content)
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
	// Create an image generation call reference with empty result
	// This is used to reference a previously generated image
	return responses.ResponseInputItemParamOfImageGenerationCall(
		c.Source.GenerationID,
		c.Source.Data,
		c.Source.GenerationStatus,
	), nil
}

func encodeAssistantToolUseContent(c *llm.ToolUseContent) (responses.ResponseInputItemUnionParam, error) {
	if c.Name == "" {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("tool use content name is empty")
	}
	return responses.ResponseInputItemParamOfFunctionCall(string(c.Input), c.ID, c.Name), nil
}

func encodeAssistantThinkingContent(c *llm.ThinkingContent) (responses.ResponseInputItemUnionParam, error) {
	return responses.ResponseInputItemUnionParam{
		OfReasoning: &responses.ResponseReasoningItemParam{
			ID: c.ID,
			Summary: []responses.ResponseReasoningItemSummaryParam{
				{
					Type: "summary_text",
					Text: c.Thinking,
				},
			},
			EncryptedContent: openai.String(c.Signature),
		},
	}, nil
}

func encodeAssistantRefusalContent() (responses.ResponseInputItemUnionParam, error) {
	return responses.ResponseInputItemUnionParam{}, errors.New("cannot proceed: refusal detected")
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
	switch c.Name {
	case "web_search_call":
		return responses.ResponseInputItemParamOfWebSearchCall(
			responses.ResponseFunctionWebSearchActionSearchParam{
				Query: "", // Empty query for completed search
				Type:  "search",
			},
			c.ID,
			responses.ResponseFunctionWebSearchStatusCompleted,
		), nil
	}
	return responses.ResponseInputItemUnionParam{},
		fmt.Errorf("unsupported server tool use name: %s", c.Name)
}

func encodeAssistantCodeInterpreterCallContent(c *CodeInterpreterCallContent) (responses.ResponseInputItemUnionParam, error) {
	param := responses.ResponseCodeInterpreterToolCallParam{}
	param.ID = c.ID
	param.Code = openai.String(c.Code)
	param.Status = responses.ResponseCodeInterpreterToolCallStatus(c.Status)
	param.ContainerID = c.ContainerID
	for _, result := range c.Results {
		switch result.Type {
		case "logs":
			param.Outputs = append(param.Outputs, responses.ResponseCodeInterpreterToolCallOutputUnionParam{
				OfLogs: &responses.ResponseCodeInterpreterToolCallOutputLogsParam{
					Logs: result.Logs,
				},
			})
		}
	}
	return responses.ResponseInputItemUnionParam{OfCodeInterpreterCall: &param}, nil
}

func encodeUserMessage(message *llm.Message) ([]responses.ResponseInputItemUnionParam, error) {
	if message.Role != llm.User {
		return nil, fmt.Errorf("message role is not user")
	}
	contentItems := make([]responses.ResponseInputContentUnionParam, 0, len(message.Content))
	var standaloneItems []responses.ResponseInputItemUnionParam

	for _, c := range message.Content {
		switch typedContent := c.(type) {
		case *llm.MCPApprovalRequestContent:
			item := responses.ResponseInputItemUnionParam{
				OfMcpApprovalRequest: &responses.ResponseInputItemMcpApprovalRequestParam{
					ID:          typedContent.ID,
					Arguments:   typedContent.Arguments,
					Name:        typedContent.Name,
					ServerLabel: typedContent.ServerLabel,
				},
			}
			standaloneItems = append(standaloneItems, item)
		case *llm.MCPApprovalResponseContent:
			approvalResponseParam := &responses.ResponseInputItemMcpApprovalResponseParam{
				ApprovalRequestID: typedContent.ApprovalRequestID,
				Approve:           typedContent.Approve,
			}
			if typedContent.Reason != "" {
				approvalResponseParam.Reason = openai.String(typedContent.Reason)
			}
			item := responses.ResponseInputItemUnionParam{
				OfMcpApprovalResponse: approvalResponseParam,
			}
			standaloneItems = append(standaloneItems, item)
		case *llm.ThinkingContent:
			// ThinkingContent is also handled as a standalone item (Reasoning)
			standaloneItems = append(standaloneItems, encodeReasoningContent(typedContent))
		default:
			encodedContent, err := encodeUserContent(c)
			if err != nil {
				return nil, fmt.Errorf("error encoding user content: %w", err)
			}
			if encodedContent != nil { // encodeUserContent might return nil for types it doesn't handle as content parts
				contentItems = append(contentItems, *encodedContent)
			}
		}
	}

	var items []responses.ResponseInputItemUnionParam
	if len(contentItems) > 0 {
		items = append(items, responses.ResponseInputItemParamOfInputMessage(contentItems, "user"))
	}
	items = append(items, standaloneItems...)
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
	case *llm.MCPApprovalRequestContent, *llm.MCPApprovalResponseContent, *llm.ThinkingContent:
		return nil, nil // Indicate that this content type is handled at a higher level
	}
	return nil, fmt.Errorf("unsupported content type for user message content part: %T", content)
}

func encodeInputTextContent(c *llm.TextContent) (*responses.ResponseInputContentUnionParam, error) {
	param := responses.ResponseInputContentParamOfInputText(c.Text)
	return &param, nil
}

func encodeReasoningContent(c *llm.ThinkingContent) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemUnionParam{
		OfReasoning: &responses.ResponseReasoningItemParam{
			ID:               "",
			EncryptedContent: openai.String(c.Signature),
		},
	}
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
