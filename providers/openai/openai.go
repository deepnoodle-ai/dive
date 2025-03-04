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
		Model:       model,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: config.Temperature,
		Stream:      true,
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

	var stream *Stream
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

		stream = &Stream{
			reader:            bufio.NewReader(resp.Body),
			body:              resp.Body,
			toolCalls:         make(map[int]*ToolCallAccumulator),
			contentBlocks:     make(map[int]*ContentBlockAccumulator),
			prefill:           config.Prefill,
			prefillClosingTag: config.PrefillClosingTag,
			messageStartSent:  false,
			eventQueue:        make([]*llm.StreamEvent, 0),
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
	reader            *bufio.Reader
	body              io.ReadCloser
	err               error
	toolCalls         map[int]*ToolCallAccumulator
	contentBlocks     map[int]*ContentBlockAccumulator
	responseID        string
	responseModel     string
	usage             Usage
	prefill           string
	prefillClosingTag string
	closeOnce         sync.Once
	messageStartSent  bool               // Track whether we've sent a message_start event
	eventQueue        []*llm.StreamEvent // Queue to store events that need to be processed
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

// Next returns the next event from the stream.
// It handles both text responses and tool calls.
//
// For text responses, it returns a simple text delta event.
// For tool calls, it accumulates the tool call information as it arrives
// and returns a delta event for each chunk of the tool call.
//
// When the stream ends or a finish reason is received, it builds a final
// response with all accumulated tool calls.
func (s *Stream) Next(ctx context.Context) (*llm.StreamEvent, bool) {
	// If we have events in the queue, return the first one
	if len(s.eventQueue) > 0 {
		event := s.eventQueue[0]
		s.eventQueue = s.eventQueue[1:]
		return event, true
	}

	// Otherwise, try to get more events
	for {
		events, err := s.next()
		if err != nil {
			if err != io.EOF {
				// EOF is the expected error when the stream ends
				s.Close()
				s.err = err
			}
			return nil, false
		}

		// If we got events, add them to the queue and return the first one
		if len(events) > 0 {
			// If there's more than one event, queue the rest
			if len(events) > 1 {
				s.eventQueue = append(s.eventQueue, events[1:]...)
			}
			return events[0], true
		}
	}
}

// next processes a single line from the stream and returns events if any are ready
func (s *Stream) next() ([]*llm.StreamEvent, error) {
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
		// Return final response if we have tool calls or content blocks
		if len(s.toolCalls) > 0 || len(s.contentBlocks) > 0 {
			return []*llm.StreamEvent{
				{
					Type:     llm.EventMessageStop,
					Response: s.buildFinalResponse(""),
				},
			}, nil
		}
		return []*llm.StreamEvent{{Type: llm.EventMessageStop}}, nil
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
	var events []*llm.StreamEvent

	// If this is the first chunk, emit a message_start event
	if !s.messageStartSent && s.responseID != "" {
		s.messageStartSent = true
		events = append(events, &llm.StreamEvent{Type: llm.EventMessageStart})
	}

	// Handle text content
	if choice.Delta.Content != "" {
		index := choice.Index

		// Check if we need to emit a content_block_start event
		if _, exists := s.contentBlocks[index]; !exists {
			s.contentBlocks[index] = &ContentBlockAccumulator{
				Type: "text",
				Text: "",
			}
			events = append(events, &llm.StreamEvent{
				Type:  llm.EventContentBlockStart,
				Index: index,
			})
		}

		// Accumulate the text
		block := s.contentBlocks[index]

		// Apply prefill if needed
		if s.prefill != "" {
			// Inject our prefill and then clear it.
			if !strings.HasPrefix(choice.Delta.Content, s.prefill) &&
				!strings.HasPrefix(s.prefill, choice.Delta.Content) {
				choice.Delta.Content = s.prefill + choice.Delta.Content
			}
			s.prefill = ""
		}

		block.Text += choice.Delta.Content

		// Emit a content_block_delta event
		events = append(events, &llm.StreamEvent{
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
				s.toolCalls[index] = &ToolCallAccumulator{
					Type: "function",
				}
				events = append(events, &llm.StreamEvent{
					Type:  llm.EventContentBlockStart,
					Index: index,
					ContentBlock: &llm.ContentBlock{
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
				events = append(events, &llm.StreamEvent{
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

	// Handle finish reason
	if choice.FinishReason != "" {
		// Create a list of blocks that need stop events
		var blocksToStop []struct {
			Index int
			Type  string
		}

		// Add tool calls that need to be stopped
		for index, toolCall := range s.toolCalls {
			if !toolCall.IsComplete {
				blocksToStop = append(blocksToStop, struct {
					Index int
					Type  string
				}{Index: index, Type: "tool"})
				toolCall.IsComplete = true
			}
		}

		// Add content blocks that need to be stopped
		for index, block := range s.contentBlocks {
			if !block.IsComplete {
				blocksToStop = append(blocksToStop, struct {
					Index int
					Type  string
				}{Index: index, Type: "content"})
				block.IsComplete = true
			}
		}

		// Add stop events for all blocks that need to be stopped
		for _, block := range blocksToStop {
			event := &llm.StreamEvent{
				Type:  llm.EventContentBlockStop,
				Index: block.Index,
			}

			// Add ContentBlock with ID and Name for tool calls
			if block.Type == "tool" {
				toolCall := s.toolCalls[block.Index]
				event.ContentBlock = &llm.ContentBlock{
					ID:   toolCall.ID,
					Name: toolCall.Name,
					Type: "tool_use",
				}
			} else if block.Type == "content" {
				contentBlock := s.contentBlocks[block.Index]
				event.ContentBlock = &llm.ContentBlock{
					Type: contentBlock.Type,
					Text: contentBlock.Text,
				}
			}

			events = append(events, event)
		}

		// Add message_delta event with stop reason
		events = append(events, &llm.StreamEvent{
			Type: llm.EventMessageDelta,
			Delta: &llm.Delta{
				Type:       "message_delta",
				StopReason: choice.FinishReason,
			},
			Response: s.buildFinalResponse(choice.FinishReason),
		})
	}

	return events, nil
}

// buildFinalResponse creates a final response with all accumulated tool calls and content blocks.
// It converts the accumulated data to the format expected by the llm package.
// This is called when the stream ends or a finish reason is received.
func (s *Stream) buildFinalResponse(stopReason string) *llm.Response {
	var toolCalls []llm.ToolCall
	var contentBlocks []*llm.Content

	// Convert accumulated tool calls to response format
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

	// Convert accumulated content blocks to response format
	for _, block := range s.contentBlocks {
		if block.Type == "text" {
			contentBlocks = append(contentBlocks, &llm.Content{
				Type: llm.ContentTypeText,
				Text: block.Text,
			})
		}
	}

	// Ensure we have at least some token usage information
	// OpenAI doesn't always include usage in streaming responses
	inputTokens := s.usage.PromptTokens
	if inputTokens == 0 && s.usage.TotalTokens > 0 {
		// If we have total tokens but no prompt tokens, estimate
		inputTokens = s.usage.TotalTokens - s.usage.CompletionTokens
	}

	return llm.NewResponse(llm.ResponseOptions{
		ID:         s.responseID,
		Model:      s.responseModel,
		Role:       llm.Assistant,
		StopReason: stopReason,
		Usage: llm.Usage{
			InputTokens:  inputTokens,
			OutputTokens: s.usage.CompletionTokens,
		},
		Message:   llm.NewMessage(llm.Assistant, contentBlocks),
		ToolCalls: toolCalls,
	})
}

func (s *Stream) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.body.Close() })
	return err
}

func (s *Stream) Err() error {
	return s.err
}
