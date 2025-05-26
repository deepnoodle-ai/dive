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
	DefaultModel    = "gpt-4.1"
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
	// Responses API specific fields
	store      *bool
	background *bool
	// Built-in tools configuration
	enabledTools           []string
	webSearchOptions       *WebSearchOptions
	imageGenerationOptions *ImageGenerationOptions
	mcpServers             map[string]MCPServerConfig
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:     os.Getenv("OPENAI_API_KEY"),
		endpoint:   DefaultEndpoint,
		client:     http.DefaultClient,
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
		Store:       p.store,
		Background:  p.background,
		Temperature: config.Temperature,
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

	// Add custom function tools
	for _, tool := range config.Tools {
		request.Tools = append(request.Tools, Tool{
			Type: "function",
			Function: &FunctionTool{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Schema(),
			},
		})
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
				if content.Type == "output_text" {
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
				// For now, we'll represent this as text content with the base64 data
				// In a real implementation, you might want a specific ImageContent type
				contentBlocks = append(contentBlocks, &llm.TextContent{
					Text: fmt.Sprintf("Generated image (base64): %s", item.Result[:50]+"..."),
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
				inputMsg.Content = append(inputMsg.Content, InputContent{
					Type: "input_text",
					Text: c.Text,
				})
				// TODO: Handle other content types (images, etc.)
			}
		}

		inputMessages = append(inputMessages, inputMsg)
	}

	return inputMessages, nil
}

// buildTools builds the tools array for the request
func (p *Provider) buildTools(config *llm.Config) ([]Tool, error) {
	var tools []Tool

	// Add enabled built-in tools
	for _, toolType := range p.enabledTools {
		switch toolType {
		case "web_search_preview":
			tool := Tool{Type: "web_search_preview"}
			if p.webSearchOptions != nil {
				tool.Domains = p.webSearchOptions.Domains
				tool.SearchContextSize = p.webSearchOptions.SearchContextSize
				tool.UserLocation = p.webSearchOptions.UserLocation
			}
			tools = append(tools, tool)

		case "image_generation":
			tool := Tool{Type: "image_generation"}
			if p.imageGenerationOptions != nil {
				tool.Size = p.imageGenerationOptions.Size
				tool.Quality = p.imageGenerationOptions.Quality
				// Note: Format parameter is not supported by the API
				// tool.Format = p.imageGenerationOptions.Format
				tool.Compression = p.imageGenerationOptions.Compression
				tool.Background = p.imageGenerationOptions.Background
				tool.PartialImages = p.imageGenerationOptions.PartialImages
			}
			tools = append(tools, tool)
		}
	}

	// Add MCP servers
	for label, mcpConfig := range p.mcpServers {
		tools = append(tools, Tool{
			Type:            "mcp",
			ServerLabel:     label,
			ServerURL:       mcpConfig.ServerURL,
			AllowedTools:    mcpConfig.AllowedTools,
			RequireApproval: mcpConfig.RequireApproval,
			Headers:         mcpConfig.Headers,
		})
	}

	return tools, nil
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
						index := 0
						return &llm.Event{
							Type:  llm.EventTypeContentBlockStart,
							Index: &index,
							ContentBlock: &llm.EventContentBlock{
								Type: llm.ContentTypeText,
								Text: currentText,
							},
						}
					}

					// If the text has changed, emit a delta event with the new text
					if currentText != s.previousText {
						deltaText := currentText
						// For simplicity, we'll send the full text as delta
						// In a real streaming scenario, we'd calculate the actual delta
						s.previousText = currentText
						index := 0
						return &llm.Event{
							Type:  llm.EventTypeContentBlockDelta,
							Index: &index,
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
			index := 0 // For simplicity, use index 0
			return &llm.Event{
				Type:  llm.EventTypeContentBlockStart,
				Index: &index,
				ContentBlock: &llm.EventContentBlock{
					Type: "tool_use",
					ID:   item.CallID,
					Name: item.Name,
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
