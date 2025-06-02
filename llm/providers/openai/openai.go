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

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers"
	"github.com/diveagents/dive/retry"
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
	DefaultModel              = "gpt-4o"
	DefaultMessagesEndpoint   = "https://api.openai.com/v1/chat/completions"
	DefaultMaxTokens          = 4096
	DefaultSystemRole         = "developer"
	ModelSystemPromptBehavior = map[string]SystemPromptBehavior{
		"o1-mini": SystemPromptBehaviorOmit,
	}
	ModelToolBehavior = map[string]ToolBehavior{
		"o1-mini": ToolBehaviorOmit,
	}
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
		client:     http.DefaultClient,
		systemRole: DefaultSystemRole,
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
	return fmt.Sprintf("openai-%s", p.model)
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
	addSystemPrompt(&request, p.GetSystemPrompt(config), p.systemRole)

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
		return nil, fmt.Errorf("empty response from openai api")
	}
	choice := result.Choices[0]

	var contentBlocks []llm.Content
	if choice.Message.Content != "" {
		contentBlocks = append(contentBlocks, &llm.TextContent{Text: choice.Message.Content.(string)})
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
	addSystemPrompt(&request, p.GetSystemPrompt(config), p.systemRole)

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
	err = retry.Do(ctx, func() error {
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
		var hasFile bool
		var fileContent []map[string]any

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
			case *llm.DocumentContent:
				fileMap := map[string]interface{}{
					"file_data": fmt.Sprintf("data:%s;base64,%s", c.Source.MediaType, c.Source.Data),
					"filename":  c.Title,
				}
				fileContent = append(fileContent, map[string]any{
					"type": "file",
					"file": fileMap,
				})
				hasFile = true
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
				case *llm.DocumentContent:
					if hasFile {
						result = append(result, Message{
							Role:    role,
							Content: fileContent,
						})
					}
				case *llm.ToolResultContent:
					// Each tool result goes in its own message
					contentStr, _ := c.Content.(string)
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
		if strings.HasPrefix(req.Model, "o") {
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

	var toolChoice string
	if len(tools) > 0 {
		if config.ToolChoice != "" {
			toolChoice = string(config.ToolChoice)
		} else if len(tools) > 0 {
			toolChoice = "auto"
		}
	}

	req.Tools = tools
	req.ToolChoice = toolChoice
	req.Temperature = config.Temperature
	req.PresencePenalty = config.PresencePenalty
	req.FrequencyPenalty = config.FrequencyPenalty
	req.ReasoningEffort = ReasoningEffort(config.ReasoningEffort)
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
	// Unmarshal the event
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

	// Emit message start event if this is the first event
	if s.eventCount == 0 {
		s.eventCount++
		events = append(events, &llm.Event{
			Type: llm.EventTypeMessageStart,
			Message: &llm.Response{
				ID:      s.responseID,
				Type:    "message",
				Role:    llm.Assistant,
				Model:   s.responseModel,
				Content: []llm.Content{},
				Usage: llm.Usage{
					InputTokens:  s.usage.PromptTokens,
					OutputTokens: s.usage.CompletionTokens,
				},
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
		// If this is a new content block, stop any open content blocks and
		// generate a content_block_start event
		index := choice.Index
		if _, exists := s.contentBlocks[index]; !exists {
			// Stop any previous content blocks that are still open
			for prevIndex, prev := range s.contentBlocks {
				if !prev.IsComplete {
					stopIndex := prevIndex
					events = append(events, &llm.Event{
						Type:  llm.EventTypeContentBlockStop,
						Index: &stopIndex,
					})
					prev.IsComplete = true
				}
			}
			// Stop any previous tool calls that are still open
			for prevIndex, prev := range s.toolCalls {
				if !prev.IsComplete {
					stopIndex := prevIndex
					events = append(events, &llm.Event{
						Type:  llm.EventTypeContentBlockStop,
						Index: &stopIndex,
					})
					prev.IsComplete = true
				}
			}
			s.contentBlocks[index] = &ContentBlockAccumulator{Type: "text"}
			events = append(events, &llm.Event{
				Type:         llm.EventTypeContentBlockStart,
				Index:        &index,
				ContentBlock: &llm.EventContentBlock{Type: "text"},
			})
		}
		// Generate a content_block_delta event
		events = append(events, &llm.Event{
			Type:  llm.EventTypeContentBlockDelta,
			Index: &index,
			Delta: &llm.EventDelta{
				Type: llm.EventDeltaTypeText,
				Text: choice.Delta.Content,
			},
		})
	}

	if len(choice.Delta.ToolCalls) > 0 {
		for _, toolCallDelta := range choice.Delta.ToolCalls {
			index := toolCallDelta.Index
			// If this is a new tool call, generate a content_block_start event
			if _, exists := s.toolCalls[index]; !exists {
				// Stop any previous content blocks that are still open
				for prevIndex, prev := range s.contentBlocks {
					if !prev.IsComplete {
						stopIndex := prevIndex
						events = append(events, &llm.Event{
							Type:  llm.EventTypeContentBlockStop,
							Index: &stopIndex,
						})
						prev.IsComplete = true
					}
				}
				// Stop any previous tool calls that are still open
				for prevIndex, prev := range s.toolCalls {
					if !prev.IsComplete {
						stopIndex := prevIndex
						events = append(events, &llm.Event{
							Type:  llm.EventTypeContentBlockStop,
							Index: &stopIndex,
						})
						prev.IsComplete = true
					}
				}
				s.toolCalls[index] = &ToolCallAccumulator{Type: "function"}
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStart,
					Index: &index,
					ContentBlock: &llm.EventContentBlock{
						ID:   toolCallDelta.ID,
						Name: toolCallDelta.Function.Name,
						Type: llm.ContentTypeToolUse,
					},
				})
			}
			toolCall := s.toolCalls[index]
			if toolCallDelta.ID != "" {
				toolCall.ID = toolCallDelta.ID
				// Update the ContentBlock in the event queue if it exists
				for _, queuedEvent := range s.eventQueue {
					if queuedEvent.Type == llm.EventTypeContentBlockStart && queuedEvent.Index != nil && *queuedEvent.Index == index {
						if queuedEvent.ContentBlock == nil {
							queuedEvent.ContentBlock = &llm.EventContentBlock{Type: "tool_use"}
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
					if queuedEvent.Type == llm.EventTypeContentBlockStart && queuedEvent.Index != nil && *queuedEvent.Index == index {
						if queuedEvent.ContentBlock == nil {
							queuedEvent.ContentBlock = &llm.EventContentBlock{Type: "tool_use"}
						}
						queuedEvent.ContentBlock.Name = toolCallDelta.Function.Name
					}
				}
			}
			if toolCallDelta.Function.Arguments != "" {
				toolCall.Arguments += toolCallDelta.Function.Arguments
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockDelta,
					Index: &index,
					Delta: &llm.EventDelta{
						Type:        llm.EventDeltaTypeInputJSON,
						PartialJSON: toolCallDelta.Function.Arguments,
					},
				})
			}
		}
	}

	if choice.FinishReason != "" {
		// Stop any open content blocks
		for index, block := range s.contentBlocks {
			blockIndex := index
			if !block.IsComplete {
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStop,
					Index: &blockIndex,
				})
				block.IsComplete = true
			}
		}
		// Stop any open tool calls
		for index, toolCall := range s.toolCalls {
			blockIndex := index
			if !toolCall.IsComplete {
				events = append(events, &llm.Event{
					Type:  llm.EventTypeContentBlockStop,
					Index: &blockIndex,
				})
				toolCall.IsComplete = true
			}
		}
		// Add message_delta event with stop reason
		stopReason := choice.FinishReason
		if stopReason == "tool_calls" {
			stopReason = "tool_use" // Match Anthropic
		}
		events = append(events, &llm.Event{
			Type:  llm.EventTypeMessageDelta,
			Delta: &llm.EventDelta{StopReason: stopReason},
			Usage: &llm.Usage{
				InputTokens:  s.usage.PromptTokens,
				OutputTokens: s.usage.CompletionTokens,
			},
		})
		events = append(events, &llm.Event{
			Type: llm.EventTypeMessageStop,
		})
	}

	return events, nil
}

func (s *StreamIterator) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.body.Close() })
	return err
}

func (s *StreamIterator) Err() error {
	return s.err
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
