package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers"
	"github.com/diveagents/dive/retry"
)

var (
	DefaultModel         = "gpt-4o"
	DefaultEndpoint      = "https://api.openai.com/v1/responses"
	DefaultMaxTokens     = 4096
	DefaultClient        = &http.Client{Timeout: 300 * time.Second}
	DefaultMaxRetries    = 6
	DefaultRetryBaseWait = 2 * time.Second
)

var _ llm.StreamingLLM = &Provider{}

type Provider struct {
	client        *http.Client
	apiKey        string
	endpoint      string
	model         string
	maxTokens     int
	maxRetries    int
	retryBaseWait time.Duration
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:        os.Getenv("OPENAI_API_KEY"),
		endpoint:      DefaultEndpoint,
		client:        DefaultClient,
		model:         DefaultModel,
		maxTokens:     DefaultMaxTokens,
		maxRetries:    DefaultMaxRetries,
		retryBaseWait: DefaultRetryBaseWait,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provider) Name() string {
	return "openai"
}

func (p *Provider) ModelName() string {
	return p.model
}

func (p *Provider) buildConfig(opts ...llm.Option) *llm.Config {
	config := &llm.Config{}
	config.Apply(opts...)
	return config
}

func (p *Provider) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	config := p.buildConfig(opts...)
	request, err := p.buildRequest(config)
	if err != nil {
		return nil, err
	}
	response, err := p.makeRequest(ctx, request, config)
	if err != nil {
		return nil, err
	}
	return p.convertResponse(response)
}

func (p *Provider) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
	config := p.buildConfig(opts...)
	request, err := p.buildRequest(config)
	if err != nil {
		return nil, err
	}
	stream := true
	request.Stream = &stream
	return p.makeStreamRequest(ctx, request, config)
}

// buildRequest converts llm.Config to Responses API request format
func (p *Provider) buildRequest(config *llm.Config) (*Request, error) {
	request := &Request{
		Model:       p.model,
		Temperature: config.Temperature,
	}

	if config.MaxTokens != nil && *config.MaxTokens > 0 {
		maxTokens := *config.MaxTokens
		request.MaxOutputTokens = maxTokens
	} else if p.maxTokens > 0 {
		request.MaxOutputTokens = p.maxTokens
	}

	// Handle reasoning effort (for o-series models)
	if config.ReasoningEffort != "" && strings.HasPrefix(p.model, "o-") {
		request.Reasoning = &ReasoningConfig{
			Effort: &config.ReasoningEffort,
		}
	}

	if config.ParallelToolCalls != nil {
		request.ParallelToolCalls = config.ParallelToolCalls
	}
	if config.PreviousResponseID != "" {
		request.PreviousResponseID = config.PreviousResponseID
	}
	if config.ServiceTier != "" {
		request.ServiceTier = config.ServiceTier
	}

	// Handle tool choice
	if config.ToolChoice != "" {
		// Map from common tool choice names to OpenAI Responses format
		switch string(config.ToolChoice) {
		case "auto":
			request.ToolChoice = "auto"
		case "none":
			request.ToolChoice = "none"
		case "required", "any":
			request.ToolChoice = "required"
		default:
			// Assume it's a specific tool name
			request.ToolChoice = map[string]interface{}{
				"type": "function",
				"function": map[string]string{
					"name": string(config.ToolChoice),
				},
			}
		}
	}

	// Handle JSON schema output format
	if jsonSchema := config.RequestHeaders.Get("X-OpenAI-Responses-JSON-Schema"); jsonSchema != "" {
		var schema interface{}
		if err := json.Unmarshal([]byte(jsonSchema), &schema); err == nil {
			request.Text = &TextConfig{
				Format: TextFormat{
					Type:   "json_schema",
					Schema: schema,
				},
			}
		}
	}

	// Convert messages to input format
	if len(config.Messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	input, err := p.convertMessagesToInput(config.Messages)
	if err != nil {
		return nil, err
	}
	request.Input = input

	if len(config.Tools) > 0 {
		var tools []any
		for _, tool := range config.Tools {
			// Handle tools that explicitly provide a configuration
			if toolWithConfig, ok := tool.(llm.ToolConfiguration); ok {
				toolConfig := toolWithConfig.ToolConfiguration(p.Name())
				// nil means no configuration is specified and to use the default
				if toolConfig != nil {
					tools = append(tools, toolConfig)
					continue
				}
			}
			// Handle tools with the default configuration behavior
			// Ensure the schema has additionalProperties set to false for OpenAI compatibility
			schema := tool.Schema()
			// schema = p.ensureSchemaAdditionalPropertiesFalse(schema)
			tools = append(tools, map[string]any{
				"name":        tool.Name(),
				"parameters":  schema,
				"type":        "function",
				"description": tool.Description(),
			})
		}
		request.Tools = tools
	}

	for _, mcpServer := range config.MCPServers {
		tool := map[string]any{
			"type":         "mcp",
			"server_label": mcpServer.Name,
			"server_url":   mcpServer.URL,
		}
		headers := map[string]string{}
		if mcpServer.ToolConfiguration != nil {
			tool["allowed_tools"] = mcpServer.ToolConfiguration.AllowedTools
		}
		if mcpServer.AuthorizationToken != "" {
			headers["Authorization"] = "Bearer " + mcpServer.AuthorizationToken
		}
		if len(mcpServer.Headers) > 0 {
			for key, value := range mcpServer.Headers {
				headers[key] = value
			}
		}
		if len(headers) > 0 {
			tool["headers"] = headers
		}
		if mcpServer.ApprovalRequirement != nil {
			tool["require_approval"] = mcpServer.ApprovalRequirement
		} else {
			// Default to requiring approval for security if not specified
			tool["require_approval"] = "always"
		}
		request.Tools = append(request.Tools, tool)
	}
	return request, nil
}

// makeRequest makes a non-streaming request to the Responses API
func (p *Provider) makeRequest(ctx context.Context, request *Request, config *llm.Config) (*Response, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	if err := config.FireHooks(ctx, &llm.HookContext{
		Type: llm.BeforeGenerate,
		Request: &llm.HookRequestContext{
			Messages: config.Messages,
			Config:   config,
			Body:     body,
		},
	}); err != nil {
		return nil, err
	}

	var result Response
	err = retry.Do(ctx, func() error {
		req, err := p.createRequest(ctx, body, config, false)
		if err != nil {
			return err
		}
		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("error making request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			statusCode := resp.StatusCode

			if statusCode == 429 {
				if config.Logger != nil {
					config.Logger.Warn("rate limit exceeded",
						"status", statusCode, "body", string(body))
				}

				// Check for Retry-After header
				if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
					// Parse Retry-After header and wait accordingly
					if waitDuration, err := time.ParseDuration(retryAfter + "s"); err == nil {
						time.Sleep(waitDuration)
					}
				}
			}
			return providers.NewError(statusCode, string(body))
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("error decoding response: %w", err)
		}
		return nil
	}, retry.WithMaxRetries(p.maxRetries), retry.WithBaseWait(p.retryBaseWait))

	if err != nil {
		return nil, err
	}
	return &result, nil
}

// makeStreamRequest makes a streaming request to the Responses API
func (p *Provider) makeStreamRequest(ctx context.Context, request *Request, config *llm.Config) (llm.StreamIterator, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	if err := config.FireHooks(ctx, &llm.HookContext{
		Type: llm.BeforeGenerate,
		Request: &llm.HookRequestContext{
			Messages: config.Messages,
			Config:   config,
			Body:     body,
		},
	}); err != nil {
		return nil, err
	}

	var stream *StreamIterator
	err = retry.Do(ctx, func() error {
		req, err := p.createRequest(ctx, body, config, true)
		if err != nil {
			return err
		}

		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("error making request: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			statusCode := resp.StatusCode

			if statusCode == 429 {
				if config.Logger != nil {
					config.Logger.Warn("rate limit exceeded",
						"status", statusCode, "body", string(body))
				}

				// Check for Retry-After header
				if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
					// Parse Retry-After header and wait accordingly
					if waitDuration, err := time.ParseDuration(retryAfter + "s"); err == nil {
						time.Sleep(waitDuration)
					}
				}
			}

			// Use shared provider error type with proper retry logic
			return providers.NewError(statusCode, string(body))
		}
		stream = NewStreamIterator(resp.Body, config.SSECallback)
		return nil
	}, retry.WithMaxRetries(p.maxRetries), retry.WithBaseWait(p.retryBaseWait))

	if err != nil {
		return nil, err
	}
	return stream, nil
}

// convertResponse converts Responses API response to llm.Response
func (p *Provider) convertResponse(response *Response) (*llm.Response, error) {
	var contentBlocks []llm.Content
	for _, item := range response.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if content.Type == "output_text" || content.Type == "text" {
					contentBlocks = append(contentBlocks, &llm.TextContent{
						Text: content.Text,
					})
				}
			}

		case "function_call":
			contentBlocks = append(contentBlocks, &llm.ToolUseContent{
				ID:    item.CallID,
				Name:  item.Name,
				Input: []byte(item.Arguments),
			})

		case "image_generation_call":
			if item.Result != "" {
				// Create proper ImageContent with base64 data
				contentBlocks = append(contentBlocks, &llm.ImageContent{
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeBase64,
						MediaType: "image/png", // Default to PNG, could be configurable
						Data:      item.Result,
					},
				})
			}

		case "web_search_call":
			if len(item.Results) > 0 {
				var resultText strings.Builder
				resultText.WriteString("Web search results:\n")
				for _, result := range item.Results {
					resultText.WriteString(fmt.Sprintf("- %s: %s\n", result.Title, result.Description))
				}
				contentBlocks = append(contentBlocks, &llm.TextContent{
					Text: resultText.String(),
				})
			}

		case "mcp_call":
			if item.Output != "" {
				contentBlocks = append(contentBlocks, &llm.TextContent{
					Text: fmt.Sprintf("MCP tool result: %s", item.Output),
				})
			}

		case "mcp_list_tools":
			if len(item.Tools) > 0 {
				var toolsText strings.Builder
				toolsText.WriteString(fmt.Sprintf("MCP server '%s' tools:\n", item.ServerLabel))
				for _, tool := range item.Tools {
					toolsText.WriteString(fmt.Sprintf("- %s\n", tool.Name))
				}
				contentBlocks = append(contentBlocks, &llm.TextContent{
					Text: toolsText.String(),
				})
			}

		case "mcp_approval_request":
			contentBlocks = append(contentBlocks, &llm.TextContent{
				Text: fmt.Sprintf("MCP approval required for tool '%s' on server '%s'", item.Name, item.ServerLabel),
			})
		}
	}

	usage := llm.Usage{}
	if response.Usage != nil {
		usage.InputTokens = response.Usage.InputTokens
		usage.OutputTokens = response.Usage.OutputTokens

		// Map cached tokens from input details to cache read tokens
		if response.Usage.InputTokensDetails != nil {
			usage.CacheReadInputTokens = response.Usage.InputTokensDetails.CachedTokens
		}

		// Note: OpenAI doesn't provide cache creation tokens in their usage data,
		// so we leave CacheCreationInputTokens at 0
	}

	// Determine stop_reason based on the response content and status
	stopReason := determineStopReason(response)

	return &llm.Response{
		ID:         response.ID,
		Model:      response.Model,
		Role:       llm.Assistant,
		Content:    contentBlocks,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

// determineStopReason maps OpenAI response data to standard stop reasons
func determineStopReason(response *Response) string {
	// Check if the response contains any tool calls (function_call items)
	for _, item := range response.Output {
		if item.Type == "function_call" {
			return "tool_use"
		}
	}

	// If response is completed without tool calls, it's an end_turn
	if response.Status == "completed" {
		return "end_turn"
	}

	// Default to end_turn for other completion scenarios
	return "end_turn"
}

// createRequest creates an HTTP request with appropriate headers
func (p *Provider) createRequest(ctx context.Context, body []byte, config *llm.Config, isStreaming bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	if isStreaming {
		req.Header.Set("Accept", "text/event-stream")
	}

	for key, values := range config.RequestHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return req, nil
}

// convertMessagesToInput converts llm.Message slice to Responses API input format
func (p *Provider) convertMessagesToInput(messages []*llm.Message) ([]*InputMessage, error) {
	var inputMessages []*InputMessage
	for _, msg := range messages {
		inputMsg := &InputMessage{
			Role: string(msg.Role),
		}
		for _, content := range msg.Content {
			switch c := content.(type) {

			case *llm.TextContent:
				if strings.HasPrefix(c.Text, "MCP_APPROVAL_RESPONSE:") {
					parts := strings.Split(c.Text, ":")
					if len(parts) == 3 {
						approvalRequestID := parts[1]
						approve := parts[2] == "true"
						inputMsg.Content = append(inputMsg.Content, &InputContent{
							Type:              "mcp_approval_response",
							ApprovalRequestID: approvalRequestID,
							Approve:           &approve,
						})
					}
				} else {
					// Use the correct content type based on message role
					contentType := "input_text"
					if msg.Role == llm.Assistant {
						contentType = "output_text"
					}
					inputMsg.Content = append(inputMsg.Content, &InputContent{
						Type: contentType,
						Text: c.Text,
					})
				}

			case *llm.ImageContent:
				if c.Source != nil {
					inputMsg.Content = append(inputMsg.Content, &InputContent{
						Type:     "image",
						ImageURL: c.Source.URL,
					})
				}

			case *llm.DocumentContent:
				if c.Source != nil {
					inputContent := &InputContent{Type: "input_file"}
					// Set filename from title if available. Otherwise, use
					// a default filename since this is required by the API.
					if c.Title != "" {
						inputContent.Filename = c.Title
					} else {
						inputContent.Filename = "document"
					}
					switch c.Source.Type {
					case llm.ContentSourceTypeBase64:
						if c.Source.MediaType == "" || c.Source.Data == "" {
							return nil, fmt.Errorf("media type and data are required for base64 document content")
						}
						inputContent.FileData = fmt.Sprintf("data:%s;base64,%s", c.Source.MediaType, c.Source.Data)
					case llm.ContentSourceTypeFile:
						if c.Source.FileID == "" {
							return nil, fmt.Errorf("file id is required file document content")
						}
						inputContent.FileID = c.Source.FileID
					case llm.ContentSourceTypeURL:
						// OpenAI Responses API doesn't support URL references directly
						// Return an error instead of silently skipping
						return nil, fmt.Errorf("URL-based document content is not supported by OpenAI Responses provider. Please download the file and use base64 content instead. URL: %s", c.Source.URL)
					}
					inputMsg.Content = append(inputMsg.Content, inputContent)
				}

			case *llm.ToolResultContent:
				if contentStr, ok := c.Content.(string); ok {
					inputMsg.Content = append(inputMsg.Content, &InputContent{
						Type: "input_text",
						Text: fmt.Sprintf("Tool result: %s", contentStr),
					})
				}

			case *llm.ToolUseContent:
				inputMsg.Content = append(inputMsg.Content, &InputContent{
					Type: "input_text",
					Text: fmt.Sprintf("Tool use: %s", c.Name),
				})

			default:
				return nil, fmt.Errorf("unsupported content type: %T", c)
			}
		}

		for _, content := range inputMsg.Content {
			fmt.Printf("xxx content: %+v\n", content)
		}

		inputMessages = append(inputMessages, inputMsg)
	}
	return inputMessages, nil
}

// NewStreamIterator creates a new StreamIterator for a Responses API response
// body. Typically this is not called directly, but rather by the provider.
func NewStreamIterator(body io.ReadCloser, callback llm.ServerSentEventsCallback) *StreamIterator {
	reader := llm.NewServerSentEventsReader[StreamEvent](body).
		WithSSECallback(callback)
	return &StreamIterator{
		body:   body,
		reader: reader,
	}
}
