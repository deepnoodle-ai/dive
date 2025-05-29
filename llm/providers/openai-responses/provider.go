package openairesponses

import (
	"bufio"
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
	DefaultModel    = "gpt-4o"
	DefaultEndpoint = "https://api.openai.com/v1/responses"
)

var _ llm.StreamingLLM = &Provider{}

type Provider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
	// Retry configuration
	maxRetries int
	baseWait   time.Duration
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:   os.Getenv("OPENAI_API_KEY"),
		endpoint: DefaultEndpoint,
		client: &http.Client{
			Timeout: 30 * time.Second, // Add reasonable timeout
		},
		maxRetries: 6,               // Default to 6 retries as before
		baseWait:   2 * time.Second, // Default base wait time
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.model == "" {
		p.model = DefaultModel
	}
	return p
}

func (p *Provider) Name() string {
	return "openai-responses-" + p.model
}

func (p *Provider) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	config := &llm.Config{}
	config.Apply(opts...)

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
	config := &llm.Config{}
	config.Apply(opts...)

	request, err := p.buildRequest(config)
	if err != nil {
		return nil, err
	}
	request.Stream = true

	return p.makeStreamRequest(ctx, request, config)
}

func (p *Provider) SupportsStreaming() bool {
	return true
}

// buildRequest converts llm.Config to Responses API request format
func (p *Provider) buildRequest(config *llm.Config) (*Request, error) {
	request := &Request{
		Model:       p.model,
		Temperature: config.Temperature,
	}

	// Handle store setting: per-request config takes precedence over provider default
	if storeValue, found := getFeatureBoolValue(config.Features, "openai-responses:store"); found {
		request.Store = &storeValue
	}

	// Handle background setting: per-request config takes precedence over provider default
	if backgroundValue, found := getFeatureBoolValue(config.Features, "openai-responses:background"); found {
		request.Background = &backgroundValue
	}

	// Convert messages to input format
	if len(config.Messages) > 0 {
		input, err := p.convertMessagesToInput(config.Messages)
		if err != nil {
			return nil, err
		}
		request.Input = input
	}

	// Add built-in tools
	tools, err := p.buildTools(config)
	if err != nil {
		return nil, err
	}
	request.Tools = tools

	// Add custom tools (check if they implement ToolConfiguration interface)
	for _, tool := range config.Tools {
		// Check if this tool implements ToolConfiguration for provider-specific handling
		if toolConfig, ok := tool.(llm.ToolConfiguration); ok {
			// Get provider-specific configuration
			providerConfig := toolConfig.ToolConfiguration("openai-responses")

			// Convert the configuration map to our Tool struct
			toolDef := Tool{}

			// Use JSON marshaling/unmarshaling for easy conversion
			configBytes, err := json.Marshal(providerConfig)
			if err != nil {
				return nil, fmt.Errorf("error marshaling tool configuration: %w", err)
			}

			if err := json.Unmarshal(configBytes, &toolDef); err != nil {
				return nil, fmt.Errorf("error unmarshaling tool configuration: %w", err)
			}

			request.Tools = append(request.Tools, toolDef)
		} else {
			// Handle as regular function tool
			request.Tools = append(request.Tools, Tool{
				Type: "function",
				Function: &FunctionTool{
					Name:        tool.Name(),
					Description: tool.Description(),
					Parameters:  tool.Schema(),
				},
			})
		}
	}

	// Add MCP servers from llm.Config (request-level configuration)
	for _, mcpServer := range config.MCPServers {
		tool := Tool{
			Type:        "mcp",
			ServerLabel: mcpServer.Name,
			ServerURL:   mcpServer.URL,
		}

		// Handle tool configuration
		if mcpServer.ToolConfiguration != nil {
			tool.AllowedTools = mcpServer.ToolConfiguration.AllowedTools
		}

		// Handle authorization
		if mcpServer.AuthorizationToken != "" {
			if tool.Headers == nil {
				tool.Headers = make(map[string]string)
			}
			tool.Headers["Authorization"] = "Bearer " + mcpServer.AuthorizationToken
		}

		// Handle approval requirements - support OpenAI's full approval modes
		if mcpServer.ApprovalRequirement != nil {
			tool.RequireApproval = mcpServer.ApprovalRequirement
		} else {
			// Default to requiring approval for security if not specified
			tool.RequireApproval = "always"
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
			if resp.StatusCode == 429 {
				if config.Logger != nil {
					config.Logger.Warn("rate limit exceeded",
						"status", resp.StatusCode, "body", string(body))
				}
			}
			return providers.NewError(resp.StatusCode, string(body))
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("error decoding response: %w", err)
		}
		return nil
	}, retry.WithMaxRetries(p.maxRetries), retry.WithBaseWait(p.baseWait))

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
			if resp.StatusCode == 429 {
				if config.Logger != nil {
					config.Logger.Warn("rate limit exceeded",
						"status", resp.StatusCode, "body", string(body))
				}
			}
			return providers.NewError(resp.StatusCode, string(body))
		}
		stream = &StreamIterator{
			body:   resp.Body,
			reader: bufio.NewReader(resp.Body),
		}
		return nil
	}, retry.WithMaxRetries(p.maxRetries), retry.WithBaseWait(p.baseWait))

	if err != nil {
		return nil, err
	}
	return stream, nil
}

// convertResponse converts Responses API response to llm.Response
func (p *Provider) convertResponse(response *Response) (*llm.Response, error) {
	var contentBlocks []llm.Content

	// Process output items
	for _, item := range response.Output {
		switch item.Type {
		case "message":
			// Convert message content
			for _, content := range item.Content {
				if content.Type == "output_text" || content.Type == "text" {
					contentBlocks = append(contentBlocks, &llm.TextContent{
						Text: content.Text,
					})
				}
			}
		case "function_call":
			// Convert function calls to tool use content
			contentBlocks = append(contentBlocks, &llm.ToolUseContent{
				ID:    item.CallID,
				Name:  item.Name,
				Input: []byte(item.Arguments),
			})
		case "image_generation_call":
			// Handle image generation results
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
			// Handle web search results
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
			// Handle MCP tool results
			if item.Output != "" {
				contentBlocks = append(contentBlocks, &llm.TextContent{
					Text: fmt.Sprintf("MCP tool result: %s", item.Output),
				})
			}
		case "mcp_list_tools":
			// Handle MCP tool list results
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
			// Handle MCP approval requests
			contentBlocks = append(contentBlocks, &llm.TextContent{
				Text: fmt.Sprintf("MCP approval required for tool '%s' on server '%s'", item.Name, item.ServerLabel),
			})
		}
	}

	usage := llm.Usage{}
	if response.Usage != nil {
		usage.InputTokens = response.Usage.InputTokens
		usage.OutputTokens = response.Usage.OutputTokens
	}

	return &llm.Response{
		ID:      response.ID,
		Model:   response.Model,
		Role:    llm.Assistant,
		Content: contentBlocks,
		Usage:   usage,
	}, nil
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
func (p *Provider) convertMessagesToInput(messages []*llm.Message) (interface{}, error) {
	// Simple case: single user message becomes string input
	if len(messages) == 1 && messages[0].Role == llm.User {
		if len(messages[0].Content) == 1 {
			if textContent, ok := messages[0].Content[0].(*llm.TextContent); ok {
				return textContent.Text, nil
			}
		}
	}

	// Complex case: convert to message array format
	var inputMessages []InputMessage
	for _, msg := range messages {
		inputMsg := InputMessage{
			Role: string(msg.Role),
		}

		for _, content := range msg.Content {
			switch c := content.(type) {
			case *llm.TextContent:
				// Check if this is an MCP approval response
				if strings.HasPrefix(c.Text, "MCP_APPROVAL_RESPONSE:") {
					parts := strings.Split(c.Text, ":")
					if len(parts) == 3 {
						approvalRequestID := parts[1]
						approve := parts[2] == "true"
						inputMsg.Content = append(inputMsg.Content, InputContent{
							Type:              "mcp_approval_response",
							ApprovalRequestID: approvalRequestID,
							Approve:           &approve,
						})
					}
				} else {
					inputMsg.Content = append(inputMsg.Content, InputContent{
						Type: "input_text",
						Text: c.Text,
					})
				}
			case *llm.ImageContent:
				// Handle image content
				if c.Source != nil {
					inputMsg.Content = append(inputMsg.Content, InputContent{
						Type:     "image",
						ImageURL: c.Source.URL,
					})
				}
			case *llm.DocumentContent:
				// Handle document content - convert to file input for OpenAI Responses API
				if c.Source != nil {
					inputContent := InputContent{
						Type: "input_file",
					}

					// Set filename from title if available
					if c.Title != "" {
						inputContent.Filename = c.Title
					}

					switch c.Source.Type {
					case llm.ContentSourceTypeBase64:
						// Convert base64 data to data URI format expected by OpenAI
						if c.Source.MediaType != "" && c.Source.Data != "" {
							inputContent.FileData = fmt.Sprintf("data:%s;base64,%s", c.Source.MediaType, c.Source.Data)
						}
					case llm.ContentSourceTypeFile:
						// Use file ID directly
						if c.Source.FileID != "" {
							inputContent.FileID = c.Source.FileID
						}
					case llm.ContentSourceTypeURL:
						// OpenAI Responses API doesn't support URL references directly
						// This would need to be downloaded and converted to base64 or file ID
						// For now, we'll skip this case or could add a warning
						continue
					}

					inputMsg.Content = append(inputMsg.Content, inputContent)
				}
			case *llm.ToolResultContent:
				// Handle tool result content - convert to text for now
				if contentStr, ok := c.Content.(string); ok {
					inputMsg.Content = append(inputMsg.Content, InputContent{
						Type: "input_text",
						Text: fmt.Sprintf("Tool result: %s", contentStr),
					})
				}
				// Add more content types as needed
			}
		}

		inputMessages = append(inputMessages, inputMsg)
	}

	return inputMessages, nil
}

// buildTools builds the tools array for the request using per-request configuration
func (p *Provider) buildTools(config *llm.Config) ([]Tool, error) {
	var tools []Tool

	// Build tools based on per-request features
	if hasFeature(config.Features, "openai-responses:web_search") {
		tool := Tool{Type: "web_search_preview"}

		// Use per-request options from headers
		if domains := config.RequestHeaders.Get("X-OpenAI-Responses-Web-Search-Domains"); domains != "" {
			tool.Domains = []string{domains} // Simplified - in practice you'd parse CSV
		}
		if contextSize := config.RequestHeaders.Get("X-OpenAI-Responses-Web-Search-Context-Size"); contextSize != "" {
			tool.SearchContextSize = contextSize
		}

		tools = append(tools, tool)
	}

	if hasFeature(config.Features, "openai-responses:image_generation") {
		tool := Tool{Type: "image_generation"}

		// Use per-request options from headers
		if size := config.RequestHeaders.Get("X-OpenAI-Responses-Image-Size"); size != "" {
			tool.Size = size
		}
		if quality := config.RequestHeaders.Get("X-OpenAI-Responses-Image-Quality"); quality != "" {
			tool.Quality = quality
		}
		if background := config.RequestHeaders.Get("X-OpenAI-Responses-Image-Background"); background != "" {
			tool.Background = background
		}

		tools = append(tools, tool)
	}

	return tools, nil
}

// hasFeature checks if a feature is enabled in the features list
func hasFeature(features []string, feature string) bool {
	for _, f := range features {
		if f == feature {
			return true
		}
	}
	return false
}

// getFeatureBoolValue parses a feature with a boolean value (e.g., "feature=true")
// Returns the boolean value and whether the feature was found
func getFeatureBoolValue(features []string, featurePrefix string) (bool, bool) {
	for _, f := range features {
		if f == featurePrefix+"=true" {
			return true, true
		}
		if f == featurePrefix+"=false" {
			return false, true
		}
	}
	return false, false
}

// StreamIterator implements llm.StreamIterator for the Responses API
type StreamIterator struct {
	reader       *bufio.Reader
	body         io.ReadCloser
	err          error
	currentEvent *llm.Event
	eventCount   int
	// Track previous state for delta detection
	previousText      string
	hasStartedContent bool
	hasEmittedStop    bool
	// Track content block indices
	nextContentIndex int
	textContentIndex int
}

// Next advances to the next event in the stream
func (s *StreamIterator) Next() bool {
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				s.err = err
			}
			return false
		}

		// Skip empty lines
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		// Remove "data: " prefix if present
		line = bytes.TrimPrefix(line, []byte("data: "))
		line = bytes.TrimSpace(line)

		// Check for stream end
		if bytes.Equal(line, []byte("[DONE]")) {
			if !s.hasEmittedStop {
				// Emit message stop event before ending
				s.hasEmittedStop = true
				s.currentEvent = &llm.Event{
					Type: llm.EventTypeMessageStop,
				}
				return true
			}
			return false
		}

		// Skip non-JSON lines (like "event: " lines or other SSE metadata)
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}

		// Parse the streaming event
		var streamEvent StreamEvent
		if err := json.Unmarshal(line, &streamEvent); err != nil {
			// Log the problematic line for debugging but continue processing
			s.err = fmt.Errorf("error parsing stream event (line: %q): %w", string(line), err)
			return false
		}

		// Convert to llm.Event
		if event := s.convertStreamEvent(&streamEvent); event != nil {
			s.currentEvent = event
			return true
		}
	}
}

// convertStreamEvent converts a StreamEvent to an llm.Event
func (s *StreamIterator) convertStreamEvent(streamEvent *StreamEvent) *llm.Event {
	if streamEvent.Response == nil {
		return nil
	}

	response := streamEvent.Response

	// Emit message start event if this is the first event
	if s.eventCount == 0 {
		s.eventCount++
		return &llm.Event{
			Type: llm.EventTypeMessageStart,
			Message: &llm.Response{
				ID:      response.ID,
				Type:    "message",
				Role:    llm.Assistant,
				Model:   response.Model,
				Content: []llm.Content{},
				Usage:   llm.Usage{},
			},
		}
	}

	// Process output items for content deltas
	for _, item := range response.Output {
		switch item.Type {
		case "message":
			// Handle text content deltas
			for _, content := range item.Content {
				if content.Type == "text" || content.Type == "output_text" {
					currentText := content.Text

					// If this is the first time we see content, emit a content block start event
					if !s.hasStartedContent {
						s.hasStartedContent = true
						s.previousText = currentText
						s.textContentIndex = s.nextContentIndex
						s.nextContentIndex++
						return &llm.Event{
							Type:  llm.EventTypeContentBlockStart,
							Index: &s.textContentIndex,
							ContentBlock: &llm.EventContentBlock{
								Type: llm.ContentTypeText,
								Text: currentText,
							},
						}
					}

					// If the text has changed, emit a delta event with only the new text
					if currentText != s.previousText {
						// Calculate the actual delta - only the newly added text
						deltaText := currentText[len(s.previousText):]
						s.previousText = currentText
						return &llm.Event{
							Type:  llm.EventTypeContentBlockDelta,
							Index: &s.textContentIndex,
							Delta: &llm.EventDelta{
								Type: llm.EventDeltaTypeText,
								Text: deltaText,
							},
						}
					}
				}
			}
		case "function_call":
			// Handle tool call events
			toolIndex := s.nextContentIndex
			s.nextContentIndex++
			return &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &toolIndex,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeToolUse,
					ID:   item.CallID,
					Name: item.Name,
				},
			}
		case "mcp_call":
			// Handle MCP tool call events
			toolIndex := s.nextContentIndex
			s.nextContentIndex++
			return &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &toolIndex,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeToolUse,
					ID:   item.ID,
					Name: item.Name,
				},
			}
		case "mcp_list_tools":
			// Handle MCP tool list events - emit as text content
			if len(item.Tools) > 0 {
				toolIndex := s.nextContentIndex
				s.nextContentIndex++
				var toolsText strings.Builder
				toolsText.WriteString(fmt.Sprintf("MCP server '%s' tools:\n", item.ServerLabel))
				for _, tool := range item.Tools {
					toolsText.WriteString(fmt.Sprintf("- %s\n", tool.Name))
				}
				return &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &toolIndex,
					ContentBlock: &llm.EventContentBlock{
						Type: llm.ContentTypeText,
						Text: toolsText.String(),
					},
				}
			}
		case "mcp_approval_request":
			// Handle MCP approval request events - emit as text content
			toolIndex := s.nextContentIndex
			s.nextContentIndex++
			return &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &toolIndex,
				ContentBlock: &llm.EventContentBlock{
					Type: llm.ContentTypeText,
					Text: fmt.Sprintf("MCP approval required for tool '%s' on server '%s'", item.Name, item.ServerLabel),
				},
			}
		}
	}

	// If we have usage information, emit a message delta event
	if response.Usage != nil {
		return &llm.Event{
			Type:  llm.EventTypeMessageDelta,
			Delta: &llm.EventDelta{}, // Empty delta is required for message delta events
			Usage: &llm.Usage{
				InputTokens:  response.Usage.InputTokens,
				OutputTokens: response.Usage.OutputTokens,
			},
		}
	}

	return nil
}

// Event returns the current event
func (s *StreamIterator) Event() *llm.Event {
	return s.currentEvent
}

// Err returns any error that occurred
func (s *StreamIterator) Err() error {
	return s.err
}

// Close closes the stream
func (s *StreamIterator) Close() error {
	if s.body != nil {
		return s.body.Close()
	}
	return nil
}
