package ollama

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
	DefaultModel     = "llama3.2"
	DefaultEndpoint  = "http://localhost:11434/v1/chat/completions"
	DefaultMaxTokens = 4096
)

var _ llm.StreamingLLM = &Provider{}

type Provider struct {
	apiKey    string
	client    *http.Client
	endpoint  string
	model     string
	maxTokens int
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:   getAPIKey(),
		endpoint: DefaultEndpoint,
		client:   http.DefaultClient,
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

func getAPIKey() string {
	if key := os.Getenv("OLLAMA_API_KEY"); key != "" {
		return key
	}
	// Ollama doesn't require an API key for local instances, but OpenAI-compatible API expects one
	return "ollama"
}

func (p *Provider) Name() string {
	return fmt.Sprintf("ollama-%s", p.model)
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
		return nil, fmt.Errorf("error converting messages: %w", err)
	}
	if config.Prefill != "" {
		msgs = append(msgs, Message{Role: "assistant", Content: config.Prefill})
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
	}, retry.WithMaxRetries(6))

	if err != nil {
		return nil, err
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("empty response from ollama api")
	}
	choice := result.Choices[0]

	var contentBlocks []llm.Content
	if choice.Message.Content != "" {
		contentBlocks = append(contentBlocks, &llm.TextContent{Text: choice.Message.Content})
	}

	// Transform tool calls into content blocks (like OpenAI)
	if len(choice.Message.ToolCalls) > 0 {
		for _, toolCall := range choice.Message.ToolCalls {
			contentBlocks = append(contentBlocks, &llm.ToolUseContent{
				ID:    toolCall.ID,
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
		msgs = append(msgs, Message{Role: "assistant", Content: config.Prefill})
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

	req, err := p.createRequest(ctx, body, config, true)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, providers.NewError(resp.StatusCode, string(body))
	}

	return &StreamIterator{
		reader:            bufio.NewReader(resp.Body),
		body:              resp.Body,
		toolCalls:         make(map[int]*ToolCallAccumulator),
		contentBlocks:     make(map[int]*ContentBlockAccumulator),
		prefill:           config.Prefill,
		prefillClosingTag: config.PrefillClosingTag,
	}, nil
}

func (p *Provider) SupportsStreaming() bool {
	return true
}

func convertMessages(messages []*llm.Message) ([]Message, error) {
	var result []Message
	for _, msg := range messages {
		converted := Message{
			Role: string(msg.Role),
		}

		var contentParts []string
		for _, content := range msg.Content {
			switch c := content.(type) {
			case *llm.TextContent:
				contentParts = append(contentParts, c.Text)
			case *llm.ImageContent:
				// Ollama supports images in OpenAI-compatible format
				// For now, we'll skip images or could add support later
				continue
			case *llm.ToolUseContent:
				// Add tool calls
				if converted.ToolCalls == nil {
					converted.ToolCalls = []ToolCall{}
				}
				converted.ToolCalls = append(converted.ToolCalls, ToolCall{
					ID:   c.ID,
					Type: "function",
					Function: Function{
						Name:      c.Name,
						Arguments: string(c.Input),
					},
				})
			case *llm.ToolResultContent:
				// Tool results are typically handled as separate messages
				if c.Content != nil {
					if str, ok := c.Content.(string); ok {
						contentParts = append(contentParts, str)
					}
				}
			}
		}

		if len(contentParts) > 0 {
			converted.Content = strings.Join(contentParts, "\n")
		}

		result = append(result, converted)
	}
	return result, nil
}

func (p *Provider) applyRequestConfig(req *Request, config *llm.Config) error {
	req.Model = p.model
	req.MaxTokens = &p.maxTokens

	if config.Temperature != nil {
		req.Temperature = config.Temperature
	}

	if config.SystemPrompt != "" {
		// Add system message at the beginning
		req.Messages = append([]Message{{
			Role:    "system",
			Content: config.SystemPrompt,
		}}, req.Messages...)
	}

	if len(config.Tools) > 0 {
		tools := make([]Tool, len(config.Tools))
		for i, tool := range config.Tools {
			tools[i] = Tool{
				Type: "function",
				Function: Function{
					Name:        tool.Name(),
					Description: tool.Description(),
					Parameters:  tool.Schema(),
				},
			}
		}
		req.Tools = tools
	}

	return nil
}

type StreamIterator struct {
	reader            *bufio.Reader
	body              io.ReadCloser
	err               error
	currentEvent      *llm.Event
	toolCalls         map[int]*ToolCallAccumulator
	contentBlocks     map[int]*ContentBlockAccumulator
	responseID        string
	responseModel     string
	usage             Usage
	prefill           string
	prefillClosingTag string
	eventCount        int
	closeOnce         sync.Once
	eventQueue        []*llm.Event
}

type ToolCallAccumulator struct {
	ID         string
	Type       string
	Name       string
	Arguments  string
	IsComplete bool
}

type ContentBlockAccumulator struct {
	Type       string
	Text       string
	IsComplete bool
}

func (s *StreamIterator) Next() bool {
	if s.err != nil {
		return false
	}

	events, err := s.next()
	if err != nil {
		s.err = err
		return false
	}

	if len(events) == 0 {
		return false
	}

	s.eventQueue = append(s.eventQueue, events...)
	if len(s.eventQueue) > 0 {
		s.currentEvent = s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return true
	}

	return false
}

func (s *StreamIterator) Event() *llm.Event {
	return s.currentEvent
}

func (s *StreamIterator) next() ([]*llm.Event, error) {
	line, err := s.reader.ReadBytes('\n')
	if err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, fmt.Errorf("error reading stream: %w", err)
	}

	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return s.next()
	}

	if !bytes.HasPrefix(line, []byte("data: ")) {
		return s.next()
	}

	data := bytes.TrimPrefix(line, []byte("data: "))
	if bytes.Equal(data, []byte("[DONE]")) {
		return nil, nil
	}

	var chunk StreamResponse
	if err := json.Unmarshal(data, &chunk); err != nil {
		return s.next()
	}

	var events []*llm.Event

	if chunk.ID != "" {
		s.responseID = chunk.ID
	}
	if chunk.Model != "" {
		s.responseModel = chunk.Model
	}

	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			// Handle text content
			blockIndex := 0
			if s.contentBlocks[blockIndex] == nil {
				s.contentBlocks[blockIndex] = &ContentBlockAccumulator{
					Type: "text",
				}

				// Send content block start event for the first text content
				index := blockIndex
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &index,
					ContentBlock: &llm.EventContentBlock{
						Type: llm.ContentTypeText,
					},
				})
			}

			block := s.contentBlocks[blockIndex]
			block.Text += choice.Delta.Content

			// Handle prefill for the first content
			content := choice.Delta.Content
			if s.eventCount == 0 && s.prefill != "" {
				content = s.prefill + content
			}

			index := blockIndex
			events = append(events, &llm.Event{
				Type:  llm.EventTypeContentBlockDelta,
				Index: &index,
				Delta: &llm.EventDelta{
					Type: llm.EventDeltaTypeText,
					Text: content,
				},
			})
		}

		if len(choice.Delta.ToolCalls) > 0 {
			for _, toolCall := range choice.Delta.ToolCalls {
				// Note: Ollama's streaming format may not include Index field
				// We'll use a simple counter for now
				index := len(s.toolCalls)
				if s.toolCalls[index] == nil {
					s.toolCalls[index] = &ToolCallAccumulator{
						ID:   toolCall.ID,
						Type: toolCall.Type,
					}

					// Send content block start event for tool use
					events = append(events, &llm.Event{
						Type:  llm.EventTypeContentBlockStart,
						Index: &index,
						ContentBlock: &llm.EventContentBlock{
							Type: llm.ContentTypeToolUse,
							ID:   toolCall.ID,
						},
					})
				}

				acc := s.toolCalls[index]
				if toolCall.Function.Name != "" {
					acc.Name = toolCall.Function.Name
				}
				if toolCall.Function.Arguments != "" {
					acc.Arguments += toolCall.Function.Arguments

					// Send input JSON delta event
					events = append(events, &llm.Event{
						Type:  llm.EventTypeContentBlockDelta,
						Index: &index,
						Delta: &llm.EventDelta{
							Type:        llm.EventDeltaTypeInputJSON,
							PartialJSON: toolCall.Function.Arguments,
						},
					})
				}
			}
		}

		if choice.FinishReason != "" {
			// Send content block stop events for any active blocks
			for index := range s.contentBlocks {
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStop,
					Index: &index,
				})
			}
			for index := range s.toolCalls {
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStop,
					Index: &index,
				})
			}

			// Stream is complete - send message stop event
			events = append(events, &llm.Event{
				Type: llm.EventTypeMessageStop,
			})
		}
	}

	s.eventCount++
	return events, nil
}

func (s *StreamIterator) Close() error {
	s.closeOnce.Do(func() {
		if s.body != nil {
			s.body.Close()
		}
	})
	return nil
}

func (s *StreamIterator) Err() error {
	return s.err
}

func (p *Provider) createRequest(ctx context.Context, body []byte, config *llm.Config, isStreaming bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	if isStreaming {
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
	}

	return req, nil
}
