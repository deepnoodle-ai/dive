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
	"sync"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers"
	"github.com/diveagents/dive/retry"
)

var (
	DefaultModel     = ModelClaude37Sonnet
	DefaultEndpoint  = "https://api.anthropic.com/v1/messages"
	DefaultVersion   = "2023-06-01"
	DefaultMaxTokens = 4096
)

var _ llm.StreamingLLM = &Provider{}

type Provider struct {
	apiKey    string
	client    *http.Client
	endpoint  string
	model     string
	version   string
	maxTokens int
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:   os.Getenv("ANTHROPIC_API_KEY"),
		client:   http.DefaultClient,
		endpoint: DefaultEndpoint,
		version:  DefaultVersion,
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.model == "" {
		p.model = DefaultModel
	}
	if p.maxTokens == 0 {
		p.maxTokens = DefaultMaxTokens
	}
	return p
}

func (p *Provider) Name() string {
	return fmt.Sprintf("anthropic-%s", p.model)
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

	messageCount := len(messages)
	if messageCount == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	for i, message := range messages {
		if len(message.Content) == 0 {
			return nil, fmt.Errorf("empty message detected (index %d)", i)
		}
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

	if config.Prefill != "" {
		msgs = append(msgs, &Message{
			Role:    "assistant",
			Content: []*ContentBlock{{Type: "text", Text: config.Prefill}},
		})
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
	if config.Prefill != "" {
		addPrefill(result.Content, config.Prefill, config.PrefillClosingTag)
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

func addPrefill(blocks []*ContentBlock, prefill, closingTag string) error {
	if prefill == "" {
		return nil
	}
	for _, block := range blocks {
		if block.Type == "text" {
			if closingTag == "" || strings.Contains(block.Text, closingTag) {
				block.Text = prefill + block.Text
				return nil
			}
			return fmt.Errorf("prefill closing tag not found")
		}
	}
	return fmt.Errorf("no text content found in message")
}

func (p *Provider) Stream(ctx context.Context, messages []*llm.Message, opts ...llm.Option) (llm.StreamIterator, error) {
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

	messageCount := len(messages)
	if messageCount == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	for i, message := range messages {
		if len(message.Content) == 0 {
			return nil, fmt.Errorf("empty message detected (index %d)", i)
		}
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

	if config.Prefill != "" {
		msgs = append(msgs, &Message{
			Role:    "assistant",
			Content: []*ContentBlock{{Type: "text", Text: config.Prefill}},
		})
	}

	reqBody := Request{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: config.Temperature,
		System:      config.SystemPrompt,
		Stream:      true,
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

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	var stream *StreamIterator
	err = retry.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(jsonBody))
		if err != nil {
			return fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Set("x-api-key", p.apiKey)
		req.Header.Set("anthropic-version", p.version)
		req.Header.Set("content-type", "application/json")
		req.Header.Set("accept", "text/event-stream")
		req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

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
			reader:            bufio.NewReader(resp.Body),
			body:              resp.Body,
			contentBlocks:     make(map[int]*ContentBlockAccumulator),
			prefill:           config.Prefill,
			prefillClosingTag: config.PrefillClosingTag,
		}
		return nil
	}, retry.WithMaxRetries(6))

	if err != nil {
		return nil, err
	}
	return stream, nil
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

	reorderMessageContent(result)

	return result, nil
}

func reorderMessageContent(messages []*Message) {
	// Any assistant message with two content blocks where the first is a tool use
	// and the second is a text block should be reordered so the text block is first.
	// See https://github.com/anthropics/claude-code/issues/473#issuecomment-2738842746
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.Content) == 2 {
			if msg.Content[0].Type == "tool_use" && msg.Content[1].Type == "text" {
				msg.Content = []*ContentBlock{msg.Content[1], msg.Content[0]}
			}
		}
	}
}

// StreamIterator implements the llm.StreamIterator interface for Anthropic streaming responses
type StreamIterator struct {
	reader            *bufio.Reader
	body              io.ReadCloser
	err               error
	currentEvent      *llm.Event
	contentBlocks     map[int]*ContentBlockAccumulator
	responseID        string
	responseModel     string
	usage             Usage
	prefill           string
	prefillClosingTag string
	closeOnce         sync.Once
	eventQueue        []*llm.Event
}

type ContentBlockAccumulator struct {
	Type        string
	Text        string
	PartialJSON string
	ToolUse     *ToolUse
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

// Next advances to the next event in the stream.
// Returns false when the stream is complete or an error occurs.
func (s *StreamIterator) Next() bool {
	// If we have events in the queue, use the first one
	if len(s.eventQueue) > 0 {
		s.currentEvent = s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return true
	}

	// Try to get more events
	for {
		events, err := s.readNext()
		if err != nil {
			if err != io.EOF {
				// EOF is expected when stream ends
				s.Close()
				s.err = err
			}
			return false
		}

		// If we got events, use the first one and queue the rest
		if len(events) > 0 {
			s.currentEvent = events[0]
			if len(events) > 1 {
				s.eventQueue = append(s.eventQueue, events[1:]...)
			}
			return true
		}
	}
}

// Event returns the current event. Should only be called after a successful Next().
func (s *StreamIterator) Event() *llm.Event {
	return s.currentEvent
}

// readNext processes a single line from the stream and returns events if any are ready
func (s *StreamIterator) readNext() ([]*llm.Event, error) {
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

	var events []*llm.Event

	switch llm.EventType(event.Type) {

	case llm.EventMessageStart:
		s.responseID = event.Message.ID
		s.responseModel = event.Message.Model
		s.usage = event.Message.Usage
		events = append(events, &llm.Event{
			Type:  llm.EventMessageStart,
			Index: event.Index,
			Message: &llm.Message{
				ID:      event.Message.ID,
				Role:    llm.Assistant,
				Content: []*llm.Content{},
			},
			Usage: &llm.Usage{
				InputTokens:              event.Message.Usage.InputTokens,
				OutputTokens:             event.Message.Usage.OutputTokens,
				CacheCreationInputTokens: event.Message.Usage.CacheCreationInputTokens,
				CacheReadInputTokens:     event.Message.Usage.CacheReadInputTokens,
			},
		})

	case llm.EventMessageStop:
		// noop

	case llm.EventContentBlockStart:
		s.contentBlocks[event.Index] = &ContentBlockAccumulator{
			Type: event.ContentBlock.Type,
			Text: event.ContentBlock.Text,
		}
		if event.ContentBlock.Type == "tool_use" {
			s.contentBlocks[event.Index].ToolUse = &ToolUse{
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
			}
		}
		events = append(events, &llm.Event{
			Type:  llm.EventContentBlockStart,
			Index: event.Index,
			ContentBlock: &llm.ContentBlock{
				ID:   event.ContentBlock.ID,
				Name: event.ContentBlock.Name,
				Type: event.ContentBlock.Type,
				Text: event.ContentBlock.Text,
			},
		})

	case llm.EventContentBlockDelta:
		block := s.contentBlocks[event.Index]
		if block == nil {
			block = &ContentBlockAccumulator{Type: event.Delta.Type}
			s.contentBlocks[event.Index] = block
		}
		if s.prefill != "" {
			event.Delta.Text = s.prefill + event.Delta.Text
			s.prefill = ""
		}
		block.Text += event.Delta.Text
		if event.Delta.Type == "input_json_delta" {
			block.PartialJSON += event.Delta.PartialJSON
		}
		if block.Type == "tool_use" && block.ToolUse == nil {
			block.ToolUse = &ToolUse{}
		}
		events = append(events, &llm.Event{
			Type:  llm.EventContentBlockDelta,
			Index: event.Index,
			Delta: &llm.Delta{
				Type:        event.Delta.Type,
				Text:        event.Delta.Text,
				PartialJSON: event.Delta.PartialJSON,
			},
		})

	case llm.EventContentBlockStop:
		// noop

	case llm.EventMessageDelta:
		s.usage.InputTokens += event.Usage.InputTokens
		s.usage.OutputTokens += event.Usage.OutputTokens
		s.usage.CacheCreationInputTokens += event.Usage.CacheCreationInputTokens
		s.usage.CacheReadInputTokens += event.Usage.CacheReadInputTokens
		events = append(events, &llm.Event{
			Type:  llm.EventMessageDelta,
			Index: event.Index,
			Delta: &llm.Delta{
				Type:         "message_delta",
				StopReason:   event.Delta.StopReason,
				StopSequence: event.Delta.StopSequence,
			},
			Response: s.buildFinalResponse(event.Delta.StopReason),
		})
	}
	return events, nil
}

func (s *StreamIterator) buildFinalResponse(stopReason string) *llm.Response {
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
		ID:         s.responseID,
		Model:      s.responseModel,
		Role:       llm.Assistant,
		StopReason: stopReason,
		ToolCalls:  toolCalls,
		Message:    llm.NewMessage(llm.Assistant, contentBlocks),
		Usage: llm.Usage{
			InputTokens:              s.usage.InputTokens,
			OutputTokens:             s.usage.OutputTokens,
			CacheCreationInputTokens: s.usage.CacheCreationInputTokens,
			CacheReadInputTokens:     s.usage.CacheReadInputTokens,
		},
	})
}

func (s *StreamIterator) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.body.Close() })
	return err
}

func (s *StreamIterator) Err() error {
	return s.err
}
