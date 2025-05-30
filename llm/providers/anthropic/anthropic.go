package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers"
	"github.com/diveagents/dive/retry"
)

var (
	DefaultModel         = ModelClaudeSonnet4
	DefaultEndpoint      = "https://api.anthropic.com/v1/messages"
	DefaultMaxTokens     = 4096
	DefaultClient        = &http.Client{Timeout: 300 * time.Second}
	DefaultMaxRetries    = 6
	DefaultRetryBaseWait = 2 * time.Second
	DefaultVersion       = "2023-06-01"
)

const (
	FeatureExtendedCache = "extended-cache-ttl-2025-04-11"
	FeaturePromptCaching = "prompt-caching-2024-07-31"
	FeatureMCPClient     = "mcp-client-2025-04-04"
	FeatureCodeExecution = "code-execution-2025-05-22"
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
	version       string
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:        os.Getenv("ANTHROPIC_API_KEY"),
		endpoint:      DefaultEndpoint,
		client:        DefaultClient,
		model:         DefaultModel,
		maxTokens:     DefaultMaxTokens,
		maxRetries:    DefaultMaxRetries,
		retryBaseWait: DefaultRetryBaseWait,
		version:       DefaultVersion,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provider) Name() string {
	return "anthropic"
}

func (p *Provider) ModelName() string {
	return p.model
}

func (p *Provider) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	config := &llm.Config{}
	config.Apply(opts...)

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}
	msgs, err := convertMessages(config.Messages)
	if err != nil {
		return nil, err
	}
	if config.Prefill != "" {
		msgs = append(msgs, llm.NewAssistantTextMessage(config.Prefill))
	}
	request.Messages = msgs

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

	var result llm.Response
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
	}, retry.WithMaxRetries(p.maxRetries), retry.WithBaseWait(p.retryBaseWait))

	if err != nil {
		return nil, err
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from anthropic api")
	}
	if config.Prefill != "" {
		addPrefill(result.Content, config.Prefill, config.PrefillClosingTag)
	}

	if err := config.FireHooks(ctx, &llm.HookContext{
		Type: llm.AfterGenerate,
		Request: &llm.HookRequestContext{
			Messages: config.Messages,
			Config:   config,
			Body:     body,
		},
		Response: &llm.HookResponseContext{
			Response: &result,
		},
	}); err != nil {
		return nil, err
	}
	return &result, nil
}

func addPrefill(blocks []llm.Content, prefill, closingTag string) error {
	if prefill == "" {
		return nil
	}
	for _, block := range blocks {
		content, ok := block.(*llm.TextContent)
		if ok {
			if closingTag == "" || strings.Contains(content.Text, closingTag) {
				content.Text = prefill + content.Text
				return nil
			}
			return fmt.Errorf("prefill closing tag not found")
		}
	}
	return fmt.Errorf("no text content found in message")
}

func (p *Provider) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
	config := &llm.Config{}
	config.Apply(opts...)

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}
	msgs, err := convertMessages(config.Messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}
	if config.Prefill != "" {
		msgs = append(msgs, llm.NewAssistantTextMessage(config.Prefill))
	}
	request.Messages = msgs
	request.Stream = true

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
			body: resp.Body,
			reader: llm.NewServerSentEventsReader[llm.Event](resp.Body).
				WithSSECallback(config.SSECallback),
			prefill:           config.Prefill,
			prefillClosingTag: config.PrefillClosingTag,
		}
		return nil
	}, retry.WithMaxRetries(p.maxRetries), retry.WithBaseWait(p.retryBaseWait))

	if err != nil {
		return nil, err
	}
	return stream, nil
}

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
	// Workaround for Anthropic bug
	reorderMessageContent(messages)
	// Anthropic errors if a message ID is set, so make a copy of the messages
	// and omit the ID field
	copied := make([]*llm.Message, len(messages))
	for i, message := range messages {
		// The "name" field in tool results can't be set either
		var copiedContent []llm.Content
		for _, content := range message.Content {
			switch c := content.(type) {
			case *llm.ToolResultContent:
				copiedContent = append(copiedContent, &llm.ToolResultContent{
					Content:   c.Content,
					ToolUseID: c.ToolUseID,
				})
			case *llm.DocumentContent:
				// Handle DocumentContent with file IDs for Anthropic API compatibility
				if c.Source != nil && c.Source.Type == llm.ContentSourceTypeFile && c.Source.FileID != "" {
					// For Anthropic API, file IDs are passed in the source structure
					docContent := &llm.DocumentContent{
						Title:        c.Title,
						Context:      c.Context,
						Citations:    c.Citations,
						CacheControl: c.CacheControl,
						Source: &llm.ContentSource{
							Type:   c.Source.Type,
							FileID: c.Source.FileID,
						},
					}
					copiedContent = append(copiedContent, docContent)
				} else {
					// Pass through other DocumentContent as-is
					copiedContent = append(copiedContent, content)
				}
			default:
				copiedContent = append(copiedContent, content)
			}
		}
		copied[i] = &llm.Message{
			Role:    message.Role,
			Content: copiedContent,
		}
	}
	return copied, nil
}

func (p *Provider) applyRequestConfig(req *Request, config *llm.Config) error {
	if model := config.Model; model != "" {
		req.Model = model
	} else {
		req.Model = p.model
	}
	if maxTokens := config.MaxTokens; maxTokens != nil {
		req.MaxTokens = maxTokens
	} else {
		req.MaxTokens = &p.maxTokens
	}

	if config.ReasoningBudget != nil {
		budget := *config.ReasoningBudget
		if budget < 1024 {
			return fmt.Errorf("reasoning budget must be at least 1024")
		}
		req.Thinking = &Thinking{
			Type:         "enabled",
			BudgetTokens: budget,
		}
	}

	// Compatibility with the OpenAI "effort" parameter
	if config.ReasoningEffort != "" {
		if req.Thinking != nil {
			return fmt.Errorf("cannot set both reasoning budget and effort")
		}
		req.Thinking = &Thinking{Type: "enabled"}
		switch config.ReasoningEffort {
		case "low":
			req.Thinking.BudgetTokens = 1024
		case "medium":
			req.Thinking.BudgetTokens = 4096
		case "high":
			req.Thinking.BudgetTokens = 16384
		default:
			return fmt.Errorf("invalid reasoning effort: %s", config.ReasoningEffort)
		}
	}

	if len(config.Tools) > 0 {
		var tools []map[string]any
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
			schema := tool.Schema()
			toolConfig := map[string]any{
				"name":        tool.Name(),
				"description": tool.Description(),
			}
			if schema.Type != "" {
				toolConfig["input_schema"] = schema
			}
			tools = append(tools, toolConfig)
		}
		req.Tools = tools
	}

	if config.ToolChoice != "" {
		req.ToolChoice = &ToolChoice{
			Type: ToolChoiceType(config.ToolChoice),
			Name: config.ToolChoiceName,
		}
		if config.ParallelToolCalls != nil && !*config.ParallelToolCalls {
			req.ToolChoice.DisableParallelUse = true
		}
	}

	if len(config.MCPServers) > 0 {
		req.MCPServers = config.MCPServers
	}

	req.Temperature = config.Temperature
	req.System = config.SystemPrompt
	return nil
}

func reorderMessageContent(messages []*llm.Message) {
	// For each assistant message, reorder content blocks so that all tool_use
	// content blocks appear after non-tool_use content blocks, while preserving
	// relative ordering within each group. This works-around an Anthropic bug.
	for _, msg := range messages {
		if msg.Role != llm.Assistant || len(msg.Content) < 2 {
			continue
		}
		// Separate blocks into tool use and non-tool use
		var toolUseBlocks []llm.Content
		var otherBlocks []llm.Content
		for _, block := range msg.Content {
			if block.Type() == llm.ContentTypeToolUse {
				toolUseBlocks = append(toolUseBlocks, block)
			} else {
				otherBlocks = append(otherBlocks, block)
			}
		}
		// If we found any tool use blocks and other blocks, reorder them
		if len(toolUseBlocks) > 0 && len(otherBlocks) > 0 {
			// Combine slices with non-tool-use blocks first
			msg.Content = append(otherBlocks, toolUseBlocks...)
		}
	}
}

// StreamIterator implements the llm.StreamIterator interface for Anthropic streaming responses
type StreamIterator struct {
	reader            *llm.ServerSentEventsReader[llm.Event]
	body              io.ReadCloser
	err               error
	currentEvent      *llm.Event
	prefill           string
	prefillClosingTag string
	closeOnce         sync.Once
}

// Next advances to the next event in the stream. Returns true if an event was
// successfully read, false when the stream is complete or an error occurs.
func (s *StreamIterator) Next() bool {
	for {
		event, ok := s.reader.Next()
		if !ok {
			s.err = s.reader.Err()
			s.Close()
			return false
		}
		processedEvent := s.processEvent(&event)
		if processedEvent != nil {
			s.currentEvent = processedEvent
			return true
		}
	}
}

// Event returns the current event. Should only be called after a successful Next().
func (s *StreamIterator) Event() *llm.Event {
	return s.currentEvent
}

// processEvent processes an Anthropic event and applies prefill logic if needed
func (s *StreamIterator) processEvent(event *llm.Event) *llm.Event {
	if event.Type == "" {
		return nil
	}

	// Apply prefill logic for the first text content block
	if s.prefill != "" && event.Type == llm.EventTypeContentBlockStart {
		if event.ContentBlock != nil && event.ContentBlock.Type == llm.ContentTypeText {
			// Add prefill to the beginning of the text
			if s.prefillClosingTag == "" || strings.Contains(event.ContentBlock.Text, s.prefillClosingTag) {
				event.ContentBlock.Text = s.prefill + event.ContentBlock.Text
				s.prefill = "" // Only apply prefill once
			}
		}
	}

	return event
}

func (s *StreamIterator) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.body.Close() })
	return err
}

func (s *StreamIterator) Err() error {
	return s.err
}

// createRequest creates an HTTP request with appropriate headers for Anthropic API calls
func (p *Provider) createRequest(ctx context.Context, body []byte, config *llm.Config, isStreaming bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", p.version)
	req.Header.Set("content-type", "application/json")

	if isStreaming {
		req.Header.Set("accept", "text/event-stream")
	}

	if config.IsFeatureEnabled(FeatureExtendedCache) {
		req.Header.Add("anthropic-beta", FeatureExtendedCache)
	} else if config.IsFeatureEnabled(FeaturePromptCaching) {
		req.Header.Add("anthropic-beta", FeaturePromptCaching)
	} else if config.Caching == nil || *config.Caching {
		req.Header.Add("anthropic-beta", FeaturePromptCaching)
	}

	if config.IsFeatureEnabled(FeatureOutput128k) {
		req.Header.Add("anthropic-beta", FeatureOutput128k)
	}

	if config.IsFeatureEnabled(FeatureMCPClient) || len(config.MCPServers) > 0 {
		req.Header.Add("anthropic-beta", FeatureMCPClient)
	}

	if config.IsFeatureEnabled(FeatureCodeExecution) {
		req.Header.Add("anthropic-beta", FeatureCodeExecution)
	}

	for key, values := range config.RequestHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return req, nil
}
