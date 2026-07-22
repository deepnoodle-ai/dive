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
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/wonton/retry"
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
	DefaultModel              = ModelGPT55
	DefaultEndpoint           = "https://api.openai.com/v1/chat/completions"
	DefaultMaxTokens          = 16384
	DefaultSystemRole         = "developer"
	DefaultClient             = &http.Client{Timeout: 300 * time.Second}
	DefaultMaxRetries         = 3
	DefaultRetryBaseWait      = 2 * time.Second
	ModelSystemPromptBehavior = map[string]SystemPromptBehavior{
		"o1-mini": SystemPromptBehaviorOmit,
	}
	ModelToolBehavior = map[string]ToolBehavior{
		"o1-mini": ToolBehaviorOmit,
	}
)

var _ llm.StreamingLLM = &Provider{}

// Provider implements an LLM provider using the OpenAI Chat Completions API.
// This is used as the base for several providers including Grok, Groq, Mistral,
// Ollama, and OpenRouter.
type Provider struct {
	name          string
	client        *http.Client
	apiKey        string
	endpoint      string
	model         string
	maxTokens     int
	maxRetries    int
	retryBaseWait time.Duration
	systemRole    string
}

// New creates a new OpenAI Completions provider with the given options.
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
	if p.name != "" {
		return p.name
	}
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
	}, retry.WithMaxAttempts(p.maxRetries+1), retry.WithBackoff(p.retryBaseWait, 5*time.Minute), retry.WithRetryIf(retry.SkipPermanent()))

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
		Usage:   result.Usage.toLLMUsage(),
	}

	llm.PopulateCost(response.Model, response.Usage.Speed == string(llm.SpeedFast), &response.Usage)

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

	stream := providers.NewRetryingStreamIterator(ctx, providers.StreamRetryConfig{
		Provider:      p.Name(),
		MaxRetries:    p.maxRetries,
		RetryBaseWait: p.retryBaseWait,
		Logger:        config.Logger,
	}, func() (llm.StreamIterator, error) {
		req, err := p.createRequest(ctx, body, config, true)
		if err != nil {
			return nil, err
		}

		resp, err := p.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error making request: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, providers.NewError(resp.StatusCode, string(body))
		}
		return &StreamIterator{
			body:              resp.Body,
			reader:            bufio.NewReader(resp.Body),
			contentBlocks:     map[int]*ContentBlockAccumulator{},
			toolCalls:         map[int]*ToolCallAccumulator{},
			toolCallIndices:   map[int]int{},
			prefill:           config.Prefill,
			prefillClosingTag: config.PrefillClosingTag,
			thinkingIndex:     -1,
			textIndex:         -1,
		}, nil
	})
	return stream, nil
}

func validateMessages(messages []*llm.Message) error {
	messageCount := len(messages)
	if messageCount == 0 {
		return fmt.Errorf("no messages provided")
	}
	// Empty messages are silently skipped during conversion rather than erroring
	return nil
}

func convertMessages(messages []*llm.Message) ([]Message, error) {
	// Chat Completions has no operator-authority role, so operator reminders
	// always render as tagged user messages (nil resolver = no native authority).
	messages, err := llm.RenderReminders(messages, nil)
	if err != nil {
		return nil, err
	}
	var result []Message
	for _, msg := range messages {
		// Skip empty messages - they can occur in edge cases during long tool-calling loops
		if len(msg.Content) == 0 {
			continue
		}
		role := strings.ToLower(string(msg.Role))

		// Partition the content blocks: tool calls, tool results, and the
		// remaining text/media content parts (order preserved).
		var toolCalls []ToolCall
		var toolResults []*llm.ToolResultContent
		var parts []ContentPart
		var hasMedia bool
		for _, c := range msg.Content {
			switch c := c.(type) {
			case *llm.ToolUseContent:
				toolCalls = append(toolCalls, ToolCall{
					ID:   c.ID,
					Type: "function",
					Function: ToolCallFunction{
						Name:      c.Name,
						Arguments: string(c.Input),
					},
				})
			case *llm.ToolResultContent:
				toolResults = append(toolResults, c)
			case *llm.TextContent:
				parts = append(parts, ContentPart{Type: "text", Text: c.Text})
			case *llm.ImageContent:
				part, err := encodeImageContentPart(c)
				if err != nil {
					return nil, err
				}
				parts = append(parts, part)
				hasMedia = true
			case *llm.DocumentContent:
				part, err := encodeDocumentContentPart(c)
				if err != nil {
					return nil, err
				}
				parts = append(parts, part)
				hasMedia = true
			case *llm.ThinkingContent, *llm.RedactedThinkingContent:
				// The Chat Completions API has no standard field for
				// replaying assistant reasoning back to the server, so
				// thinking content (which this provider's own stream
				// iterator can produce from "reasoning" deltas) is
				// skipped on encode rather than erroring.
			default:
				return nil, fmt.Errorf("unsupported content type: %s", c.Type())
			}
		}
		if hasMedia && role == "assistant" {
			return nil, fmt.Errorf("image and document content is not supported in assistant messages by the chat completions API")
		}

		// A single message carries all tool calls, plus any accompanying text.
		if len(toolCalls) > 0 {
			result = append(result, Message{
				Role:      role,
				Content:   joinTextParts(parts),
				ToolCalls: toolCalls,
			})
			parts = nil
		}

		// One "tool" message per tool result.
		for _, trc := range toolResults {
			toolMessage, err := convertToolResultContent(trc)
			if err != nil {
				return nil, err
			}
			result = append(result, toolMessage)
		}

		// Remaining content: messages with media carry a content-part array;
		// text-only messages keep the plain-string content shape.
		if len(parts) > 0 {
			if hasMedia {
				result = append(result, Message{Role: role, ContentParts: parts})
			} else {
				for _, p := range parts {
					result = append(result, Message{Role: role, Content: p.Text})
				}
			}
		}
	}
	return result, nil
}

// joinTextParts flattens text content parts into a single string for message
// shapes that only accept plain-string content.
func joinTextParts(parts []ContentPart) string {
	var texts []string
	for _, p := range parts {
		if p.Type == "text" && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n\n")
}

// encodeImageContentPart converts an ImageContent block to an image_url
// content part. Base64 sources are inlined as data URLs; URL sources pass
// through. The Chat Completions API has no file-ID image reference.
func encodeImageContentPart(c *llm.ImageContent) (ContentPart, error) {
	if c.Source == nil {
		return ContentPart{}, fmt.Errorf("image content has nil source")
	}
	switch c.Source.Type {
	case llm.ContentSourceTypeBase64:
		if c.Source.MediaType == "" || c.Source.Data == "" {
			return ContentPart{}, fmt.Errorf("media type and data are required for base64 image content")
		}
		dataURL := fmt.Sprintf("data:%s;base64,%s", c.Source.MediaType, c.Source.Data)
		return ContentPart{Type: "image_url", ImageURL: &ImageURLPart{URL: dataURL}}, nil
	case llm.ContentSourceTypeURL:
		if c.Source.URL == "" {
			return ContentPart{}, fmt.Errorf("URL is required for URL-based image content")
		}
		return ContentPart{Type: "image_url", ImageURL: &ImageURLPart{URL: c.Source.URL}}, nil
	default:
		return ContentPart{}, fmt.Errorf("unsupported image source type for the chat completions API: %s", c.Source.Type)
	}
}

// encodeDocumentContentPart converts a DocumentContent block to a file
// content part (base64 and file-ID sources) or a text part (text sources).
// The Chat Completions API has no URL-based file reference.
func encodeDocumentContentPart(c *llm.DocumentContent) (ContentPart, error) {
	if c.Source == nil {
		return ContentPart{}, fmt.Errorf("document content has nil source")
	}
	switch c.Source.Type {
	case llm.ContentSourceTypeBase64:
		if c.Source.MediaType == "" || c.Source.Data == "" {
			return ContentPart{}, fmt.Errorf("media type and data are required for base64 document content")
		}
		filename := c.Title
		if filename == "" {
			filename = "document"
		}
		dataURL := fmt.Sprintf("data:%s;base64,%s", c.Source.MediaType, c.Source.Data)
		return ContentPart{Type: "file", File: &FilePart{Filename: filename, FileData: dataURL}}, nil
	case llm.ContentSourceTypeFile:
		if c.Source.FileID == "" {
			return ContentPart{}, fmt.Errorf("file ID is required for file-based document content")
		}
		return ContentPart{Type: "file", File: &FilePart{FileID: c.Source.FileID}}, nil
	case llm.ContentSourceTypeText:
		if c.Source.Data == "" {
			return ContentPart{}, fmt.Errorf("data is required for text document content")
		}
		return ContentPart{Type: "text", Text: c.Source.Data}, nil
	case llm.ContentSourceTypeURL:
		return ContentPart{}, fmt.Errorf("url-based document content is not supported by the chat completions API; use a base64 or file source")
	default:
		return ContentPart{}, fmt.Errorf("unsupported document source type: %s", c.Source.Type)
	}
}

func convertToolResultContent(c *llm.ToolResultContent) (Message, error) {
	contentStr, err := toolResultContentString(c)
	if err != nil {
		return Message{}, err
	}
	return Message{
		Role:       "tool",
		Content:    contentStr,
		ToolCallID: c.ToolUseID,
	}, nil
}

func toolResultContentString(c *llm.ToolResultContent) (string, error) {
	if providers.IsEmptyToolResultContent(c.Content) {
		return providers.EmptyToolResultText, nil
	}
	switch content := c.Content.(type) {
	case string:
		return content, nil
	case []*dive.ToolResultContent:
		return toolResultTextBlocks(content), nil
	default:
		var blocks []*dive.ToolResultContent
		if err := c.DecodeContent(&blocks); err == nil && blocks != nil {
			return toolResultTextBlocks(blocks), nil
		}
		return "", fmt.Errorf("unsupported tool result content type")
	}
}

// toolResultTextBlocks flattens tool result content blocks to a single
// string. Chat Completions tool messages are text-only, so non-text blocks
// (e.g. images from an MCP tool) are represented with a placeholder rather
// than being dropped silently, as is a result with no renderable text at all.
func toolResultTextBlocks(content []*dive.ToolResultContent) string {
	var texts []string
	for _, c := range content {
		switch c.Type {
		case dive.ToolResultContentTypeText, "":
			if c.Text != "" {
				texts = append(texts, c.Text)
			}
		default:
			texts = append(texts, fmt.Sprintf("[%s content omitted]", c.Type))
		}
	}
	if len(texts) == 0 {
		return providers.EmptyToolResultText
	}
	return strings.Join(texts, "\n")
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
	reasoningEffort, includeReasoningEffort, err := p.resolveReasoningEffort(req.Model, config)
	if err != nil {
		return err
	}
	if includeReasoningEffort {
		requestedReasoningEffort := config.ReasoningEffort
		var adjusted bool
		reasoningEffort, adjusted = normalizeToolReasoningEffort(req.Model, reasoningEffort, len(tools) > 0)
		if adjusted && config.Logger != nil {
			config.Logger.Warn("reasoning effort is not supported with function tools for this model in Chat Completions; using none",
				"model", req.Model,
				"requested_reasoning_effort", requestedReasoningEffort,
				"effective_reasoning_effort", reasoningEffort)
		}
		req.ReasoningEffort = reasoningEffort
	}
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
