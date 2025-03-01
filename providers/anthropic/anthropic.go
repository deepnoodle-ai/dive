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

	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/providers"
	"github.com/getstingrai/dive/retry"
)

var (
	DefaultModel     = "claude-3-7-sonnet-20250219"
	DefaultEndpoint  = "https://api.anthropic.com/v1/messages"
	DefaultVersion   = "2023-06-01"
	DefaultMaxTokens = 4096
)

var _ llm.LLM = &Provider{}

type Provider struct {
	apiKey    string
	client    *http.Client
	endpoint  string
	model     string
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
		model:     DefaultModel,
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

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from anthropic api")
	}

	toolCalls, contentBlocks := processContentBlocks(result.Content)

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
		ToolCalls: toolCalls,
		Message:   llm.NewMessage(llm.Assistant, contentBlocks),
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

// processContentBlocks converts Anthropic content blocks to LLM content blocks and tool calls
func processContentBlocks(blocks []*ContentBlock) ([]llm.ToolCall, []*llm.Content) {
	var toolCalls []llm.ToolCall
	var contentBlocks []*llm.Content

	for _, block := range blocks {
		switch block.Type {
		case "text":
			contentBlocks = append(contentBlocks, &llm.Content{
				Type: llm.ContentTypeText,
				Text: block.Text,
			})
		case "tool_use":
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:    block.ID, // e.g. "toolu_01A09q90qw90lq917835lq9"
				Name:  block.Name,
				Input: string(block.Input),
			})
			contentBlocks = append(contentBlocks, &llm.Content{
				Type:  llm.ContentTypeToolUse,
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}

	return toolCalls, contentBlocks
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
		reader:        bufio.NewReader(resp.Body),
		body:          resp.Body,
		contentBlocks: make(map[int]*ContentBlockAccumulator),
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
	usage          Usage
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

// ---- Example response event stream ----

// event: message_start
// data: {"type": "message_start", "message": {"id": "msg_1nZdL29xx5MUA1yADyHTEsnR8uuvGzszyY", "type": "message", "role": "assistant", "content": [], "model": "claude-3-7-sonnet-20250219", "stop_reason": null, "stop_sequence": null, "usage": {"input_tokens": 25, "output_tokens": 1}}}

// event: content_block_start
// data: {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}}

// event: ping
// data: {"type": "ping"}

// event: content_block_delta
// data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "Hello"}}

// event: content_block_delta
// data: {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "!"}}

// event: content_block_stop
// data: {"type": "content_block_stop", "index": 0}

// event: message_delta
// data: {"type": "message_delta", "delta": {"stop_reason": "end_turn", "stop_sequence":null}, "usage": {"output_tokens": 15}}

// event: message_stop
// data: {"type": "message_stop"}

// ---- End example response event stream ----

func (s *Stream) Next(ctx context.Context) (*llm.StreamEvent, bool) {
	for {
		event, err := s.next()
		if err != nil {
			if err != io.EOF {
				// EOF is the expected error when the stream ends
				s.err = err
			}
			return nil, false
		}
		if event != nil {
			return event, true
		}
	}
}

func (s *Stream) next() (*llm.StreamEvent, error) {
	line, err := s.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	// Skip empty lines
	if len(bytes.TrimSpace(line)) == 0 {
		return nil, nil
	}

	// Parse the event type from the SSE format
	if bytes.HasPrefix(line, []byte("event: ")) {
		return nil, nil
	}

	// Remove "data: " prefix if present
	line = bytes.TrimPrefix(line, []byte("data: "))

	// Check for stream end
	if bytes.Equal(bytes.TrimSpace(line), []byte("[DONE]")) {
		return nil, nil
	}

	var event StreamEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return nil, err
	}

	switch event.Type {
	case "message_start":
		s.currentMessage = &event
		s.usage = event.Message.Usage

	case "content_block_start":
		s.contentBlocks[event.Index] = &ContentBlockAccumulator{
			Type: event.ContentBlock.Type,
			Text: event.ContentBlock.Text,
		}

	case "content_block_stop":
		if block, exists := s.contentBlocks[event.Index]; exists {
			block.IsComplete = true
		}

	case "content_block_delta":
		block, exists := s.contentBlocks[event.Index]
		if !exists {
			block = &ContentBlockAccumulator{Type: event.Delta.Type}
			s.contentBlocks[event.Index] = block
		}
		// Accumulate both the text and the partial JSON on the block
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
				}, nil
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
				}, nil
			}
		}

	case "message_delta":
		// Combine initial usage with this updated usage
		usage := s.usage
		usage.InputTokens += event.Usage.InputTokens
		usage.OutputTokens += event.Usage.OutputTokens
		usage.CacheCreationInputTokens += event.Usage.CacheCreationInputTokens
		usage.CacheReadInputTokens += event.Usage.CacheReadInputTokens
		response := s.buildFinalResponse(event.Delta.StopReason, usage)
		return &llm.StreamEvent{
			Type:  llm.EventMessageDelta,
			Index: event.Index,
			Delta: &llm.Delta{
				Type:         "message_delta",
				StopReason:   event.Delta.StopReason,
				StopSequence: event.Delta.StopSequence,
			},
			Response: response,
		}, nil

	case "message_stop":
		return &llm.StreamEvent{Type: llm.EventMessageStop}, nil

	case "ping":
		return &llm.StreamEvent{Type: llm.EventPing}, nil
	}
	return nil, nil
}

func (s *Stream) buildFinalResponse(stopReason string, usage Usage) *llm.Response {
	blocks := make([]*ContentBlock, 0, len(s.contentBlocks))
	for _, block := range s.contentBlocks {
		contentBlock := &ContentBlock{
			Type: block.Type,
			Text: block.Text,
		}
		if block.Type == "tool_use" && block.ToolUse != nil {
			contentBlock.ID = block.ToolUse.ID
			contentBlock.Name = block.ToolUse.Name
			contentBlock.Input = json.RawMessage(block.PartialJSON)
		}
		blocks = append(blocks, contentBlock)
	}
	toolCalls, contentBlocks := processContentBlocks(blocks)

	return llm.NewResponse(llm.ResponseOptions{
		ID:         s.currentMessage.Message.ID,
		Model:      s.currentMessage.Message.Model,
		Role:       llm.Assistant,
		StopReason: stopReason,
		ToolCalls:  toolCalls,
		Message:    llm.NewMessage(llm.Assistant, contentBlocks),
		Usage: llm.Usage{
			InputTokens:              usage.InputTokens,
			OutputTokens:             usage.OutputTokens,
			CacheCreationInputTokens: usage.CacheCreationInputTokens,
			CacheReadInputTokens:     usage.CacheReadInputTokens,
		},
	})
}

func (s *Stream) Close() error {
	return s.body.Close()
}

func (s *Stream) Err() error {
	return s.err
}
