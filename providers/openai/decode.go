package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/openai/openai-go/responses"
)

func decodeAssistantResponse(response *responses.Response) (*llm.Response, error) {
	// Initialize as empty slice to ensure Content is never nil
	contentBlocks := make([]llm.Content, 0)

	for _, item := range response.Output {
		decodedContent, err := decodeResponseItem(item)
		if err != nil {
			return nil, fmt.Errorf("error decoding response item: %w", err)
		}
		if decodedContent != nil {
			contentBlocks = append(contentBlocks, decodedContent...)
		}
	}

	usage := llm.Usage{}
	usage.InputTokens = int(response.Usage.InputTokens)
	usage.OutputTokens = int(response.Usage.OutputTokens)
	usage.CacheReadInputTokens = int(response.Usage.InputTokensDetails.CachedTokens)

	// Determine stop reason based on the response content and status
	stopReason := determineStopReason(response)

	return &llm.Response{
		ID:         response.ID,
		Model:      string(response.Model),
		Role:       llm.Assistant,
		Content:    contentBlocks,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

func decodeResponseItem(item responses.ResponseOutputItemUnion) ([]llm.Content, error) {
	switch item.Type {
	case "message":
		return decodeMessageContent(item.AsMessage())
	case "function_call":
		return decodeFunctionCallContent(item.AsFunctionCall())
	case "image_generation_call":
		return decodeImageGenerationCallContent(item.AsImageGenerationCall())
	case "web_search_call":
		return decodeWebSearchCallContent(item.AsWebSearchCall())
	case "mcp_call":
		return decodeMcpCallContent(item.AsMcpCall())
	case "mcp_list_tools":
		return decodeMcpListToolsContent(item.AsMcpListTools())
	case "mcp_approval_request":
		return decodeMcpApprovalRequestContent(item.AsMcpApprovalRequest())
	case "reasoning":
		return decodeReasoningContent(item.AsReasoning())
	case "file_search_call":
		return decodeFileSearchCallContent(item.AsFileSearchCall())
	case "computer_call":
		return decodeComputerCallContent(item.AsComputerCall())
	case "code_interpreter_call":
		return decodeCodeInterpreterCallContent(item.AsCodeInterpreterCall())
	case "local_shell_call":
		return decodeLocalShellCallContent(item.AsLocalShellCall())
	default:
		// Unknown item type, skip silently
		return nil, nil
	}
}

func decodeMessageContent(outputMsg responses.ResponseOutputMessage) ([]llm.Content, error) {
	var contentBlocks []llm.Content
	for _, content := range outputMsg.Content {
		switch content.Type {
		case "output_text":
			outputText := content.AsOutputText()
			textContent := &llm.TextContent{
				Text: outputText.Text,
			}
			// Convert OpenAI annotations to web_search_result_location citations
			if len(outputText.Annotations) > 0 {
				citations := make([]llm.Citation, 0, len(outputText.Annotations))
				for _, annotation := range outputText.Annotations {
					switch annotation.Type {
					case "url_citation":
						urlCitation := annotation.AsURLCitation()
						citations = append(citations, &llm.WebSearchResultLocation{
							Type:  "web_search_result_location",
							URL:   urlCitation.URL,
							Title: urlCitation.Title,
							// StartIndex: int(urlCitation.StartIndex),
							// EndIndex:   int(urlCitation.EndIndex),
						})
					}
				}
				textContent.Citations = citations
			}
			contentBlocks = append(contentBlocks, textContent)
		case "refusal":
			contentBlocks = append(contentBlocks, &llm.RefusalContent{
				Text: content.AsRefusal().Refusal,
			})
		}
	}
	return contentBlocks, nil
}

func decodeFunctionCallContent(functionCall responses.ResponseFunctionToolCall) ([]llm.Content, error) {
	return []llm.Content{
		&llm.ToolUseContent{
			ID:    functionCall.CallID,
			Name:  functionCall.Name,
			Input: []byte(functionCall.Arguments),
		},
	}, nil
}

func decodeImageGenerationCallContent(imgCall responses.ResponseOutputItemImageGenerationCall) ([]llm.Content, error) {
	if imgCall.Result == "" {
		return nil, nil
	}
	imageType, err := llm.DetectImageType(imgCall.Result)
	if err != nil {
		// PNG is the default for OpenAI, so we'll use that if we
		// can't detect the type. Sadly, the OpenAI response doesn't
		// just include the image type in this block.
		imageType = llm.ImageTypePNG
	}
	return []llm.Content{
		&llm.ImageContent{
			Source: &llm.ContentSource{
				Type:             llm.ContentSourceTypeBase64,
				GenerationID:     imgCall.ID,
				GenerationStatus: imgCall.Status,
				MediaType:        string(imageType),
				Data:             imgCall.Result,
			},
		},
	}, nil
}

func decodeWebSearchCallContent(call responses.ResponseFunctionWebSearch) ([]llm.Content, error) {
	// https://platform.openai.com/docs/guides/tools-web-search?api-mode=responses
	return []llm.Content{
		&llm.ServerToolUseContent{
			ID:   call.ID,
			Name: "web_search_call",
		},
	}, nil
}

func decodeMcpCallContent(mcpCall responses.ResponseOutputItemMcpCall) ([]llm.Content, error) {
	var contentBlocks []llm.Content

	// Always create the tool use content block
	toolUseContent := &llm.MCPToolUseContent{
		ID:         mcpCall.ID,
		Name:       mcpCall.Name,
		ServerName: mcpCall.ServerLabel,
		Input:      []byte(mcpCall.Arguments),
	}
	contentBlocks = append(contentBlocks, toolUseContent)

	// Create tool result content block if there's output or error
	if mcpCall.Output != "" || mcpCall.Error != "" {
		var resultContent []*llm.ContentChunk
		isError := false

		if mcpCall.Error != "" {
			// If there's an error, add it as text content and mark as error
			resultContent = append(resultContent, &llm.ContentChunk{
				Type: "text",
				Text: mcpCall.Error,
			})
			isError = true
		} else if mcpCall.Output != "" {
			// If there's output, add it as text content
			resultContent = append(resultContent, &llm.ContentChunk{
				Type: "text",
				Text: mcpCall.Output,
			})
		}

		toolResultContent := &llm.MCPToolResultContent{
			ToolUseID: mcpCall.ID,
			IsError:   isError,
			Content:   resultContent,
		}
		contentBlocks = append(contentBlocks, toolResultContent)
	}

	return contentBlocks, nil
}

func decodeMcpListToolsContent(mcpList responses.ResponseOutputItemMcpListTools) ([]llm.Content, error) {
	tools := make([]*llm.MCPToolDefinition, 0, len(mcpList.Tools))
	for _, tool := range mcpList.Tools {
		toolDef := &llm.MCPToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
		}
		// Include the input schema if available
		if tool.InputSchema != nil {
			// Convert the input schema to map[string]interface{}
			schemaBytes, err := json.Marshal(tool.InputSchema)
			if err == nil {
				var schemaMap map[string]interface{}
				if err := json.Unmarshal(schemaBytes, &schemaMap); err == nil {
					toolDef.InputSchema = schemaMap
				}
			}
		}
		tools = append(tools, toolDef)
	}
	return []llm.Content{
		&llm.MCPListToolsContent{
			ServerLabel: mcpList.ServerLabel,
			Tools:       tools,
		},
	}, nil
}

func decodeMcpApprovalRequestContent(mcpApproval responses.ResponseOutputItemMcpApprovalRequest) ([]llm.Content, error) {
	return []llm.Content{
		&llm.MCPApprovalRequestContent{
			ID:          mcpApproval.ID,
			Arguments:   mcpApproval.Arguments,
			Name:        mcpApproval.Name,
			ServerLabel: mcpApproval.ServerLabel,
		},
	}, nil
}

func decodeReasoningContent(reasoning responses.ResponseReasoningItem) ([]llm.Content, error) {
	var summaryItems []string
	for _, summary := range reasoning.Summary {
		summaryItems = append(summaryItems, summary.Text)
	}
	// Do we need to capture the reasoning.ID field?
	return []llm.Content{
		&llm.ThinkingContent{
			ID:        reasoning.ID,
			Thinking:  strings.Join(summaryItems, "\n\n"),
			Signature: reasoning.EncryptedContent,
		},
	}, nil
}

func decodeCodeInterpreterCallContent(codeCall responses.ResponseCodeInterpreterToolCall) ([]llm.Content, error) {
	// Convert results from ResponseCodeInterpreterToolCall to CodeInterpreterCallResult
	var results []CodeInterpreterCallResult
	for _, result := range codeCall.Outputs {
		callResult := CodeInterpreterCallResult{Type: result.Type}
		switch result.Type {
		case "logs":
			logs := result.AsLogs()
			callResult.Logs = logs.Logs
		}
		results = append(results, callResult)
	}
	return []llm.Content{
		&CodeInterpreterCallContent{
			ID:          codeCall.ID,
			Code:        codeCall.Code,
			Results:     results,
			Status:      string(codeCall.Status),
			ContainerID: codeCall.ContainerID,
		},
	}, nil
}

func decodeFileSearchCallContent(fileSearchCall responses.ResponseFileSearchToolCall) ([]llm.Content, error) {
	return nil, fmt.Errorf("file search call is not yet supported")
}

func decodeComputerCallContent(computerCall responses.ResponseComputerToolCall) ([]llm.Content, error) {
	return nil, fmt.Errorf("computer call is not yet supported")
}

func decodeLocalShellCallContent(shellCall responses.ResponseOutputItemLocalShellCall) ([]llm.Content, error) {
	return nil, fmt.Errorf("local shell call is not yet supported")
}
