package google

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
	"google.golang.org/genai"
)

const googleThoughtSignatureMetadataKey = "google.thought_signature"

// convertGoogleResponse converts a Google GenAI response to a Dive LLM response
func convertGoogleResponse(resp *genai.GenerateContentResponse, model string) (*llm.Response, error) {
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
			// Gemini does not always populate FunctionCall.ID, so synthesize a
			// unique ID when it is missing. Tool result matching is keyed by
			// this ID, so it must be unique within the conversation.
			toolCallID := part.FunctionCall.ID
			if toolCallID == "" {
				toolCallID = generateToolCallID(part.FunctionCall.Name)
			}
			content = append(content, &llm.ToolUseContent{
				ID:       toolCallID,
				Name:     part.FunctionCall.Name,
				Input:    json.RawMessage(args),
				Metadata: providerMetadataForGooglePart(part),
			})
		} else {
			// Handle other types as text (fallback)
			content = append(content, &llm.TextContent{Text: fmt.Sprintf("%v", part)})
		}
	}

	// Convert usage information
	var usage llm.Usage
	if resp.UsageMetadata != nil {
		usage = convertUsageMetadata(resp.UsageMetadata)
	}

	diveResponse := &llm.Response{
		ID:      fmt.Sprintf("google_%d", candidate.Index),
		Model:   model,
		Role:    llm.Assistant,
		Content: content,
		Type:    "text",
		Usage:   usage,
	}

	// Set stop reason
	diveResponse.StopReason = convertFinishReason(candidate.FinishReason)

	return diveResponse, nil
}

// convertUsageMetadata converts genai usage metadata to llm.Usage, carrying
// cached-content and thoughts token counts where the API reports them.
func convertUsageMetadata(metadata *genai.GenerateContentResponseUsageMetadata) llm.Usage {
	return llm.Usage{
		InputTokens:          int(metadata.PromptTokenCount),
		OutputTokens:         int(metadata.CandidatesTokenCount),
		CacheReadInputTokens: int(metadata.CachedContentTokenCount),
		ReasoningTokens:      int(metadata.ThoughtsTokenCount),
	}
}

// convertFinishReason maps a genai finish reason to Dive's stop reason
// vocabulary for this provider (matching convertGoogleResponse).
func convertFinishReason(reason genai.FinishReason) string {
	switch reason {
	case genai.FinishReasonStop:
		return "stop"
	case genai.FinishReasonMaxTokens:
		return "max_tokens"
	default:
		return "other"
	}
}

// toolCallCounter provides unique suffixes for synthesized tool call IDs.
var toolCallCounter atomic.Int64

// generateToolCallID generates a unique ID for a tool call. Gemini does not
// always supply IDs for function calls, so we synthesize one using a
// package-level counter. The ID is generated once when the model's response
// is received and is carried on the ToolUseContent (and the corresponding
// ToolResultContent.ToolUseID) thereafter, so converting messages back to
// Google format reuses the stored ID rather than regenerating it.
func generateToolCallID(toolName string) string {
	return fmt.Sprintf("call_%s_%d", toolName, toolCallCounter.Add(1))
}

// convertToolUseToFunctionCall converts a Dive ToolUseContent back to Google FunctionCall format
func convertToolUseToFunctionCall(toolUse *llm.ToolUseContent) (*genai.Part, error) {
	if toolUse == nil {
		return nil, fmt.Errorf("tool use content is nil")
	}

	// Parse the input JSON to a map
	var args map[string]any
	if len(toolUse.Input) > 0 {
		if err := json.Unmarshal(toolUse.Input, &args); err != nil {
			return nil, fmt.Errorf("error unmarshaling input for tool %q: %w", toolUse.Name, err)
		}
	} else {
		args = map[string]any{}
	}

	part := &genai.Part{
		FunctionCall: &genai.FunctionCall{
			ID:   toolUse.ID,
			Name: toolUse.Name,
			Args: args,
		},
	}
	signature, err := googleThoughtSignatureFromToolUse(toolUse)
	if err != nil {
		return nil, err
	}
	part.ThoughtSignature = signature
	return part, nil
}

// joinToolResultText flattens tool result content blocks to a single string.
func joinToolResultText(blocks []*dive.ToolResultContent) string {
	var parts []string
	for _, b := range blocks {
		parts = append(parts, b.Text)
	}
	return strings.Join(parts, "\n\n")
}

// convertToolResultToFunctionResponse converts a generic llm.ToolResultContent to a genai.FunctionResponse part
func convertToolResultToFunctionResponse(content *llm.ToolResultContent, functionName string) (*genai.Part, error) {
	if content == nil {
		return nil, fmt.Errorf("content is nil")
	}
	var outputValue any
	switch c := content.Content.(type) {
	case string:
		outputValue = c
	case []byte:
		outputValue = string(c)
	case []*dive.ToolResultContent:
		outputValue = joinToolResultText(c)
	default:
		// Content that round-tripped through JSON (session persistence,
		// Message.Copy) arrives as generic []any rather than typed blocks.
		var blocks []*dive.ToolResultContent
		if err := content.DecodeContent(&blocks); err == nil && blocks != nil {
			outputValue = joinToolResultText(blocks)
		} else {
			data, err := json.Marshal(content.Content)
			if err != nil {
				return nil, fmt.Errorf("error marshaling tool result content: %w", err)
			}
			outputValue = string(data)
		}
	}
	responseData := map[string]any{}
	if content.IsError {
		responseData["error"] = outputValue
	} else {
		responseData["output"] = outputValue
	}
	return &genai.Part{
		FunctionResponse: &genai.FunctionResponse{
			ID:       content.ToolUseID,
			Name:     functionName,
			Response: responseData,
		},
	}, nil
}

func providerMetadataForGooglePart(part *genai.Part) llm.ProviderMetadata {
	if part == nil || len(part.ThoughtSignature) == 0 {
		return nil
	}
	return llm.ProviderMetadata{
		googleThoughtSignatureMetadataKey: base64.StdEncoding.EncodeToString(part.ThoughtSignature),
	}
}

func googleThoughtSignatureFromToolUse(toolUse *llm.ToolUseContent) ([]byte, error) {
	if toolUse == nil || toolUse.Metadata == nil {
		return nil, nil
	}
	encoded := strings.TrimSpace(toolUse.Metadata[googleThoughtSignatureMetadataKey])
	if encoded == "" {
		return nil, nil
	}
	signature, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid Google thought signature for tool %q: %w", toolUse.Name, err)
	}
	return signature, nil
}

// messagesToContents converts Dive messages to genai.Content format for GenerateContent API
func messagesToContents(messages []*llm.Message) ([]*genai.Content, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	contents := make([]*genai.Content, 0, len(messages))

	// Track tool uses for matching with results
	toolUses := map[string]*llm.ToolUseContent{}

	for i, message := range messages {
		if len(message.Content) == 0 {
			return nil, fmt.Errorf("empty message detected (index %d)", i)
		}
		// Convert role
		role := string(message.Role)
		if message.Role == llm.Assistant {
			role = "model"
		}
		content := &genai.Content{
			Role: role,
		}

		// Convert content items to parts
		for _, c := range message.Content {
			switch ct := c.(type) {
			case *llm.TextContent:
				content.Parts = append(content.Parts, genai.NewPartFromText(ct.Text))
			case *llm.ImageContent:
				if ct.Source == nil {
					return nil, fmt.Errorf("image content has nil source")
				}
				switch ct.Source.Type {
				case llm.ContentSourceTypeURL:
					content.Parts = append(content.Parts, genai.NewPartFromURI(ct.Source.URL, ct.Source.MediaType))
				case llm.ContentSourceTypeBase64:
					data, err := ct.Source.DecodedData()
					if err != nil {
						return nil, fmt.Errorf("failed to decode image data: %w", err)
					}
					content.Parts = append(content.Parts, genai.NewPartFromBytes(data, ct.Source.MediaType))
				default:
					return nil, fmt.Errorf("unsupported image source type: %s", ct.Source.Type)
				}
			case *llm.ToolUseContent:
				// Track tool use for later matching
				toolUses[ct.ID] = ct
				part, err := convertToolUseToFunctionCall(ct)
				if err != nil {
					return nil, err
				}
				content.Parts = append(content.Parts, part)
			case *llm.ToolResultContent:
				// Get the function name from the tracked tool uses
				var functionName string
				if toolUse, ok := toolUses[ct.ToolUseID]; ok {
					functionName = toolUse.Name
				} else {
					return nil, fmt.Errorf("tool use not found for tool result: %s", ct.ToolUseID)
				}
				part, err := convertToolResultToFunctionResponse(ct, functionName)
				if err != nil {
					return nil, err
				}
				content.Parts = append(content.Parts, part)
			}
		}
		contents = append(contents, content)
	}

	return contents, nil
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
	if diveSchema.Properties != nil {
		genaiSchema.Properties = make(map[string]*genai.Schema)
		for name, prop := range diveSchema.Properties {
			genaiSchema.Properties[name] = convertPropertyToGenAI(prop)
		}
	}
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
	if len(prop.Enum) > 0 {
		enumValues := make([]string, 0, len(prop.Enum))
		for _, v := range prop.Enum {
			if s, ok := v.(string); ok {
				enumValues = append(enumValues, s)
			}
		}
		genaiSchema.Enum = enumValues
	}
	if prop.Items != nil {
		genaiSchema.Items = convertPropertyToGenAI(prop.Items)
	}
	if prop.Properties != nil {
		genaiSchema.Properties = make(map[string]*genai.Schema)
		for name, nestedProp := range prop.Properties {
			genaiSchema.Properties[name] = convertPropertyToGenAI(nestedProp)
		}
	}
	if len(prop.Required) > 0 {
		genaiSchema.Required = prop.Required
	}
	return genaiSchema
}

// buildGenAIGenerateConfig creates genai.GenerateContentConfig from Request
func buildGenAIGenerateConfig(request *Request) (*genai.GenerateContentConfig, error) {
	genConfig := &genai.GenerateContentConfig{}
	if request.Temperature != nil {
		temp := float32(*request.Temperature)
		genConfig.Temperature = &temp
	}
	if request.MaxTokens > 0 {
		genConfig.MaxOutputTokens = int32(request.MaxTokens)
	}
	if request.System != "" {
		genConfig.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText(request.System)},
		}
	}
	if len(request.Tools) > 0 {
		tools := make([]*genai.Tool, 0, len(request.Tools))
		for _, tool := range request.Tools {
			var schema *genai.Schema
			if inputSchema, ok := tool["input_schema"]; ok && inputSchema != nil {
				schema = convertAnySchemaToGenAI(inputSchema)
			}
			name, ok := tool["name"].(string)
			if !ok {
				return nil, fmt.Errorf("name is required for tool %v", tool)
			}
			description, ok := tool["description"].(string)
			if !ok {
				return nil, fmt.Errorf("description is required for tool %v", tool)
			}
			genaiTool := &genai.Tool{
				FunctionDeclarations: []*genai.FunctionDeclaration{{
					Name:        name,
					Description: description,
					Parameters:  schema,
				}},
			}
			tools = append(tools, genaiTool)
		}
		genConfig.Tools = tools
		genConfig.ToolConfig = &genai.ToolConfig{FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAuto}}
	}
	return genConfig, nil
}
