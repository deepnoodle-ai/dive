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
	"sync"

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

var _ llm.StreamingLLM = &Provider{}

type Provider struct {
	apiKey     string
	endpoint   string
	model      string
	systemRole string
	corePrompt string
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

func (p *Provider) Name() string {
	return fmt.Sprintf("openai-%s", p.model)
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

	if config.Prefill != "" {
		msgs = append(msgs, Message{Role: "assistant", Content: config.Prefill})
	}

	reqBody := Request{
		Model:            model,
		Messages:         msgs,
		MaxTokens:        maxTokens,
		Temperature:      config.Temperature,
		Tools:            tools,
		ToolChoice:       toolChoice,
		PresencePenalty:  config.PresencePenalty,
		FrequencyPenalty: config.FrequencyPenalty,
		ReasoningFormat:  config.ReasoningFormat,
		ReasoningEffort:  ReasoningEffort(config.ReasoningEffort),
	}

	if systemPrompt := p.GetSystemPrompt(config); systemPrompt != "" {
		reqBody.Messages = append([]Message{{
			Role:    p.systemRole,
			Content: systemPrompt,
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

	if config.Prefill != "" {
		for _, block := range contentBlocks {
			if block.Type == llm.ContentTypeText {
				if config.PrefillClosingTag == "" ||
					strings.Contains(block.Text, config.PrefillClosingTag) {
					block.Text = config.Prefill + block.Text
				}
				break
			}
		}
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

// Stream implements the llm.Stream interface for OpenAI streaming responses.
// It supports both text responses and tool calls.
//
// For tool calls, the implementation accumulates the tool call information
// as it arrives in chunks and builds a final response when the stream ends.
// This is necessary because tool calls can be split across multiple chunks.
//
// The implementation is based on the OpenAI API documentation:
// https://platform.openai.com/docs/api-reference/chat/create
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

	if config.Prefill != "" {
		msgs = append(msgs, Message{Role: "assistant", Content: config.Prefill})
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
		Model:            model,
		Messages:         msgs,
		MaxTokens:        maxTokens,
		Temperature:      config.Temperature,
		Stream:           true,
		StreamOptions:    &StreamOptions{IncludeUsage: true},
		Tools:            tools,
		ToolChoice:       toolChoice,
		PresencePenalty:  config.PresencePenalty,
		FrequencyPenalty: config.FrequencyPenalty,
		ReasoningFormat:  config.ReasoningFormat,
		ReasoningEffort:  ReasoningEffort(config.ReasoningEffort),
	}

	if systemPrompt := p.GetSystemPrompt(config); systemPrompt != "" {
		reqBody.Messages = append([]Message{{
			Role:    p.systemRole,
			Content: systemPrompt,
		}}, reqBody.Messages...)
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
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")

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
			toolCalls:         map[int]*ToolCallAccumulator{},
			contentBlocks:     map[int]*ContentBlockAccumulator{},
			prefill:           config.Prefill,
			prefillClosingTag: config.PrefillClosingTag,
			closeOnce:         sync.Once{},
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

func (p *Provider) GetSystemPrompt(c *llm.Config) string {
	var parts []string
	if p.corePrompt != "" {
		parts = append(parts, p.corePrompt)
	}
	if c.SystemPrompt != "" {
		parts = append(parts, c.SystemPrompt)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
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
				Content:   textContent,
				ToolCalls: toolCalls,
			})
		}

		// Process non-tool-use content blocks
		if !hasToolUse || hasToolResult {
			for _, c := range msg.Content {
				switch c.Type {
				case llm.ContentTypeText:
					if !hasToolUse {
						// Only add text content if not already added with tool calls
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

// Next advances to the next event in the stream. Returns false when the stream
// is complete or an error occurs.
func (s *StreamIterator) Next() bool {
	// If we have events in the queue, use the first one
	if len(s.eventQueue) > 0 {
		s.currentEvent = s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return true
	}

	// Try to get more events
	for {
		events, err := s.next()
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

// next processes a single line from the stream and returns events if any are ready
func (s *StreamIterator) next() ([]*llm.Event, error) {
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

	var event StreamResponse
	if err := json.Unmarshal(line, &event); err != nil {
		return nil, err
	}
	if event.ID != "" {
		s.responseID = event.ID
	}
	if event.Model != "" {
		s.responseModel = event.Model
	}
	if event.Usage.TotalTokens > 0 {
		s.usage = event.Usage
	}
	if len(event.Choices) == 0 {
		return nil, nil
	}
	choice := event.Choices[0]
	var events []*llm.Event

	// Emit message start event if this is the first chunk
	if s.responseID != "" && s.currentEvent == nil {
		events = append(events, &llm.Event{
			Type: llm.EventMessageStart,
			Message: &llm.Message{
				ID:      s.responseID,
				Role:    llm.Assistant,
				Content: []*llm.Content{},
			},
			Usage: &llm.Usage{
				InputTokens:  s.usage.PromptTokens,
				OutputTokens: s.usage.CompletionTokens,
			},
		})
	}

	if choice.Delta.Reasoning != "" {
		// TODO: handle accumulated reasoning
	}

	// Handle text content
	if choice.Delta.Content != "" {
		// Apply and clear prefill if there is one
		if s.prefill != "" {
			if !strings.HasPrefix(choice.Delta.Content, s.prefill) &&
				!strings.HasPrefix(s.prefill, choice.Delta.Content) {
				choice.Delta.Content = s.prefill + choice.Delta.Content
			}
			s.prefill = ""
		}
		index := choice.Index
		if _, exists := s.contentBlocks[index]; !exists {
			s.contentBlocks[index] = &ContentBlockAccumulator{Type: "text", Text: ""}
			events = append(events, &llm.Event{
				Type:         llm.EventContentBlockStart,
				Index:        index,
				ContentBlock: &llm.ContentBlock{Type: "text", Text: ""},
			})
		}
		// Accumulate the text
		block := s.contentBlocks[index]
		block.Text += choice.Delta.Content
		events = append(events, &llm.Event{
			Type:  llm.EventContentBlockDelta,
			Index: index,
			Delta: &llm.Delta{
				Type: "text_delta",
				Text: choice.Delta.Content,
			},
		})
	}

	if len(choice.Delta.ToolCalls) > 0 {
		for _, toolCallDelta := range choice.Delta.ToolCalls {
			index := toolCallDelta.Index
			if _, exists := s.toolCalls[index]; !exists {
				s.toolCalls[index] = &ToolCallAccumulator{Type: "function"}
				events = append(events, &llm.Event{
					Type:  llm.EventContentBlockStart,
					Index: index,
					ContentBlock: &llm.ContentBlock{
						ID:   toolCallDelta.ID,
						Name: toolCallDelta.Function.Name,
						Type: "tool_use",
					},
				})
			}
			toolCall := s.toolCalls[index]
			if toolCallDelta.ID != "" {
				toolCall.ID = toolCallDelta.ID
				// Update the ContentBlock in the event queue if it exists
				for _, queuedEvent := range s.eventQueue {
					if queuedEvent.Type == llm.EventContentBlockStart && queuedEvent.Index == index {
						if queuedEvent.ContentBlock == nil {
							queuedEvent.ContentBlock = &llm.ContentBlock{Type: "tool_use"}
						}
						queuedEvent.ContentBlock.ID = toolCallDelta.ID
					}
				}
			}
			if toolCallDelta.Type != "" {
				toolCall.Type = toolCallDelta.Type
			}
			if toolCallDelta.Function.Name != "" {
				toolCall.Name = toolCallDelta.Function.Name
				// Update the ContentBlock in the event queue if it exists
				for _, queuedEvent := range s.eventQueue {
					if queuedEvent.Type == llm.EventContentBlockStart && queuedEvent.Index == index {
						if queuedEvent.ContentBlock == nil {
							queuedEvent.ContentBlock = &llm.ContentBlock{Type: "tool_use"}
						}
						queuedEvent.ContentBlock.Name = toolCallDelta.Function.Name
					}
				}
			}
			if toolCallDelta.Function.Arguments != "" {
				toolCall.Arguments += toolCallDelta.Function.Arguments
				events = append(events, &llm.Event{
					Type:  llm.EventContentBlockDelta,
					Index: index,
					Delta: &llm.Delta{
						Type:        "input_json_delta",
						PartialJSON: toolCallDelta.Function.Arguments,
					},
				})
			}
		}
	}

	if choice.FinishReason != "" {
		// Add message_delta event with stop reason
		events = append(events, &llm.Event{
			Type:     llm.EventMessageDelta,
			Response: s.buildFinalResponse(choice.FinishReason),
			Delta: &llm.Delta{
				Type:       "message_delta",
				StopReason: choice.FinishReason,
			},
		})
	}

	return events, nil
}

// buildFinalResponse creates a final response with all accumulated tool calls
// and content blocks. It converts the accumulated data to the format expected
// by the llm package. This is called when the stream ends or a finish reason
// is received.
func (s *StreamIterator) buildFinalResponse(stopReason string) *llm.Response {
	var toolCalls []llm.ToolCall
	var contentBlocks []*llm.Content
	for _, toolCall := range s.toolCalls {
		if toolCall.Name != "" {
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:    toolCall.ID,
				Name:  toolCall.Name,
				Input: toolCall.Arguments,
			})
			contentBlocks = append(contentBlocks, &llm.Content{
				Type:  llm.ContentTypeToolUse,
				ID:    toolCall.ID,
				Name:  toolCall.Name,
				Input: json.RawMessage(toolCall.Arguments),
			})
		}
	}
	for _, block := range s.contentBlocks {
		if block.Type == "text" {
			contentBlocks = append(contentBlocks, &llm.Content{
				Type: llm.ContentTypeText,
				Text: block.Text,
			})
		}
	}
	return llm.NewResponse(llm.ResponseOptions{
		ID:         s.responseID,
		Model:      s.responseModel,
		Role:       llm.Assistant,
		StopReason: stopReason,
		Message:    llm.NewMessage(llm.Assistant, contentBlocks),
		ToolCalls:  toolCalls,
		Usage: llm.Usage{
			InputTokens:  s.usage.PromptTokens,
			OutputTokens: s.usage.CompletionTokens,
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

type stopBlock struct {
	Index int
	Type  string
}
