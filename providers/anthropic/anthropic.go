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

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/wonton/retry"
)

const ProviderName = "anthropic"

var (
	DefaultModel         = ModelClaudeOpus48
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
	return ProviderName
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
	logCacheThrash(config, &result.Usage)
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
	msgs, err := convertMessages(config.Messages)
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

	var stream *StreamIterator
	err = retry.DoSimple(ctx, func() error {
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
			body: resp.Body,
			reader: llm.NewServerSentEventsReader[llm.Event](resp.Body).
				WithSSECallback(config.SSECallback),
			prefill:           config.Prefill,
			prefillClosingTag: config.PrefillClosingTag,
		}
		return nil
	}, retry.WithMaxAttempts(p.maxRetries+1), retry.WithBackoff(p.retryBaseWait, 5*time.Minute), retry.WithRetryIf(retry.SkipPermanent()))

	if err != nil {
		return nil, err
	}
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
					Content:      c.Content,
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
	if config.Caching != nil && !*config.Caching {
		return
	}

	stableTTL := stablePrefixTTL(config)
	clearRequestCacheControl(req)

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

// supportsAutomaticCaching reports whether the configured endpoint supports
// Anthropic automatic prompt caching (top-level cache_control). It is available
// on the first-party Claude API but not on Bedrock or Vertex, which are reached
// through different endpoints.
func (p *Provider) supportsAutomaticCaching() bool {
	return p.endpoint == DefaultEndpoint
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

	if !modelRejectsTemperature(req.Model) {
		req.Temperature = config.Temperature
	} else if config.Temperature != nil && config.Logger != nil {
		config.Logger.Warn("temperature is not supported by this model and will be ignored",
			"model", req.Model)
	}
	if config.SystemPrompt != "" {
		req.System = []*SystemBlock{{Type: "text", Text: config.SystemPrompt}}
	}
	return nil
}

// applyReasoningConfig maps Dive's reasoning/thinking options onto the Anthropic
// request, accounting for per-model differences:
//   - Opus 4.5+, Sonnet 4.6, and the Claude 5 models (Fable 5, Mythos 5) take
//     the native effort parameter via output_config; older models emulate
//     effort with a thinking budget.
//   - Opus 4.6/4.7/4.8, Sonnet 4.6, and the Claude 5 models support adaptive
//     thinking. Opus 4.7/4.8 and the Claude 5 models reject manual budgets, so
//     a budget set against them transparently falls back to adaptive thinking.
func applyReasoningConfig(req *Request, config *llm.Config) error {
	model := req.Model

	thinking, err := resolveThinking(model, config)
	if err != nil {
		return err
	}

	if config.ReasoningEffort != "" {
		effort, err := normalizeReasoningEffort(model, config.ReasoningEffort)
		if err != nil {
			return err
		}
		if modelSupportsEffortParam(model) {
			req.OutputConfig = &OutputConfig{Effort: string(effort)}
		} else {
			// Legacy: emulate the effort parameter with a thinking budget.
			// This model lacks the native effort parameter, so honoring effort
			// here would re-enable thinking — don't silently override an
			// explicit disable.
			if config.Thinking == llm.ThinkingTypeDisabled {
				return fmt.Errorf("cannot set reasoning effort with thinking disabled on model %s: it has no native effort parameter and effort is emulated with a thinking budget", model)
			}
			if thinking != nil && config.ReasoningBudget != nil {
				return fmt.Errorf("cannot set both reasoning budget and effort on model %s", model)
			}
			budget, err := legacyEffortBudget(effort)
			if err != nil {
				return err
			}
			thinking = &Thinking{Type: "enabled", BudgetTokens: budget}
		}
	}

	if thinking != nil {
		if config.ThinkingDisplay != "" {
			thinking.Display = string(config.ThinkingDisplay)
		}
		req.Thinking = thinking
	}
	return nil
}

// resolveThinking determines the thinking configuration from the budget and
// explicit thinking-type options, independent of the effort parameter.
func resolveThinking(model string, config *llm.Config) (*Thinking, error) {
	adaptiveOnly := modelRejectsManualThinking(model) // Opus 4.7/4.8, Fable 5, Mythos 5

	switch config.Thinking {
	case llm.ThinkingTypeDisabled:
		return nil, nil
	case llm.ThinkingTypeAdaptive:
		return &Thinking{Type: "adaptive"}, nil
	case llm.ThinkingTypeEnabled:
		if adaptiveOnly {
			return &Thinking{Type: "adaptive"}, nil
		}
		if config.ReasoningBudget == nil {
			return nil, fmt.Errorf("thinking type %q requires a reasoning budget; use WithReasoningBudget or WithAdaptiveThinking", llm.ThinkingTypeEnabled)
		}
		// Budget provided: handled by the block below.
	}

	if config.ReasoningBudget != nil {
		budget := *config.ReasoningBudget
		if budget < 1024 {
			return nil, fmt.Errorf("reasoning budget must be at least 1024")
		}
		if adaptiveOnly {
			if config.Logger != nil {
				config.Logger.Warn("model does not support manual thinking budgets; using adaptive thinking",
					"model", model)
			}
			return &Thinking{Type: "adaptive"}, nil
		}
		return &Thinking{Type: "enabled", BudgetTokens: budget}, nil
	}

	return nil, nil
}

// normalizeReasoningEffort maps provider-neutral efforts onto Anthropic's
// documented effort levels while keeping older low/medium/high behavior intact.
func normalizeReasoningEffort(model string, effort llm.ReasoningEffort) (llm.ReasoningEffort, error) {
	switch effort {
	case llm.ReasoningEffortLow,
		llm.ReasoningEffortMedium,
		llm.ReasoningEffortHigh:
		return effort, nil
	case llm.ReasoningEffortMinimal:
		return llm.ReasoningEffortLow, nil
	case llm.ReasoningEffortNone:
		return "", fmt.Errorf("reasoning effort %q is not supported by Anthropic models", effort)
	case llm.ReasoningEffortXHigh:
		if modelSupportsXHighEffort(model) {
			return effort, nil
		}
		return llm.ReasoningEffortHigh, nil
	case llm.ReasoningEffortMax:
		if modelSupportsMaxEffort(model) {
			return effort, nil
		}
		return llm.ReasoningEffortHigh, nil
	default:
		return effort, nil
	}
}

// legacyEffortBudget maps a reasoning effort level to a thinking token budget
// for older models that lack the native effort parameter.
func legacyEffortBudget(effort llm.ReasoningEffort) (int, error) {
	switch effort {
	case llm.ReasoningEffortLow, llm.ReasoningEffortMinimal:
		return 1024, nil
	case llm.ReasoningEffortMedium:
		return 4096, nil
	case llm.ReasoningEffortHigh, llm.ReasoningEffortXHigh, llm.ReasoningEffortMax:
		return 16384, nil
	default:
		return 0, fmt.Errorf("invalid reasoning effort: %s", effort)
	}
}

// modelSupportsEffortParam reports whether the model accepts the native
// output_config.effort parameter (Opus 4.5+, Sonnet 4.6, Fable 5, Mythos 5).
func modelSupportsEffortParam(model string) bool {
	switch {
	case strings.HasPrefix(model, "claude-opus-4-5"),
		strings.HasPrefix(model, "claude-opus-4-6"),
		strings.HasPrefix(model, "claude-opus-4-7"),
		strings.HasPrefix(model, "claude-opus-4-8"),
		strings.HasPrefix(model, "claude-sonnet-4-6"),
		strings.HasPrefix(model, "claude-fable-5"),
		strings.HasPrefix(model, "claude-mythos-5"):
		return true
	}
	return false
}

func modelSupportsXHighEffort(model string) bool {
	return strings.HasPrefix(model, "claude-opus-4-7") ||
		strings.HasPrefix(model, "claude-opus-4-8") ||
		strings.HasPrefix(model, "claude-fable-5") ||
		strings.HasPrefix(model, "claude-mythos-5")
}

func modelSupportsMaxEffort(model string) bool {
	return strings.HasPrefix(model, "claude-opus-4-6") ||
		strings.HasPrefix(model, "claude-opus-4-7") ||
		strings.HasPrefix(model, "claude-opus-4-8") ||
		strings.HasPrefix(model, "claude-sonnet-4-6") ||
		strings.HasPrefix(model, "claude-fable-5") ||
		strings.HasPrefix(model, "claude-mythos-5")
}

// modelRejectsTemperature reports whether the model rejects the temperature
// parameter (Claude 5 models: Fable 5, Mythos 5).
func modelRejectsTemperature(model string) bool {
	return strings.HasPrefix(model, "claude-fable-5") ||
		strings.HasPrefix(model, "claude-mythos-5")
}

// modelRejectsManualThinking reports whether the model rejects manual extended
// thinking budgets and supports only adaptive thinking (Opus 4.7/4.8, Fable 5,
// Mythos 5). On the Claude 5 models adaptive thinking is always on and an
// explicit thinking disable is also rejected by the API; Dive already omits
// the thinking parameter entirely when thinking is disabled, so disables are
// safe across all of these models.
func modelRejectsManualThinking(model string) bool {
	return strings.HasPrefix(model, "claude-opus-4-7") ||
		strings.HasPrefix(model, "claude-opus-4-8") ||
		strings.HasPrefix(model, "claude-fable-5") ||
		strings.HasPrefix(model, "claude-mythos-5")
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
