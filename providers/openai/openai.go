package openai

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

	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/providers"
	"github.com/getstingrai/dive/retry"
)

var (
	DefaultModel            = "gpt-4o"
	DefaultMessagesEndpoint = "https://api.openai.com/v1/chat/completions"
	DefaultMaxTokens        = 4096
	DefaultSystemRole       = "developer"
)

var _ llm.LLM = &Provider{}

type Provider struct {
	apiKey     string
	endpoint   string
	model      string
	systemRole string
	maxTokens  int
	client     *http.Client
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:     os.Getenv("OPENAI_API_KEY"),
		endpoint:   DefaultMessagesEndpoint,
		model:      DefaultModel,
		maxTokens:  DefaultMaxTokens,
		client:     http.DefaultClient,
		systemRole: DefaultSystemRole,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provider) Generate(ctx context.Context, messages []*llm.Message, opts ...llm.Option) (*llm.Response, error) {
	config := &llm.Config{}
	for _, opt := range opts {
		opt(config)
	}

	if hooks := config.Hooks[llm.BeforeGenerate]; hooks != nil {
		hooks(ctx, &llm.HookContext{
			Type:     llm.BeforeGenerate,
			Messages: messages,
		})
	}

	model := config.Model
	if model == "" {
		model = p.model
	}

	maxTokens := config.MaxTokens
	if maxTokens == nil {
		maxTokens = &p.maxTokens
	}

	msgs, err := convertMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}

	messageCount := len(messages)
	if messageCount == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	for i, message := range messages {
		if len(message.Content) == 0 {
			return nil, fmt.Errorf("empty message detected (index %d)", i)
		}
	}

	var tools []Tool
	for _, tool := range config.Tools {
		tools = append(tools, Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        tool.Definition().Name,
				Description: tool.Definition().Description,
				Parameters:  tool.Definition().Parameters,
			},
		})
	}

	var toolChoice string
	if config.ToolChoice.Type != "" {
		toolChoice = config.ToolChoice.Type
	} else if len(tools) > 0 {
		toolChoice = "auto"
	}

	reqBody := Request{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: config.Temperature,
		Tools:       tools,
		ToolChoice:  toolChoice,
	}

	if config.SystemPrompt != "" {
		reqBody.Messages = append([]Message{{
			Role:    p.systemRole,
			Content: config.SystemPrompt,
		}}, reqBody.Messages...)
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	var result Response
	err = retry.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(jsonBody))
		if err != nil {
			return fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
		req.Header.Set("Content-Type", "application/json")
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
	}, retry.WithMaxRetries(6))
	if err != nil {
		return nil, err
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty response from openai api")
	}
	choice := result.Choices[0]

	var toolCalls []llm.ToolCall
	var contentBlocks []*llm.Content
	if len(choice.Message.ToolCalls) > 0 {
		for _, toolCall := range choice.Message.ToolCalls {
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:    toolCall.ID,
				Name:  toolCall.Function.Name,
				Input: toolCall.Function.Arguments,
			})
			contentBlocks = append(contentBlocks, &llm.Content{
				Type:  llm.ContentTypeToolUse,
				ID:    toolCall.ID, // e.g. call_12345xyz
				Name:  toolCall.Function.Name,
				Input: json.RawMessage(toolCall.Function.Arguments),
			})
		}
	} else {
		contentBlocks = append(contentBlocks, &llm.Content{
			Type: llm.ContentTypeText,
			Text: choice.Message.Content,
		})
	}

	response := llm.NewResponse(llm.ResponseOptions{
		ID:    result.ID,
		Model: model,
		Role:  llm.Assistant,
		Usage: llm.Usage{
			InputTokens:  result.Usage.PromptTokens,
			OutputTokens: result.Usage.CompletionTokens,
		},
		Message:   llm.NewMessage(llm.Assistant, contentBlocks),
		ToolCalls: toolCalls,
	})

	if hooks := config.Hooks[llm.AfterGenerate]; hooks != nil {
		hooks(ctx, &llm.HookContext{
			Type:     llm.AfterGenerate,
			Messages: messages,
			Response: response,
		})
	}

	return response, nil
}

func (p *Provider) Stream(ctx context.Context, messages []*llm.Message, opts ...llm.Option) (llm.Stream, error) {
	config := &llm.Config{}
	for _, opt := range opts {
		opt(config)
	}

	model := config.Model
	if model == "" {
		model = p.model
	}

	msgs, err := convertMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}

	maxTokens := config.MaxTokens
	if maxTokens == nil {
		maxTokens = &p.maxTokens
	}

	reqBody := Request{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: config.Temperature,
		Stream:      true,
	}

	if config.SystemPrompt != "" {
		reqBody.Messages = append([]Message{{
			Role:    p.systemRole,
			Content: config.SystemPrompt,
		}}, reqBody.Messages...)
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("error from API (status %d): %s", resp.StatusCode, string(body))
	}

	return &Stream{
		reader: bufio.NewReader(resp.Body),
		body:   resp.Body,
	}, nil
}

func (p *Provider) SupportsStreaming() bool {
	return true
}

func convertMessages(messages []*llm.Message) ([]Message, error) {
	var result []Message
	for _, msg := range messages {
		role := strings.ToLower(string(msg.Role))

		// Group all tool use content blocks into a single message
		var toolCalls []ToolCall
		var textContent string
		var hasToolUse bool
		var hasToolResult bool

		// First pass: collect all tool use content blocks and check for tool results
		for _, c := range msg.Content {
			if c.Type == llm.ContentTypeToolUse {
				hasToolUse = true
				toolCalls = append(toolCalls, ToolCall{
					ID:   c.ID,
					Type: "function",
					Function: ToolCallFunction{
						Name:      c.Name,
						Arguments: string(c.Input),
					},
				})
			} else if c.Type == llm.ContentTypeText {
				textContent = c.Text
			} else if c.Type == llm.ContentTypeToolResult {
				hasToolResult = true
			}
		}

		// Create a single message for all tool use content blocks
		if hasToolUse {
			result = append(result, Message{
				Role:      role,
				Content:   textContent, // Can be empty for pure tool use messages
				ToolCalls: toolCalls,
			})
		}

		// Process non-tool-use content blocks
		if !hasToolUse || hasToolResult {
			for _, c := range msg.Content {
				switch c.Type {
				case llm.ContentTypeText:
					if !hasToolUse { // Only add text content if not already added with tool calls
						result = append(result, Message{Role: role, Content: c.Text})
					}
				case llm.ContentTypeToolResult:
					// Each tool result goes in its own message
					result = append(result, Message{
						Role:       "tool",
						Content:    c.Text,
						ToolCallID: c.ToolUseID,
					})
				case llm.ContentTypeToolUse:
					// Already handled above
				default:
					return nil, fmt.Errorf("unsupported content type: %s", c.Type)
				}
			}
		}
	}
	return result, nil
}

type Stream struct {
	reader *bufio.Reader
	body   io.ReadCloser
	err    error
}

func (s *Stream) Next(ctx context.Context) (*llm.StreamEvent, bool) {
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			s.err = err
			return nil, false
		}

		// Skip empty lines
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		// Remove "data: " prefix if present
		line = bytes.TrimPrefix(line, []byte("data: "))

		// Check for stream end
		if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
			return nil, false
		}

		var event StreamResponse
		if err := json.Unmarshal(line, &event); err != nil {
			continue // Skip malformed events
		}

		if len(event.Choices) > 0 {
			choice := event.Choices[0]
			if choice.Delta.Content != "" {
				return &llm.StreamEvent{
					Type: llm.EventContentBlockDelta,
					Delta: &llm.Delta{
						Type: "text_delta",
						Text: choice.Delta.Content,
					},
				}, true
			}
		}
	}
}

func (s *Stream) Close() error {
	return s.body.Close()
}

func (s *Stream) Err() error {
	return s.err
}
