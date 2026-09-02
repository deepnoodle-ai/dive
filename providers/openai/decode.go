package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/openai/openai-go/v3/responses"
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

	inputTokens, cacheReadInputTokens, cacheCreationInputTokens := normalizeInputTokens(
		response.Usage.InputTokens,
		response.Usage.InputTokensDetails.CachedTokens,
		response.Usage.InputTokensDetails.CacheWriteTokens,
	)
	usage := llm.Usage{
		InputTokens:              inputTokens,
		OutputTokens:             int(response.Usage.OutputTokens),
		CacheReadInputTokens:     cacheReadInputTokens,
		CacheCreationInputTokens: cacheCreationInputTokens,
		ReasoningTokens:          int(response.Usage.OutputTokensDetails.ReasoningTokens),
	}

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

// normalizeInputTokens converts OpenAI's subset-shaped usage into Dive's
// disjoint input buckets and clamps invalid provider counts.
func normalizeInputTokens(prompt, cached, written int64) (inputTokens, cacheReadInputTokens, cacheCreationInputTokens int) {
	prompt = max(0, prompt)
	cached = min(max(0, cached), prompt)
	written = min(max(0, written), prompt-cached)
	return int(prompt - cached - written), int(cached), int(written)
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
	phase := string(outputMsg.Phase)
	for _, content := range outputMsg.Content {
		switch content.Type {
		case "output_text":
			outputText := content.AsOutputText()
			textContent := &llm.TextContent{
				Text: outputText.Text,
				// Each text block records the phase of the output message it
				// came from, so a response mixing commentary with a final
				// answer replays each block under its own phase.
				Metadata: openAIPhaseMetadata(phase),
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
	//
	// The action records what the model did with the web: the query it ran, the
	// page it opened, and — when the request asked for
	// web_search_call.action.sources — the hits behind the answer. Only the
	// call's ID used to survive decoding, so a caller who paid for the sources
	// include had no way to read what came back.
	content := &llm.ServerToolUseContent{
		ID:    call.ID,
		Name:  "web_search_call",
		Input: webSearchActionInput(call.Action),
	}
	if results := webSearchResults(call.RawJSON()); len(results) > 0 {
		if content.Input == nil {
			content.Input = map[string]any{}
		}
		content.Input["results"] = results
	}
	return []llm.Content{content}, nil
}

// webSearchResults pulls the hits that include: ["web_search_call.results"]
// adds to a call: title, url, and the snippet the model actually saw. The SDK
// struct has no field for them, so they only exist in the raw payload, and
// without this the include is paid for and then discarded.
func webSearchResults(raw string) []any {
	if raw == "" {
		return nil
	}
	var payload struct {
		Results []map[string]any `json:"results"`
	}
	// A shape the SDK does not model is not an error worth failing a decode
	// over: the answer and its citations are already decoded by this point.
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	results := make([]any, 0, len(payload.Results))
	for _, result := range payload.Results {
		results = append(results, result)
	}
	if len(results) == 0 {
		return nil
	}
	return results
}

// webSearchActionInput flattens the action union into the fields the variant
// actually set. It returns nil rather than an empty map when the action is
// absent, so a provider that omits it reads the same as before.
func webSearchActionInput(action responses.ResponseFunctionWebSearchActionUnion) map[string]any {
	input := map[string]any{}
	if action.Type != "" {
		input["type"] = action.Type
	}
	if action.Query != "" {
		input["query"] = action.Query
	}
	if len(action.Queries) > 0 {
		input["queries"] = action.Queries
	}
	if action.URL != "" {
		input["url"] = action.URL
	}
	if action.Pattern != "" {
		input["pattern"] = action.Pattern
	}
	if len(action.Sources) > 0 {
		sources := make([]any, 0, len(action.Sources))
		for _, source := range action.Sources {
			sources = append(sources, map[string]any{
				"type": string(source.Type),
				"url":  source.URL,
			})
		}
		input["sources"] = sources
	}
	if len(input) == 0 {
		return nil
	}
	return input
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
	errorText, hasError := mcpCallErrorText(mcpCall.Error)
	if mcpCall.Output != "" || hasError {
		var resultContent []*llm.ContentChunk
		isError := false

		if hasError {
			// If there's an error, add it as text content and mark as error
			resultContent = append(resultContent, &llm.ContentChunk{
				Type: "text",
				Text: errorText,
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

// mcpCallErrorText flattens an mcp_call error to text and reports whether the
// call actually failed. openai-go v3.51 replaced the plain error string with a
// union of mcp_protocol_error, mcp_tool_execution_error, and http_error: the
// protocol and HTTP variants carry a code and message, while the execution
// variant carries the server's own content, which may be a string or the
// structured block list MCP servers return. Type is the discriminant, so an
// empty Type means no error was reported.
func mcpCallErrorText(callErr responses.McpToolCallErrorUnion) (string, bool) {
	if callErr.Type == "" {
		return "", false
	}
	if content := callErr.Content; content != nil {
		if text, ok := content.(string); ok {
			if text != "" {
				return text, true
			}
		} else if encoded, err := json.Marshal(content); err == nil {
			return string(encoded), true
		}
	}
	if callErr.Message != "" {
		if callErr.Code != 0 {
			return fmt.Sprintf("%s (code %d)", callErr.Message, callErr.Code), true
		}
		return callErr.Message, true
	}
	// The variant reported a failure but carried no detail Dive can render;
	// the type itself is still worth surfacing over an empty string.
	return callErr.Type, true
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
	var reasoningItems []string
	for _, content := range reasoning.Content {
		reasoningItems = append(reasoningItems, content.Text)
	}
	metadata, err := openAIReasoningMetadata(summaryItems, reasoningItems)
	if err != nil {
		return nil, err
	}
	displayItems := summaryItems
	if len(reasoningItems) > 0 {
		displayItems = reasoningItems
	}
	return []llm.Content{
		&llm.ThinkingContent{
			ID:        reasoning.ID,
			Thinking:  strings.Join(displayItems, "\n\n"),
			Signature: reasoning.EncryptedContent,
			Metadata:  metadata,
		},
	}, nil
}

// openAIPhaseMetadata returns metadata carrying an assistant output message's
// phase, or nil when OpenAI did not label the message. Each call returns a
// fresh map so blocks decoded from one message do not share state.
func openAIPhaseMetadata(phase string) llm.ProviderMetadata {
	if phase == "" {
		return nil
	}
	return llm.ProviderMetadata{openAIPhaseMetadataKey: phase}
}

func openAIReasoningMetadata(summaryItems, reasoningItems []string) (llm.ProviderMetadata, error) {
	metadata := llm.ProviderMetadata{}
	if len(summaryItems) > 0 {
		data, err := json.Marshal(summaryItems)
		if err != nil {
			return nil, err
		}
		metadata[openAIReasoningSummaryMetadataKey] = string(data)
	}
	if len(reasoningItems) > 0 {
		data, err := json.Marshal(reasoningItems)
		if err != nil {
			return nil, err
		}
		metadata[openAIReasoningContentMetadataKey] = string(data)
	}
	if len(metadata) == 0 {
		return nil, nil
	}
	return metadata, nil
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
	var results []FileSearchCallResult
	for _, result := range fileSearchCall.Results {
		results = append(results, FileSearchCallResult{
			FileID:   result.FileID,
			Filename: result.Filename,
			Score:    result.Score,
			Text:     result.Text,
		})
	}
	return []llm.Content{
		&FileSearchCallContent{
			ID:      fileSearchCall.ID,
			Queries: fileSearchCall.Queries,
			Status:  string(fileSearchCall.Status),
			Results: results,
		},
	}, nil
}

func decodeComputerCallContent(computerCall responses.ResponseComputerToolCall) ([]llm.Content, error) {
	return nil, fmt.Errorf("computer call is not yet supported")
}

func decodeLocalShellCallContent(shellCall responses.ResponseOutputItemLocalShellCall) ([]llm.Content, error) {
	return nil, fmt.Errorf("local shell call is not yet supported")
}
