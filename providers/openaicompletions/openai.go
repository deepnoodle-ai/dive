package openaicompletions

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

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/retry"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

// SystemPromptBehavior describes how the system prompt should be handled for a
// given model.
type SystemPromptBehavior string

const (
	// SystemPromptBehaviorOmit instructs the provider to omit the system prompt
	// from the request.
	SystemPromptBehaviorOmit SystemPromptBehavior = "omit"

	// SystemPromptBehaviorUser instructs the provider to add a user message with
	// the system prompt to the beginning of the request.
	SystemPromptBehaviorUser SystemPromptBehavior = "user"
)

// ToolBehavior describes how tools should be handled for a given model.
type ToolBehavior string

const (
	ToolBehaviorOmit  ToolBehavior = "omit"
	ToolBehaviorError ToolBehavior = "error"
)

var (
	DefaultModel              = ModelGPT5
	DefaultEndpoint           = "https://api.openai.com/v1/chat/completions"
	DefaultMaxTokens          = 4096
	DefaultSystemRole         = "developer"
	DefaultClient             = &http.Client{Timeout: 300 * time.Second}
	DefaultMaxRetries         = 6
	DefaultRetryBaseWait      = 2 * time.Second
	ModelSystemPromptBehavior = map[string]SystemPromptBehavior{
		"o1-mini": SystemPromptBehaviorOmit,
	}
	ModelToolBehavior = map[string]ToolBehavior{
		"o1-mini": ToolBehaviorOmit,
	}
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
	systemRole    string
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
		systemRole:    DefaultSystemRole,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provider) Name() string {
	return "openai-completions"
}

func (p *Provider) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	config := &llm.Config{}
	config.Apply(opts...)

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}

	if err := validateMessages(config.Messages); err != nil {
		return nil, err
	}
	msgs, err := convertMessages(config.Messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}
	if config.Prefill != "" {
		msgs = append(msgs, Message{Role: "assistant", Content: config.Prefill})
	}

	request.Messages = msgs
	addSystemPrompt(&request, config.SystemPrompt, p.systemRole)

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
	err = retry.DoSimple(ctx, func() error {
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
	}, retry.WithMaxAttempts(p.maxRetries+1), retry.WithBackoff(p.retryBaseWait, 5*time.Minute))

	if err != nil {
		return nil, err
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty response from openai api")
	}
	choice := result.Choices[0]

	var contentBlocks []llm.Content
	if choice.Message.Content != "" {
		contentBlocks = append(contentBlocks, &llm.TextContent{Text: choice.Message.Content})
	}

	// Transform tool calls into content blocks (like Anthropic)
	if len(choice.Message.ToolCalls) > 0 {
		for _, toolCall := range choice.Message.ToolCalls {
			contentBlocks = append(contentBlocks, &llm.ToolUseContent{
				ID:    toolCall.ID, // e.g. call_12345xyz
				Name:  toolCall.Function.Name,
				Input: []byte(toolCall.Function.Arguments),
			})
		}
	}

	if config.Prefill != "" {
		for _, block := range contentBlocks {
			if textContent, ok := block.(*llm.TextContent); ok {
				if config.PrefillClosingTag == "" ||
					strings.Contains(textContent.Text, config.PrefillClosingTag) {
					textContent.Text = config.Prefill + textContent.Text
				}
				break
			}
		}
	}

	response := &llm.Response{
		ID:      result.ID,
		Model:   p.model,
		Role:    llm.Assistant,
		Content: contentBlocks,
		Usage: llm.Usage{
			InputTokens:  result.Usage.PromptTokens,
			OutputTokens: result.Usage.CompletionTokens,
		},
	}

	if err := config.FireHooks(ctx, &llm.HookContext{
		Type: llm.AfterGenerate,
		Request: &llm.HookRequestContext{
			Messages: config.Messages,
			Config:   config,
			Body:     body,
		},
		Response: &llm.HookResponseContext{
			Response: response,
		},
	}); err != nil {
		return nil, err
	}
	return response, nil
}

func (p *Provider) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
	// Relevant OpenAI API documentation:
	// https://platform.openai.com/docs/api-reference/chat/create

	config := &llm.Config{}
	config.Apply(opts...)

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}

	if err := validateMessages(config.Messages); err != nil {
		return nil, err
	}
	msgs, err := convertMessages(config.Messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}
	if config.Prefill != "" {
		msgs = append(msgs, Message{Role: "assistant", Content: config.Prefill})
	}

	request.Messages = msgs
	request.Stream = true
	request.StreamOptions = &StreamOptions{IncludeUsage: true}
	addSystemPrompt(&request, config.SystemPrompt, p.systemRole)

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
	err = retry.DoSimple(ctx, func() error {
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
			body:              resp.Body,
			reader:            bufio.NewReader(resp.Body),
			contentBlocks:     map[int]*ContentBlockAccumulator{},
			toolCalls:         map[int]*ToolCallAccumulator{},
			prefill:           config.Prefill,
			prefillClosingTag: config.PrefillClosingTag,
		}
		return nil
	}, retry.WithMaxAttempts(p.maxRetries+1), retry.WithBackoff(p.retryBaseWait, 5*time.Minute))

	if err != nil {
		return nil, err
	}
	return stream, nil
}

func validateMessages(messages []*llm.Message) error {
	messageCount := len(messages)
	if messageCount == 0 {
		return fmt.Errorf("no messages provided")
	}
	for i, message := range messages {
		if len(message.Content) == 0 {
			return fmt.Errorf("empty message detected (index %d)", i)
		}
	}
	return nil
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
			switch c := c.(type) {
			case *llm.ToolUseContent:
				hasToolUse = true
				toolCalls = append(toolCalls, ToolCall{
					ID:   c.ID,
					Type: "function",
					Function: ToolCallFunction{
						Name:      c.Name,
						Arguments: string(c.Input),
					},
				})
			case *llm.TextContent:
				textContent = c.Text
			case *llm.ToolResultContent:
				hasToolResult = true
			}
		}

		// Create a single message for all tool use content blocks
		if hasToolUse {
			result = append(result, Message{
				Role:      role,
				Content:   textContent,
				ToolCalls: toolCalls,
			})
		}

		// Process non-tool-use content blocks
		if !hasToolUse || hasToolResult {
			for _, c := range msg.Content {
				switch c := c.(type) {
				case *llm.TextContent:
					if !hasToolUse {
						result = append(result, Message{Role: role, Content: c.Text})
					}
				case *llm.ToolResultContent:
					// Each tool result goes in its own message
					var contentStr string
					switch content := c.Content.(type) {
					case string:
						contentStr = content
					case []*dive.ToolResultContent:
						var texts []string
						for _, c := range content {
							if c.Text != "" {
								texts = append(texts, c.Text)
							}
						}
						contentStr = strings.Join(texts, "\n")
					default:
						return nil, fmt.Errorf("unsupported tool result content type")
					}
					result = append(result, Message{
						Role:       "tool",
						Content:    contentStr,
						ToolCallID: c.ToolUseID,
					})
				case *llm.ToolUseContent:
					// Already handled above
				default:
					return nil, fmt.Errorf("unsupported content type: %s", c.Type())
				}
			}
		}
	}
	return result, nil
}

func (p *Provider) applyRequestConfig(req *Request, config *llm.Config) error {
	if model := config.Model; model != "" {
		req.Model = model
	} else {
		req.Model = p.model
	}

	var maxTokens int
	if ptr := config.MaxTokens; ptr != nil {
		maxTokens = *ptr
	} else {
		maxTokens = p.maxTokens
	}

	if maxTokens > 0 {
		if strings.HasPrefix(req.Model, "o") || strings.HasPrefix(req.Model, "gpt-5") {
			req.MaxCompletionTokens = &maxTokens
		} else {
			req.MaxTokens = &maxTokens
		}
	}

	var tools []Tool
	for _, tool := range config.Tools {
		tools = append(tools, Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.Schema(),
			},
		})
	}

	if behavior, ok := ModelToolBehavior[req.Model]; ok {
		if behavior == ToolBehaviorError {
			if len(config.Tools) > 0 {
				return fmt.Errorf("model %q does not support tools", req.Model)
			}
		} else if behavior == ToolBehaviorOmit {
			tools = []Tool{}
		}
	}

	var toolChoice any
	if len(tools) > 0 {
		toolChoice = "auto"
		if config.ToolChoice != nil {
			switch config.ToolChoice.Type {
			case llm.ToolChoiceTypeAny:
				toolChoice = "required"
			case llm.ToolChoiceTypeNone:
				toolChoice = "none"
			case llm.ToolChoiceTypeAuto:
				toolChoice = "auto"
			case llm.ToolChoiceTypeTool:
				toolChoice = map[string]any{
					"type":     "function",
					"function": map[string]any{"name": config.ToolChoice.Name},
				}
			default:
				return fmt.Errorf("invalid tool choice type: %s", config.ToolChoice.Type)
			}
		}
		req.ToolChoice = toolChoice
	}

	req.Tools = tools
	req.Temperature = config.Temperature
	req.PresencePenalty = config.PresencePenalty
	req.FrequencyPenalty = config.FrequencyPenalty
	req.ReasoningEffort = ReasoningEffort(config.ReasoningEffort)
	return nil
}

func addSystemPrompt(request *Request, systemPrompt, defaultSystemRole string) {
	if systemPrompt == "" {
		return
	}
	if behavior, ok := ModelSystemPromptBehavior[request.Model]; ok {
		switch behavior {
		case SystemPromptBehaviorOmit:
			return
		case SystemPromptBehaviorUser:
			message := Message{
				Role:    "user",
				Content: systemPrompt,
			}
			request.Messages = append([]Message{message}, request.Messages...)
		}
		return
	}
	request.Messages = append([]Message{{
		Role:    defaultSystemRole,
		Content: systemPrompt,
	}}, request.Messages...)
}

// createRequest creates an HTTP request with appropriate headers for OpenAI API calls
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
