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
	"github.com/getstingrai/agents/providers"
	"github.com/getstingrai/agents/retry"
)

var (
	DefaultModel     = "claude-3-5-sonnet-20241022"
	DefaultEndpoint  = "https://api.anthropic.com/v1/messages"
	DefaultVersion   = "2023-06-01"
	DefaultMaxTokens = 4096
)

var _ llm.LLM = &Provider{}

type Provider struct {
	apiKey    string
	client    *http.Client
	endpoint  string
	version   string
	maxTokens int
	caching   bool
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:    os.Getenv("ANTHROPIC_API_KEY"),
		client:    http.DefaultClient,
		endpoint:  DefaultEndpoint,
		version:   DefaultVersion,
		maxTokens: DefaultMaxTokens,
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
		model = DefaultModel
	}

	maxTokens := config.MaxTokens
	if maxTokens == nil {
		maxTokens = &p.maxTokens
	}

	msgs, err := convertMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}

	if config.CacheControl != "" && len(msgs) > 0 {
		lastMessage := msgs[len(msgs)-1]
		if len(lastMessage.Content) > 0 {
			lastContent := lastMessage.Content[len(lastMessage.Content)-1]
			lastContent.SetCacheControl(config.CacheControl)
		}
	}

	reqBody := Request{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: config.Temperature,
		System:      config.SystemPrompt,
	}

	if len(config.Tools) > 0 {
		var tools []*Tool
		for _, tool := range config.Tools {
			toolDef := tool.Definition()
			tools = append(tools, &Tool{
				Name:        toolDef.Name,
				Description: toolDef.Description,
				InputSchema: toolDef.Parameters,
			})
		}
		reqBody.Tools = tools
	}
	if config.ToolChoice.Type != "" {
		reqBody.ToolChoice = &config.ToolChoice
	}

	jsonBody, err := json.MarshalIndent(reqBody, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	var result Response
	err = retry.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(jsonBody))
		if err != nil {
			return fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Set("x-api-key", p.apiKey)
		req.Header.Set("anthropic-version", p.version)
		req.Header.Set("content-type", "application/json")
		req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("error making request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return providers.NewError(resp.StatusCode, string(body))
		}
		var result Response
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("error decoding response: %w", err)
		}
		return nil
	}, retry.WithMaxRetries(5))
	if err != nil {
		return nil, err
	}

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from anthropic api")
	}

	var contentBlocks []*llm.Content
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			contentBlocks = append(contentBlocks, &llm.Content{
				Type: llm.ContentTypeText,
				Text: block.Text,
			})
		case "tool_use":
			contentBlocks = append(contentBlocks, &llm.Content{
				Type:  llm.ContentTypeToolUse,
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}

	var toolCalls []llm.ToolCall
	for _, content := range contentBlocks {
		if content.Type == llm.ContentTypeToolUse {
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:    content.ID, // e.g. "toolu_01A09q90qw90lq917835lq9"
				Name:  content.Name,
				Input: string(content.Input),
			})
		}
	}

	response := llm.NewResponse(llm.ResponseOptions{
		ID:         result.ID,
		Model:      model,
		Role:       llm.Assistant,
		StopReason: result.StopReason,
		Usage: llm.Usage{
			InputTokens:              result.Usage.InputTokens,
			OutputTokens:             result.Usage.OutputTokens,
			CacheCreationInputTokens: result.Usage.CacheCreationInputTokens,
			CacheReadInputTokens:     result.Usage.CacheReadInputTokens,
		},
		Message: &llm.Message{
			Role:    llm.Assistant,
			Content: contentBlocks,
		},
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
		model = DefaultModel
	}

	maxTokens := config.MaxTokens
	if maxTokens == nil {
		maxTokens = &p.maxTokens
	}

	msgs, err := convertMessages(messages)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
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

	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", p.version)
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

func convertMessages(messages []*llm.Message) ([]*Message, error) {
	var result []*Message
	for _, msg := range messages {
		var blocks []*ContentBlock
		for _, c := range msg.Content {
			switch c.Type {
			case llm.ContentTypeText:
				blocks = append(blocks, &ContentBlock{
					Type: "text",
					Text: c.Text,
				})
			case llm.ContentTypeImage:
				blocks = append(blocks, &ContentBlock{
					Type: "image",
					Source: &ImageSource{
						Type:      "base64",
						MediaType: c.MediaType,
						Data:      c.Data,
					},
				})
			case llm.ContentTypeToolUse:
				blocks = append(blocks, &ContentBlock{
					Type:  "tool_use",
					ID:    c.ID,
					Name:  c.Name,
					Input: json.RawMessage(c.Input),
				})
			case llm.ContentTypeToolResult:
				blocks = append(blocks, &ContentBlock{
					Type:      "tool_result",
					ToolUseID: c.ToolUseID,
					Content:   c.Text, // oddly we have to rename to "content"
				})
			default:
				return nil, fmt.Errorf("unsupported content type: %s", c.Type)
			}
		}
		result = append(result, &Message{
			Role:    strings.ToLower(string(msg.Role)),
			Content: blocks,
		})
	}
	return result, nil
}

// Stream implements the llm.Stream interface for Anthropic streaming responses
type Stream struct {
	reader         *bufio.Reader
	body           io.ReadCloser
	err            error
	contentBlocks  map[int]*ContentBlockAccumulator
	currentMessage *StreamEvent
}

type ContentBlockAccumulator struct {
	Type        string
	Text        string
	PartialJSON string
	ToolUse     *ToolUse
	IsComplete  bool
}

type ToolUse struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type Delta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

func (s *Stream) Next(ctx context.Context) (*llm.StreamEvent, bool) {
	if s.contentBlocks == nil {
		s.contentBlocks = make(map[int]*ContentBlockAccumulator)
	}
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

		// Parse the event type from the SSE format
		if bytes.HasPrefix(line, []byte("event: ")) {
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
			s.currentMessage = &event

		case "content_block_start":
			s.contentBlocks[event.Index] = &ContentBlockAccumulator{
				Type: event.ContentBlock.Type,
				Text: event.ContentBlock.Text,
				// ToolUse: event.ContentBlock.ToolUse,
			}

		case "content_block_stop":
			if block, exists := s.contentBlocks[event.Index]; exists {
				block.IsComplete = true
			}

		case "content_block_delta":
			block, exists := s.contentBlocks[event.Index]
			if !exists {
				block = &ContentBlockAccumulator{
					Type: event.Delta.Type,
				}
				s.contentBlocks[event.Index] = block
			}

			switch event.Delta.Type {
			case "text_delta":
				if event.Delta.Text != "" {
					block.Text += event.Delta.Text
					return &llm.StreamEvent{
						Type:  llm.EventContentBlockDelta,
						Index: event.Index,
						Delta: &llm.Delta{
							Type: "text_delta",
							Text: event.Delta.Text,
						},
						AccumulatedText: block.Text,
					}, true
				}

			case "input_json_delta":
				if event.Delta.PartialJSON != "" {
					block.PartialJSON += event.Delta.PartialJSON
					return &llm.StreamEvent{
						Type:  llm.EventContentBlockDelta,
						Index: event.Index,
						Delta: &llm.Delta{
							Type:        "input_json_delta",
							PartialJSON: event.Delta.PartialJSON,
						},
						AccumulatedJSON: block.PartialJSON,
					}, true
				}
			}

		case "message_delta":
			if event.Delta.StopReason != "" {
				return &llm.StreamEvent{
					Type:  llm.EventMessageDelta,
					Index: event.Index,
					Delta: &llm.Delta{
						Type:         "message_delta",
						StopReason:   event.Delta.StopReason,
						StopSequence: event.Delta.StopSequence,
					},
					AccumulatedText: event.Delta.Text,
					AccumulatedJSON: event.Delta.PartialJSON,
				}, true
			}

		case "message_stop", "ping":
			continue
		}
	}
}

func (s *Stream) Close() error {
	return s.body.Close()
}

func (s *Stream) Err() error {
	return nil
}
