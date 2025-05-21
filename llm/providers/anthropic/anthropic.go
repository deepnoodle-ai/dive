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
	DefaultMaxTokens = 8192
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
		endpoint: DefaultEndpoint,
		client:   http.DefaultClient,
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

func (p *Provider) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	config := &llm.Config{}
	config.Apply(opts...)

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}
	msgs, err := convertMessages(config.Messages)
	if err != nil {
		return nil, err
	}
	if config.Caching == nil || *config.Caching {
		// lastMessage := msgs[len(msgs)-1]
		// if len(lastMessage.Content) > 0 {
		// 	lastContent := lastMessage.Content[len(lastMessage.Content)-1]
		// 	lastContent.CacheControl = &llm.CacheControl{Type: llm.CacheControlTypeEphemeral}
		// }
		// Remove cache control from prior messages if present
		// for i := 0; i < len(msgs)-1; i++ {
		// 	message := msgs[i]
		// 	for _, content := range message.Content {
		// 		content.CacheControl = nil
		// 	}
		// }
	}
	if config.Prefill != "" {
		msgs = append(msgs, llm.NewAssistantTextMessage(config.Prefill))
	}
	request.Messages = msgs

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	fmt.Printf("request body: %s\n", string(body))

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

	var result llm.Response
	err = retry.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(body))
		if err != nil {
			return fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Set("x-api-key", p.apiKey)
		req.Header.Set("anthropic-version", p.version)
		req.Header.Set("content-type", "application/json")
		req.Header.Add("anthropic-beta", "prompt-caching-2024-07-31")
		if config.IsFeatureEnabled(FeatureOutput128k) {
			req.Header.Add("anthropic-beta", FeatureOutput128k)
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
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from anthropic api")
	}
	if config.Prefill != "" {
		addPrefill(result.Content, config.Prefill, config.PrefillClosingTag)
	}

	if err := config.FireHooks(ctx, &llm.HookContext{
		Type: llm.AfterGenerate,
		Request: &llm.HookRequestContext{
			Messages: config.Messages,
			Config:   config,
			Body:     body,
		},
		Response: &llm.HookResponseContext{
			Response: &result,
		},
	}); err != nil {
		return nil, err
	}
	return &result, nil
}

func addPrefill(blocks []llm.Content, prefill, closingTag string) error {
	if prefill == "" {
		return nil
	}
	for _, block := range blocks {
		content, ok := block.(*llm.TextContent)
		if ok {
			if closingTag == "" || strings.Contains(content.Text, closingTag) {
				content.Text = prefill + content.Text
				return nil
			}
			return fmt.Errorf("prefill closing tag not found")
		}
	}
	return fmt.Errorf("no text content found in message")
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
	if config.Caching == nil || *config.Caching {
		// lastMessage := msgs[len(msgs)-1]
		// if len(lastMessage.Content) > 0 {
		// 	lastContent := lastMessage.Content[len(lastMessage.Content)-1]
		// 	lastContent.CacheControl = &llm.CacheControl{Type: llm.CacheControlTypeEphemeral}
		// }
		// Remove cache control from prior messages if present
		// for i := 0; i < len(msgs)-1; i++ {
		// 	message := msgs[i]
		// 	for _, content := range message.Content {
		// 		content.CacheControl = nil
		// 	}
		// }
	}
	if config.Prefill != "" {
		msgs = append(msgs, llm.NewAssistantTextMessage(config.Prefill))
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

	var stream *StreamIterator
	err = retry.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(body))
		if err != nil {
			return fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Set("x-api-key", p.apiKey)
		req.Header.Set("anthropic-version", p.version)
		req.Header.Set("content-type", "application/json")
		req.Header.Set("accept", "text/event-stream")
		req.Header.Add("anthropic-beta", "prompt-caching-2024-07-31")
		if config.IsFeatureEnabled(FeatureOutput128k) {
			req.Header.Add("anthropic-beta", FeatureOutput128k)
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

func convertMessages(messages []*llm.Message) ([]*llm.Message, error) {
	messageCount := len(messages)
	if messageCount == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	for i, message := range messages {
		if len(message.Content) == 0 {
			return nil, fmt.Errorf("empty message detected (index %d)", i)
		}
	}
	// Workaround for Anthropic bug
	reorderMessageContent(messages)
	// Anthropic errors if a message ID is set, so make a copy of the messages
	// and omit the ID field
	copied := make([]*llm.Message, len(messages))
	for i, message := range messages {
		// The "name" field in tool results can't be set either
		var copiedContent []llm.Content
		for _, content := range message.Content {
			toolResultContent, ok := content.(*llm.ToolResultContent)
			if ok {
				copiedContent = append(copiedContent, &llm.ToolResultContent{
					Content:   toolResultContent.Content,
					ToolUseID: toolResultContent.ToolUseID,
				})
			} else {
				copiedContent = append(copiedContent, content)
			}
		}
		copied[i] = &llm.Message{
			Role:    message.Role,
			Content: copiedContent,
		}
	}
	return copied, nil
}

func (p *Provider) applyRequestConfig(req *Request, config *llm.Config) error {
	if model := config.Model; model != "" {
		req.Model = model
	} else {
		req.Model = p.model
	}
	if maxTokens := config.MaxTokens; maxTokens != nil {
		req.MaxTokens = maxTokens
	} else {
		req.MaxTokens = &p.maxTokens
	}

	if config.ReasoningBudget != nil {
		budget := *config.ReasoningBudget
		if budget < 1024 {
			return fmt.Errorf("reasoning budget must be at least 1024")
		}
		req.Thinking = &Thinking{
			Type:         "enabled",
			BudgetTokens: budget,
		}
	}

	// Compatibility with the OpenAI "effort" parameter
	if config.ReasoningEffort != "" {
		if req.Thinking != nil {
			return fmt.Errorf("cannot set both reasoning budget and effort")
		}
		req.Thinking = &Thinking{Type: "enabled"}
		switch config.ReasoningEffort {
		case "low":
			req.Thinking.BudgetTokens = 1024
		case "medium":
			req.Thinking.BudgetTokens = 4096
		case "high":
			req.Thinking.BudgetTokens = 16384
		default:
			return fmt.Errorf("invalid reasoning effort: %s", config.ReasoningEffort)
		}
	}

	if len(config.Tools) > 0 {
		var tools []map[string]any
		for _, tool := range config.Tools {
			tools = append(tools, map[string]any{
				"name":         tool.Name(),
				"description":  tool.Description(),
				"input_schema": tool.Schema(),
			})
		}
		req.Tools = tools
	}

	if config.ToolChoice != "" {
		req.ToolChoice = &ToolChoice{
			Type: ToolChoiceType(config.ToolChoice),
			Name: config.ToolChoiceName,
		}
		if config.ParallelToolCalls != nil && !*config.ParallelToolCalls {
			req.ToolChoice.DisableParallelUse = true
		}
	}

	req.Temperature = config.Temperature
	req.System = config.SystemPrompt
	return nil
}

func reorderMessageContent(messages []*llm.Message) {
	// For each assistant message, reorder content blocks so that all tool_use
	// content blocks appear after non-tool_use content blocks, while preserving
	// relative ordering within each group. This works-around an Anthropic bug.
	for _, msg := range messages {
		if msg.Role != llm.Assistant || len(msg.Content) < 2 {
			continue
		}
		// Separate blocks into tool use and non-tool use
		var toolUseBlocks []llm.Content
		var otherBlocks []llm.Content
		for _, block := range msg.Content {
			if block.Type() == llm.ContentTypeToolUse {
				toolUseBlocks = append(toolUseBlocks, block)
			} else {
				otherBlocks = append(otherBlocks, block)
			}
		}
		// If we found any tool use blocks and other blocks, reorder them
		if len(toolUseBlocks) > 0 && len(otherBlocks) > 0 {
			// Combine slices with non-tool-use blocks first
			msg.Content = append(otherBlocks, toolUseBlocks...)
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
	prefill           string
	prefillClosingTag string
	closeOnce         sync.Once
}

type ContentBlockAccumulator struct {
	Type        string
	Text        string
	PartialJSON string
	ToolUse     *ToolUse
	Thinking    string
	Signature   string
}

type ToolUse struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// Next advances to the next event in the stream. Returns true if an event was
// successfully read, false when the stream is complete or an error occurs.
func (s *StreamIterator) Next() bool {
	for {
		event, err := s.readNext()
		if err != nil {
			if err != io.EOF {
				s.err = err
			}
			s.Close()
			return false
		}
		if event != nil {
			s.currentEvent = event
			return true
		}
	}
}

// Event returns the current event. Should only be called after a successful Next().
func (s *StreamIterator) Event() *llm.Event {
	return s.currentEvent
}

// readNext processes a single line from the stream and returns an event
func (s *StreamIterator) readNext() (*llm.Event, error) {
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
	var event llm.Event
	if err := json.Unmarshal(line, &event); err != nil {
		return nil, err
	}
	if event.Type == "" {
		return nil, fmt.Errorf("invalid event detected")
	}
	return &event, nil
}

func (s *StreamIterator) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.body.Close() })
	return err
}

func (s *StreamIterator) Err() error {
	return s.err
}
