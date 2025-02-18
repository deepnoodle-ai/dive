package anthropic

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

	"github.com/getstingrai/agents/llm"
)

var (
	DefaultModel            = "claude-3-5-sonnet-20241022"
	DefaultMessagesEndpoint = "https://api.anthropic.com/v1/messages"
	DefaultAnthropicVersion = "2023-06-01"
	DefaultMaxTokens        = 4000
)

var _ llm.LLM = &Provider{}

type Provider struct {
	apiKey           string
	anthropicVersion string
	messagesEndpoint string
	maxTokens        int
	client           *http.Client
}

func New() *Provider {
	return &Provider{
		apiKey:           os.Getenv("ANTHROPIC_API_KEY"),
		anthropicVersion: DefaultAnthropicVersion,
		messagesEndpoint: DefaultMessagesEndpoint,
		maxTokens:        DefaultMaxTokens,
		client:           http.DefaultClient,
	}
}

func (p *Provider) WithMaxTokens(maxTokens int) *Provider {
	p.maxTokens = maxTokens
	return p
}

func (p *Provider) WithMessagesEndpoint(messagesEndpoint string) *Provider {
	p.messagesEndpoint = messagesEndpoint
	return p
}

func (p *Provider) WithClient(client *http.Client) *Provider {
	p.client = client
	return p
}

func (p *Provider) WithAPIKey(apiKey string) *Provider {
	p.apiKey = apiKey
	return p
}

func (p *Provider) WithAnthropicVersion(anthropicVersion string) *Provider {
	p.anthropicVersion = anthropicVersion
	return p
}

func (p *Provider) Generate(ctx context.Context, messages []*llm.Message, opts ...llm.GenerateOption) (*llm.Response, error) {
	config := &llm.GenerateConfig{}
	for _, opt := range opts {
		opt(config)
	}

	model := config.Model
	if model == "" {
		model = DefaultModel
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
		System:      config.SystemPrompt,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.messagesEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", p.anthropicVersion)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error from anthropic api (status %d): %s", resp.StatusCode, string(body))
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from anthropic api")
	}

	var contentBlocks []llm.Content
	for _, block := range result.Content {
		contentBlocks = append(contentBlocks, llm.Content{
			Type: llm.ContentTypeText,
			Text: block.Text,
		})
	}

	return llm.NewResponse(llm.ResponseOptions{
		ID:    result.ID,
		Model: model,
		Role:  llm.Assistant,
		Usage: llm.Usage{
			InputTokens:  result.Usage.InputTokens,
			OutputTokens: result.Usage.OutputTokens,
		},
		Message: &llm.Message{
			Role:    llm.Assistant,
			Content: contentBlocks,
		},
	}), nil
}

func (p *Provider) Stream(ctx context.Context, messages []*llm.Message, opts ...llm.GenerateOption) (llm.Stream, error) {
	config := &llm.GenerateConfig{}
	for _, opt := range opts {
		opt(config)
	}

	model := config.Model
	if model == "" {
		model = DefaultModel
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
		System:      config.SystemPrompt,
		Stream:      true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.messagesEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", p.anthropicVersion)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "text/event-stream")

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
		var content []ContentBlock

		for _, c := range msg.Content {
			switch c.Type {
			case llm.ContentTypeText:
				content = append(content, ContentBlock{
					Type: "text",
					Text: c.Text,
				})
			case llm.ContentTypeImage:
				content = append(content, ContentBlock{
					Type: "image",
					Source: &ImageSource{
						Type:      "base64",
						MediaType: c.MediaType,
						Data:      c.Data,
					},
				})
			default:
				return nil, fmt.Errorf("unsupported content type: %s", c.Type)
			}
		}

		result = append(result, Message{
			Role:    strings.ToLower(string(msg.Role)),
			Content: content,
		})
	}

	return result, nil
}

// Stream implements the llm.Stream interface for Anthropic streaming responses
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

		var event StreamEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue // Skip malformed events
		}

		switch event.Type {
		case "message_start":
			continue // Skip message start events
		case "content_block_start":
			continue // Skip content block start events
		case "content_block_delta":
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				return &llm.StreamEvent{
					Type: llm.EventContentBlockDelta,
					Delta: &llm.Delta{
						Type: "text_delta",
						Text: event.Delta.Text,
					},
				}, true
			}
		case "message_delta":
			if event.Delta.StopReason != "" {
				return nil, false
			}
		}
	}
}

func (s *Stream) Close() error {
	return s.body.Close()
}

func (s *Stream) Err() error {
	return nil
}
