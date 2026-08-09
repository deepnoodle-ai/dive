package anthropic

import (
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

const ProviderName = "anthropic"

var (
	DefaultEndpoint      = "https://api.anthropic.com/v1/messages"
	DefaultMaxTokens     = 32768
	DefaultClient        = &http.Client{Timeout: 300 * time.Second}
	DefaultMaxRetries    = 3
	DefaultRetryBaseWait = 2 * time.Second
	DefaultVersion       = "2023-06-01"
)

var _ llm.StreamingLLM = &Provider{}

// Provider implements the Anthropic LLM provider for Claude models.
type Provider struct {
	name          string
	client        *http.Client
	apiKey        string
	endpoint      string
	model         string
	maxTokens     int
	maxRetries    int
	retryBaseWait time.Duration
	version       string
}

// New creates a new Anthropic provider with the given options.
func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:        os.Getenv("ANTHROPIC_API_KEY"),
		endpoint:      DefaultEndpoint,
		client:        DefaultClient,
		model:         DefaultModel,
		maxTokens:     DefaultMaxTokens,
		maxRetries:    DefaultMaxRetries,
		retryBaseWait: DefaultRetryBaseWait,
		version:       DefaultVersion,
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
	return ProviderName
}

func (p *Provider) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	config := &llm.Config{}
	config.Apply(opts...)

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}
	rendered, err := p.renderReminders(config.Messages, request.Model)
	if err != nil {
		return nil, err
	}
	msgs, err := convertMessages(rendered)
	if err != nil {
		return nil, err
	}
	if config.Prefill != "" {
		msgs = append(msgs, llm.NewAssistantTextMessage(config.Prefill))
	}
	request.Messages = msgs
	p.applyCaching(&request, config)

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

	var result llm.Response
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
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from anthropic api")
	}
	finalizeUsage(config, request.Model, &result.Usage)
	if config.Prefill != "" {
		if err := addPrefill(result.Content, config.Prefill, config.PrefillClosingTag); err != nil {
			return nil, err
		}
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

func (p *Provider) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
	config := &llm.Config{}
	config.Apply(opts...)

	var request Request
	if err := p.applyRequestConfig(&request, config); err != nil {
		return nil, err
	}
	rendered, err := p.renderReminders(config.Messages, request.Model)
	if err != nil {
		return nil, err
	}
	msgs, err := convertMessages(rendered)
	if err != nil {
		return nil, fmt.Errorf("error converting messages: %w", err)
	}
	if config.Prefill != "" {
		msgs = append(msgs, llm.NewAssistantTextMessage(config.Prefill))
	}
	request.Messages = msgs
	request.Stream = true
	p.applyCaching(&request, config)

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
			body: resp.Body,
			reader: llm.NewServerSentEventsReader[llm.Event](resp.Body).
				WithSSECallback(config.SSECallback),
			prefill:           config.Prefill,
			prefillClosingTag: config.PrefillClosingTag,
		}, nil
	})
	return stream, nil
}

func convertMessages(messages []*llm.Message) ([]*llm.Message, error) {
	messageCount := len(messages)
	if messageCount == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	// Filter out empty messages instead of erroring - they can occur in edge cases
	// during long tool-calling loops and are simply ignored by the API
	filtered := make([]*llm.Message, 0, len(messages))
	for _, message := range messages {
		if len(message.Content) > 0 {
			filtered = append(filtered, message)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("all messages are empty")
	}
	messages = filtered
	// Anthropic errors if a message ID is set, so make a copy of the messages
	// and omit the ID field
	copied := make([]*llm.Message, len(messages))
	for i, message := range messages {
		// The "name" field in tool results can't be set either
		var copiedContent []llm.Content
		for _, content := range message.Content {
			switch c := content.(type) {
			case *llm.ToolResultContent:
				copiedContent = append(copiedContent, &llm.ToolResultContent{
					Content:      convertToolResultBlocks(c),
					ToolUseID:    c.ToolUseID,
					IsError:      c.IsError,
					CacheControl: c.CacheControl,
				})
			case *llm.DocumentContent:
				// Handle DocumentContent with file IDs for Anthropic API compatibility
				if c.Source != nil && c.Source.Type == llm.ContentSourceTypeFile && c.Source.FileID != "" {
					// For Anthropic API, file IDs are passed in the source structure
					docContent := &llm.DocumentContent{
						Title:        c.Title,
						Context:      c.Context,
						Citations:    c.Citations,
						CacheControl: c.CacheControl,
						Source: &llm.ContentSource{
							Type:   c.Source.Type,
							FileID: c.Source.FileID,
						},
					}
					copiedContent = append(copiedContent, docContent)
				} else {
					// Clone to avoid mutating caller's content during applyCacheControl
					copiedContent = append(copiedContent, c.CloneContent())
				}
			default:
				if cloner, ok := content.(llm.ContentCloner); ok {
					copiedContent = append(copiedContent, cloner.CloneContent())
				} else {
					copiedContent = append(copiedContent, content)
				}
			}
		}
		copied[i] = &llm.Message{
			Role:    message.Role,
			Content: copiedContent,
		}
	}
	// Workaround for Anthropic bug. Run on the copies so the caller's
	// messages are not mutated.
	reorderMessageContent(copied)
	return copied, nil
}

// convertToolResultBlocks renders tool_result content in the Anthropic wire
// shape. Typed tool result blocks (from toolkit and MCP tools) carry text in
// {"type":"text","text":...} form, which happens to match Anthropic's text
// block — but image blocks ({"type":"image","data":...,"mimeType":...}) do
// not, so they are converted to native image blocks with a base64 source.
// Content that is not block-shaped passes through unchanged. A result with no
// renderable blocks becomes a single placeholder text block, since Anthropic
// accepts neither an empty text block nor an empty content array.
func convertToolResultBlocks(c *llm.ToolResultContent) any {
	blocks := providers.ToolResultBlocks(c)
	if blocks == nil {
		if providers.IsEmptyToolResultContent(c.Content) {
			return []llm.Content{&llm.TextContent{Text: providers.EmptyToolResultText}}
		}
		return c.Content
	}
	content := make([]llm.Content, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case dive.ToolResultContentTypeImage:
			mediaType := b.MimeType
			if mediaType == "" {
				if detected, err := llm.DetectImageType(b.Data); err == nil {
					mediaType = string(detected)
				}
			}
			if mediaType == "" || b.Data == "" {
				content = append(content, &llm.TextContent{Text: "[image content omitted]"})
				continue
			}
			content = append(content, &llm.ImageContent{
				Source: &llm.ContentSource{
					Type:      llm.ContentSourceTypeBase64,
					MediaType: mediaType,
					Data:      b.Data,
				},
			})
		case dive.ToolResultContentTypeText, "":
			// Anthropic rejects empty text blocks, so skip them.
			if b.Text != "" {
				content = append(content, &llm.TextContent{Text: b.Text})
			}
		default:
			content = append(content, &llm.TextContent{Text: fmt.Sprintf("[%s content omitted]", b.Type)})
		}
	}
	if len(content) == 0 {
		content = append(content, &llm.TextContent{Text: providers.EmptyToolResultText})
	}
	return content
}

const (
	// cacheLookbackWindow is the number of trailing content blocks Anthropic
	// searches backward from a cache breakpoint to find a reusable cache entry.
	// A breakpoint further than this from the prior cached prefix forces a full
	// rewrite of the intervening blocks.
	cacheLookbackWindow = 20
	// cacheAnchorGap is the maximum number of content blocks we allow between
	// consecutive breakpoints. Kept safely under cacheLookbackWindow so the
	// chain of breakpoints stays reachable even when a single turn appends a
	// large fan-out of tool-call / tool-result blocks.
	cacheAnchorGap = 15
)

// applyCaching places cache-control breakpoints across the request using the
// hybrid strategy from docs/specs/anthropic-prompt-caching-hybrid.md:
//
//   - An explicit breakpoint on the last system block caches the stable
//     tools+system prefix independently of the moving message tail, so a
//     message-tier cache miss reads the (large) prefix instead of rewriting it.
//   - The moving conversation tail is cached via Anthropic automatic caching
//     (a top-level cache_control) when the endpoint supports it, or via an
//     explicit tail breakpoint on endpoints that don't (Bedrock / Vertex /
//     custom).
//   - Explicit anchor breakpoints are placed walking backward from the tail so
//     no two breakpoints are more than cacheAnchorGap blocks apart, keeping the
//     chain inside the 20-block lookback window during high tool-call fan-out.
//
// Stable-prefix breakpoints (system + anchors) use the 1-hour TTL when the
// extended-cache feature is enabled; the tail stays at the default 5-minute
// TTL. Breakpoints are capped at the 4 the API allows (automatic consumes one).
//
// The request's messages/system should already be copies (system is built
// fresh; messages come from convertMessages) so mutation here is safe.
func (p *Provider) applyCaching(req *Request, config *llm.Config) {
	// Start from a clean slate so caller-provided cache markers never leak
	// through — in particular on opt-out, where we strip them and bail.
	clearRequestCacheControl(req)
	if config.Caching != nil && !*config.Caching {
		return
	}

	stableTTL := stablePrefixTTL(config)

	// Total explicit block-level breakpoints used so far. The API allows 4; when
	// automatic caching is on it consumes one, leaving 3 for explicit blocks.
	explicitUsed := 0
	explicitBudget := 4

	automatic := p.supportsAutomaticCaching()
	if automatic {
		explicitBudget = 3
		req.CacheControl = &llm.CacheControl{Type: llm.CacheControlTypeEphemeral}
	}

	// Slot: stable tools+system prefix.
	if setLastSystemBreakpoint(req.System, stableTTL) {
		explicitUsed++
	}

	if len(req.Messages) == 0 {
		return
	}

	// Tail handling. Automatic caching owns the tail when supported; otherwise
	// fall back to an explicit breakpoint on the last content block.
	if !automatic {
		if setLastMessageTailBreakpoint(req.Messages) {
			explicitUsed++
		}
	}

	// Anchors defend the 20-block lookback window for the recent conversation.
	remaining := explicitBudget - explicitUsed
	placeCacheAnchors(req.Messages, remaining, stableTTL)
}

// logCacheThrash surfaces prompt-cache thrash: when caching is enabled but a
// request writes far more cache than it reads, a large prefix was rewritten
// instead of reused (e.g. a breakpoint fell outside the 20-block lookback
// window). It warns only on a meaningful write that dominates the read, so
// steady-state cold starts and small requests stay quiet. No-ops without a
// logger or when caching is disabled.
func logCacheThrash(config *llm.Config, usage *llm.Usage) {
	if config.Logger == nil || (config.Caching != nil && !*config.Caching) {
		return
	}
	const minWriteTokens = 4096
	write := usage.CacheCreationInputTokens
	read := usage.CacheReadInputTokens
	if write >= minWriteTokens && write > read {
		config.Logger.Warn("prompt cache write exceeded read; prefix likely rewritten",
			"cache_creation_tokens", write,
			"cache_read_tokens", read,
			"input_tokens", usage.InputTokens)
	}
}

// finalizeUsage runs post-response usage bookkeeping for the non-streaming
// path: it logs cache thrash and attaches an estimated cost from registered
// model pricing. Fast mode is detected from the served speed or the request so
// premium fast-mode pricing is used when applicable. (Streamed responses get
// the same cost treatment centrally via llm.ResponseAccumulator.)
func finalizeUsage(config *llm.Config, model string, usage *llm.Usage) {
	logCacheThrash(config, usage)
	fast := usage.Speed == string(llm.SpeedFast) ||
		config.Speed == llm.SpeedFast ||
		config.IsFeatureEnabled(FeatureFastMode)
	llm.PopulateCost(model, fast, usage)
}

// supportsAutomaticCaching reports whether the configured endpoint supports
// Anthropic automatic prompt caching (top-level cache_control). It is available
// on the first-party Claude API but not on Bedrock or Vertex, which are reached
// through different endpoints.
func (p *Provider) supportsAutomaticCaching() bool {
	return p.endpoint == DefaultEndpoint
}

func (p *Provider) renderReminders(messages []*llm.Message, model string) ([]*llm.Message, error) {
	return llm.RenderReminders(messages, func(index int, all []*llm.Message) (llm.Role, bool) {
		if !p.supportsNativeSystemReminders(model) || !nativeSystemReminderPlacement(index, all) {
			return llm.User, false
		}
		return llm.System, true
	})
}

func (p *Provider) supportsNativeSystemReminders(model string) bool {
	return p.endpoint == DefaultEndpoint && strings.HasPrefix(model, ModelClaudeOpus48)
}

func nativeSystemReminderPlacement(index int, messages []*llm.Message) bool {
	if index <= 0 || index >= len(messages) {
		return false
	}
	previous := messages[index-1]
	if previous == nil || (previous.Role != llm.User && !messageHasServerToolUse(previous)) {
		return false
	}
	if index+1 == len(messages) {
		return true
	}
	next := messages[index+1]
	return next != nil && next.Role == llm.Assistant
}

func messageHasServerToolUse(message *llm.Message) bool {
	if message.Role != llm.Assistant {
		return false
	}
	for _, content := range message.Content {
		if _, ok := content.(*llm.ServerToolUseContent); ok {
			return true
		}
	}
	return false
}

// stablePrefixTTL returns the TTL to use for stable-prefix breakpoints (system
// and anchors). The 1-hour cache is used only when the extended-cache feature
// is enabled; otherwise the default 5-minute cache (empty TTL) applies.
func stablePrefixTTL(config *llm.Config) string {
	if config.IsFeatureEnabled(FeatureExtendedCache) {
		return llm.CacheTTL1h
	}
	return ""
}

// clearRequestCacheControl removes any pre-existing cache_control markers from
// the system blocks and message contents so placement starts from a clean
// slate (some content types preserve CacheControl across convertMessages).
func clearRequestCacheControl(req *Request) {
	req.CacheControl = nil
	for _, block := range req.System {
		block.CacheControl = nil
	}
	for _, message := range req.Messages {
		for _, content := range message.Content {
			if setter, ok := content.(llm.CacheControlSetter); ok {
				setter.SetCacheControl(nil)
			}
		}
	}
}

// setLastSystemBreakpoint marks the final system block with cache control,
// caching the tools+system prefix. Returns false when there is no system prompt.
func setLastSystemBreakpoint(system []*SystemBlock, ttl string) bool {
	if len(system) == 0 {
		return false
	}
	system[len(system)-1].CacheControl = &llm.CacheControl{
		Type: llm.CacheControlTypeEphemeral,
		TTL:  ttl,
	}
	return true
}

// setLastMessageTailBreakpoint sets an explicit ephemeral (5-minute) breakpoint
// on the last cacheable content block of the last message. Used as the
// portability fallback when automatic caching is unavailable.
func setLastMessageTailBreakpoint(messages []*llm.Message) bool {
	contents := messages[len(messages)-1].Content
	for i := len(contents) - 1; i >= 0; i-- {
		if setter, ok := contents[i].(llm.CacheControlSetter); ok {
			setter.SetCacheControl(&llm.CacheControl{Type: llm.CacheControlTypeEphemeral})
			return true
		}
	}
	return false
}

// placeCacheAnchors walks backward over the message content blocks and drops up
// to maxAnchors explicit breakpoints so that consecutive breakpoints are never
// more than cacheAnchorGap blocks apart. This keeps every breakpoint within the
// API's lookback window even when one turn appends a large fan-out of blocks,
// bounding a cache miss to a single gap instead of the whole message prefix.
//
// The tail block (whether owned by automatic caching or an explicit fallback
// breakpoint) is counted toward the first gap but is never itself anchored.
func placeCacheAnchors(messages []*llm.Message, maxAnchors int, ttl string) {
	if maxAnchors <= 0 {
		return
	}
	placed := 0
	blocks := 0
	first := true
	for mi := len(messages) - 1; mi >= 0 && placed < maxAnchors; mi-- {
		contents := messages[mi].Content
		for bi := len(contents) - 1; bi >= 0 && placed < maxAnchors; bi-- {
			// The tail block is already the tail breakpoint (automatic owns the
			// real tail; the explicit fallback marked the last block). Count it
			// toward the first gap but never anchor on it.
			if first {
				first = false
				blocks++
				continue
			}
			blocks++
			if blocks < cacheAnchorGap {
				continue
			}
			if setter, ok := contents[bi].(llm.CacheControlSetter); ok {
				setter.SetCacheControl(&llm.CacheControl{
					Type: llm.CacheControlTypeEphemeral,
					TTL:  ttl,
				})
				placed++
				blocks = 0
			}
		}
	}
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

	if err := applyReasoningConfig(req, config); err != nil {
		return err
	}
	if requestHasThinkingEnabled(req.Model, req.Thinking) && config.Prefill != "" {
		return fmt.Errorf("anthropic extended thinking cannot be used with prefilled assistant responses")
	}

	if config.Speed != "" {
		req.Speed = string(config.Speed)
	}

	if len(config.Tools) > 0 {
		var tools []map[string]any
		for _, tool := range config.Tools {
			// Handle tools that explicitly provide a configuration
			if toolWithConfig, ok := tool.(llm.ToolConfiguration); ok {
				toolConfig := toolWithConfig.ToolConfiguration(p.Name())
				// nil means no configuration is specified and to use the default
				if toolConfig != nil {
					tools = append(tools, toolConfig)
					continue
				}
			}
			// Handle tools with the default configuration behavior
			schema := tool.Schema()
			toolConfig := map[string]any{
				"name":        tool.Name(),
				"description": tool.Description(),
			}
			if schema.Type != "" {
				toolConfig["input_schema"] = schema
			}
			tools = append(tools, toolConfig)
		}
		req.Tools = tools
	}

	if config.ToolChoice != nil && len(config.Tools) > 0 {
		if requestThinkingBlocksForcedToolChoice(req.Model, req.Thinking) && forcedToolChoice(config.ToolChoice.Type) {
			return fmt.Errorf("anthropic extended thinking only supports tool_choice auto or none; got %q", config.ToolChoice.Type)
		}
		req.ToolChoice = &ToolChoice{
			Type: ToolChoiceType(config.ToolChoice.Type),
			Name: config.ToolChoice.Name,
		}
		if config.ParallelToolCalls != nil && !*config.ParallelToolCalls {
			req.ToolChoice.DisableParallelToolUse = true
		}
	}

	if len(config.MCPServers) > 0 {
		req.MCPServers = config.MCPServers
	}

	if config.ContextManagement != nil {
		req.ContextManagement = config.ContextManagement
	}

	if modelAcceptsTemperature(req.Model) && !requestHasThinkingEnabled(req.Model, req.Thinking) {
		req.Temperature = config.Temperature
	} else if config.Temperature != nil && config.Logger != nil {
		config.Logger.Warn("temperature is not supported by this Anthropic request and will be ignored",
			"model", req.Model)
	}
	if config.SystemPrompt != "" {
		req.System = []*SystemBlock{{Type: "text", Text: config.SystemPrompt}}
	}
	return nil
}

// defaultThinkingBudget is the budget used when a caller asks for thinking
// without saying how much — it matches the budget medium effort maps to.
const defaultThinkingBudget = 4096

// minThinkingBudget is the smallest budget the API accepts.
const minThinkingBudget = 1024

func warnf(config *llm.Config, msg string, args ...any) {
	if config.Logger != nil {
		config.Logger.Warn(msg, args...)
	}
}

// applyReasoningConfig maps Dive's reasoning/thinking options onto the Anthropic
// request. Per-model differences come from modelCapabilityTable rather than from
// prefix tests scattered through this file.
//
// Settings a model cannot accept are clamped or dropped with a warning, never
// turned into an error: one ModelSettings is meant to survive being pointed at a
// different model. Unknown models — fine-tunes, gateways, custom deployments —
// keep their parameters untouched, since Dive cannot know what they accept.
func applyReasoningConfig(req *Request, config *llm.Config) error {
	model := req.Model
	caps, known := lookupCapabilities(model)

	thinking := resolveThinking(model, caps, known, config)

	if config.ReasoningEffort != "" {
		thinking = applyEffort(req, config, model, caps, known, thinking)
	}

	if thinking != nil {
		thinking = clampThinkingBudget(req, config, thinking)
	}
	if thinking != nil {
		if config.ThinkingDisplay != "" && thinking.Type != "disabled" {
			thinking.Display = string(config.ThinkingDisplay)
		}
		req.Thinking = thinking
	}
	return nil
}

// applyEffort places the requested effort on the request using whichever
// mechanism the model has: the native effort parameter, an emulated thinking
// budget, or neither.
func applyEffort(
	req *Request,
	config *llm.Config,
	model string,
	caps modelCapabilities,
	known bool,
	thinking *Thinking,
) *Thinking {
	// An unknown model keeps the pre-table behavior: effort is emulated with a
	// thinking budget, since the native parameter is newer than most gateways.
	kind := reasoningLegacyBudget
	if known {
		kind = caps.reasoningKind()
	}

	switch kind {
	case reasoningNone:
		warnf(config, "model does not support reasoning; ignoring reasoning effort",
			"model", model, "effort", config.ReasoningEffort)
		return thinking

	case reasoningNative:
		effort, clamped := llm.ClampReasoningEffort(config.ReasoningEffort, caps.efforts)
		if clamped {
			warnf(config, "model does not support the requested reasoning effort; clamping",
				"model", model, "requested", config.ReasoningEffort, "using", effort)
		}
		// Opus 5 rejects an effort above high while thinking is explicitly
		// disabled, so the cap applies only to that combination.
		if config.Thinking == llm.ThinkingTypeDisabled && caps.disabledEffortCap != "" {
			capped, ok := llm.ClampReasoningEffort(effort, effortsUpTo(caps.efforts, caps.disabledEffortCap))
			if ok && capped != effort {
				warnf(config, "model caps reasoning effort while thinking is disabled; clamping",
					"model", model, "requested", effort, "using", capped)
				effort = capped
			}
		}
		req.OutputConfig = &OutputConfig{Effort: string(effort)}
		return thinking

	default: // reasoningLegacyBudget
		// The model has no native effort parameter, so effort is emulated with
		// a thinking budget. That cannot coexist with an explicit disable or
		// with a budget the caller set themselves. The disable is checked on
		// the config rather than the resolved value, because for most models
		// "disabled" is expressed by omitting the thinking parameter entirely.
		if config.Thinking == llm.ThinkingTypeDisabled {
			warnf(config, "model emulates reasoning effort with a thinking budget; ignoring effort because thinking is disabled",
				"model", model, "effort", config.ReasoningEffort)
			return thinking
		}
		if config.ReasoningBudget != nil {
			warnf(config, "model emulates reasoning effort with a thinking budget; keeping the explicit budget and ignoring effort",
				"model", model, "effort", config.ReasoningEffort)
			return thinking
		}
		budget, ok := legacyEffortBudget(config.ReasoningEffort)
		if !ok {
			warnf(config, "unrecognized reasoning effort for a model without a native effort parameter; ignoring it",
				"model", model, "effort", config.ReasoningEffort)
			return thinking
		}
		return &Thinking{Type: "enabled", BudgetTokens: budget}
	}
}

// resolveThinking determines the thinking configuration from the budget and
// explicit thinking-type options, independent of the effort parameter.
func resolveThinking(model string, caps modelCapabilities, known bool, config *llm.Config) *Thinking {
	// Unknown models keep the pre-table assumption: manual budgets work,
	// adaptive thinking is passed through as asked.
	canBudget := !known || caps.manualBudget
	canAdapt := !known || caps.adaptive

	switch config.Thinking {
	case llm.ThinkingTypeDisabled:
		// Fable 5 and Mythos 5 reject an explicit disable; omitting the
		// parameter is the accepted way to ask them for less.
		if known && !caps.explicitDisable {
			warnf(config, "model does not accept an explicit thinking disable; omitting the thinking parameter",
				"model", model)
			return nil
		}
		// Models that think by default need the explicit disable to actually
		// turn it off. Everywhere else an omitted parameter already means off.
		if known && caps.thinkingOnByDefault {
			return &Thinking{Type: "disabled"}
		}
		return nil

	case llm.ThinkingTypeAdaptive:
		if canAdapt {
			return &Thinking{Type: "adaptive"}
		}
		// The 4.5 generation answers 400 for adaptive thinking. The caller
		// still asked to think, so fall back to the mechanism it does have.
		if canBudget {
			budget := defaultThinkingBudget
			if config.ReasoningBudget != nil {
				budget = *config.ReasoningBudget
			} else if b, ok := legacyEffortBudget(config.ReasoningEffort); ok {
				budget = b
			}
			warnf(config, "model does not support adaptive thinking; using a manual thinking budget",
				"model", model, "budget", budget)
			return &Thinking{Type: "enabled", BudgetTokens: clampBudgetFloor(budget)}
		}
		warnf(config, "model does not support thinking; ignoring the thinking setting", "model", model)
		return nil

	case llm.ThinkingTypeEnabled:
		if !canBudget {
			if canAdapt {
				return &Thinking{Type: "adaptive"}
			}
			warnf(config, "model does not support thinking; ignoring the thinking setting", "model", model)
			return nil
		}
		if config.ReasoningBudget == nil {
			// Asking to think without saying how much: prefer adaptive where
			// the model has it, otherwise pick the default budget.
			if canAdapt {
				return &Thinking{Type: "adaptive"}
			}
			warnf(config, "thinking was enabled without a reasoning budget; using the default budget",
				"model", model, "budget", defaultThinkingBudget)
			return &Thinking{Type: "enabled", BudgetTokens: defaultThinkingBudget}
		}
		// Budget provided: handled by the block below.
	}

	if config.ReasoningBudget != nil {
		budget := *config.ReasoningBudget
		if budget < minThinkingBudget {
			warnf(config, "reasoning budget is below the minimum; clamping",
				"model", model, "requested", budget, "using", minThinkingBudget)
			budget = minThinkingBudget
		}
		if !canBudget {
			if canAdapt {
				warnf(config, "model does not support manual thinking budgets; using adaptive thinking",
					"model", model)
				return &Thinking{Type: "adaptive"}
			}
			warnf(config, "model does not support thinking; ignoring the reasoning budget", "model", model)
			return nil
		}
		return &Thinking{Type: "enabled", BudgetTokens: budget}
	}

	return nil
}

func clampBudgetFloor(budget int) int {
	if budget < minThinkingBudget {
		return minThinkingBudget
	}
	return budget
}

// legacyEffortBudget maps a reasoning effort level to a thinking token budget
// for models that lack the native effort parameter. The bool reports whether
// the effort was recognized.
func legacyEffortBudget(effort llm.ReasoningEffort) (int, bool) {
	switch effort {
	case llm.ReasoningEffortLow, llm.ReasoningEffortMinimal:
		return 1024, true
	case llm.ReasoningEffortMedium:
		return 4096, true
	case llm.ReasoningEffortHigh, llm.ReasoningEffortXHigh, llm.ReasoningEffortMax:
		return 16384, true
	default:
		return 0, false
	}
}

// clampThinkingBudget keeps the thinking budget below max_tokens, which the API
// requires outside interleaved thinking. When max_tokens leaves no room for a
// valid budget at all, thinking is dropped rather than sent as a doomed request.
func clampThinkingBudget(req *Request, config *llm.Config, thinking *Thinking) *Thinking {
	if thinking.Type != "enabled" || config.IsFeatureEnabled(FeatureInterleavedThinking) {
		return thinking
	}
	if req.MaxTokens == nil || thinking.BudgetTokens < *req.MaxTokens {
		return thinking
	}
	capped := *req.MaxTokens - 1
	if capped < minThinkingBudget {
		warnf(config, "max_tokens leaves no room for a thinking budget; disabling thinking",
			"model", req.Model, "max_tokens", *req.MaxTokens, "budget", thinking.BudgetTokens)
		return nil
	}
	warnf(config, "reasoning budget must be below max_tokens; clamping",
		"model", req.Model, "requested", thinking.BudgetTokens, "using", capped)
	thinking.BudgetTokens = capped
	return thinking
}

func requestHasThinkingEnabled(model string, thinking *Thinking) bool {
	if thinking != nil {
		return thinking.Type != "disabled"
	}
	return modelRunsThinkingByDefault(model)
}

// requestThinkingBlocksForcedToolChoice reports whether the request's thinking
// configuration rules out a forced tool_choice. An explicit thinking config is
// authoritative. When the request omits it, only models that always run thinking
// and reject an explicit disable (Fable 5, Mythos 5) block forced tool choice:
// Opus 5 and Sonnet 5 default thinking on but the caller can still turn it off,
// so Dive leaves that request to the API rather than rejecting it here.
func requestThinkingBlocksForcedToolChoice(model string, thinking *Thinking) bool {
	if thinking != nil {
		return thinking.Type != "disabled"
	}
	return modelRunsThinkingByDefault(model) && !modelDefaultsThinkingOn(model)
}

func forcedToolChoice(choice llm.ToolChoiceType) bool {
	return choice == llm.ToolChoiceTypeAny || choice == llm.ToolChoiceTypeTool
}

// The predicates below read modelCapabilityTable. Each keeps the permissive
// answer for an unknown model, so a fine-tune or gateway deployment behaves as
// it did before the table existed.

// modelSupportsEffortParam reports whether the model accepts the native
// output_config.effort parameter.
func modelSupportsEffortParam(model string) bool {
	caps, known := lookupCapabilities(model)
	return known && caps.reasoningKind() == reasoningNative
}

func modelSupportsXHighEffort(model string) bool {
	return modelSupportsEffortLevel(model, llm.ReasoningEffortXHigh)
}

func modelSupportsMaxEffort(model string) bool {
	return modelSupportsEffortLevel(model, llm.ReasoningEffortMax)
}

func modelSupportsEffortLevel(model string, effort llm.ReasoningEffort) bool {
	caps, known := lookupCapabilities(model)
	if !known {
		return false
	}
	for _, level := range caps.efforts {
		if level == effort {
			return true
		}
	}
	return false
}

// modelAcceptsTemperature reports whether the model accepts the temperature
// parameter. Unknown models are assumed to accept it, as they did before.
func modelAcceptsTemperature(model string) bool {
	caps, known := lookupCapabilities(model)
	return !known || caps.temperature
}

// modelRejectsManualThinking reports whether the model rejects manual extended
// thinking budgets and supports only adaptive thinking.
func modelRejectsManualThinking(model string) bool {
	caps, known := lookupCapabilities(model)
	return known && !caps.manualBudget && caps.adaptive
}

// modelDefaultsThinkingOn reports whether the model runs with adaptive thinking
// when the request omits the thinking parameter *and* accepts an explicit
// disable to turn it off. Fable 5 and Mythos 5 also think by default but reject
// the disable, so they are excluded here — see modelRunsThinkingByDefault.
func modelDefaultsThinkingOn(model string) bool {
	caps, known := lookupCapabilities(model)
	return known && caps.thinkingOnByDefault && caps.explicitDisable
}

// modelRejectsDisabledThinkingAboveHighEffort reports whether the model rejects
// an explicit thinking disable combined with an effort level above high.
func modelRejectsDisabledThinkingAboveHighEffort(model string) bool {
	caps, known := lookupCapabilities(model)
	return known && caps.disabledEffortCap != ""
}

// modelRunsThinkingByDefault reports whether omitting the thinking parameter
// still leaves adaptive thinking active.
func modelRunsThinkingByDefault(model string) bool {
	caps, known := lookupCapabilities(model)
	return known && caps.thinkingOnByDefault
}

// createRequest creates an HTTP request with appropriate headers for Anthropic API calls
func (p *Provider) createRequest(ctx context.Context, body []byte, config *llm.Config, isStreaming bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", p.version)
	req.Header.Set("content-type", "application/json")

	if isStreaming {
		req.Header.Set("accept", "text/event-stream")
	}

	var betaFeatures []string
	// Prompt caching is GA and needs no beta header; the extended (1-hour) cache
	// still advertises its beta. Both are only sent when explicitly enabled.
	if config.IsFeatureEnabled(FeatureExtendedCache) {
		betaFeatures = append(betaFeatures, FeatureExtendedCache)
	} else if config.IsFeatureEnabled(FeaturePromptCaching) {
		betaFeatures = append(betaFeatures, FeaturePromptCaching)
	}

	if config.IsFeatureEnabled(FeatureOutput128k) {
		betaFeatures = append(betaFeatures, FeatureOutput128k)
	}

	if config.IsFeatureEnabled(FeatureMCPClientV2) || len(config.MCPServers) > 0 {
		betaFeatures = append(betaFeatures, FeatureMCPClientV2)
	} else if config.IsFeatureEnabled(FeatureMCPClient) {
		betaFeatures = append(betaFeatures, FeatureMCPClient)
	}

	if config.IsFeatureEnabled(FeatureContextManagement) || config.ContextManagement != nil {
		betaFeatures = append(betaFeatures, FeatureContextManagement)
	}

	if config.IsFeatureEnabled(FeatureCodeExecution) {
		betaFeatures = append(betaFeatures, FeatureCodeExecution)
	}

	if config.IsFeatureEnabled(FeatureContext1M) {
		betaFeatures = append(betaFeatures, FeatureContext1M)
	}

	if config.Speed == llm.SpeedFast || config.IsFeatureEnabled(FeatureFastMode) {
		betaFeatures = append(betaFeatures, FeatureFastMode)
	}

	if config.IsFeatureEnabled(FeatureCompact) {
		betaFeatures = append(betaFeatures, FeatureCompact)
	}

	if config.IsFeatureEnabled(FeatureFilesAPI) {
		betaFeatures = append(betaFeatures, FeatureFilesAPI)
	}

	if config.IsFeatureEnabled(FeatureInterleavedThinking) {
		betaFeatures = append(betaFeatures, FeatureInterleavedThinking)
	}

	if config.IsFeatureEnabled(FeatureComputerUse45_46) {
		betaFeatures = append(betaFeatures, FeatureComputerUse45_46)
	} else if config.IsFeatureEnabled(FeatureComputerUse) {
		betaFeatures = append(betaFeatures, FeatureComputerUse)
	}

	if len(betaFeatures) > 0 {
		req.Header.Set("anthropic-beta", strings.Join(betaFeatures, ","))
	}

	for key, values := range config.RequestHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return req, nil
}
