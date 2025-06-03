package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diveagents/dive/llm"
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
	// Web search call is a tool invocation, not a result
	return []llm.Content{
		&llm.ToolUseContent{
			ID:    call.ID,
			Name:  "web_search",
			Input: []byte("{}"), // OpenAI web search calls don't include the query in the call object
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
			Thinking:  strings.Join(summaryItems, "\n\n"),
			Signature: reasoning.EncryptedContent,
		},
	}, nil
}

func decodeFileSearchCallContent(fileSearchCall responses.ResponseFileSearchToolCall) ([]llm.Content, error) {
	// Use the actual queries from the file search call
	var inputBytes []byte
	if len(fileSearchCall.Queries) > 0 {
		queriesJSON, err := json.Marshal(map[string]interface{}{
			"queries": fileSearchCall.Queries,
		})
		if err != nil {
			return nil, fmt.Errorf("error marshaling file search queries: %w", err)
		}
		inputBytes = queriesJSON
	} else {
		inputBytes = []byte("{}")
	}

	return []llm.Content{
		&llm.ToolUseContent{
			ID:    fileSearchCall.ID,
			Name:  "file_search",
			Input: inputBytes,
		},
	}, nil
}

func decodeComputerCallContent(computerCall responses.ResponseComputerToolCall) ([]llm.Content, error) {
	// Convert Action to JSON for Input field
	actionBytes, err := json.Marshal(computerCall.Action)
	if err != nil {
		return nil, fmt.Errorf("error marshaling computer action: %w", err)
	}
	return []llm.Content{
		&llm.ToolUseContent{
			ID:    computerCall.ID,
			Name:  "computer",
			Input: actionBytes,
		},
	}, nil
}

func decodeCodeInterpreterCallContent(codeCall responses.ResponseCodeInterpreterToolCall) ([]llm.Content, error) {
	// Use the actual code for Input, not the results
	var inputBytes []byte
	if codeCall.Code != "" {
		codeJSON, err := json.Marshal(map[string]interface{}{
			"code": codeCall.Code,
		})
		if err != nil {
			return nil, fmt.Errorf("error marshaling code interpreter code: %w", err)
		}
		inputBytes = codeJSON
	} else {
		inputBytes = []byte("{}")
	}

	return []llm.Content{
		&llm.ToolUseContent{
			ID:    codeCall.ID,
			Name:  "code_interpreter",
			Input: inputBytes,
		},
	}, nil
}

func decodeLocalShellCallContent(shellCall responses.ResponseOutputItemLocalShellCall) ([]llm.Content, error) {
	// For local shell calls, we'll need to check what fields are actually available
	// This might need adjustment based on the actual structure
	return []llm.Content{
		&llm.ToolUseContent{
			ID:    shellCall.ID,
			Name:  "local_shell",
			Input: []byte("{}"), // Placeholder until we know the actual structure
		},
	}, nil
}
