package llm

import (
	"context"
	"net/http"

	"github.com/diveagents/dive/slogger"
)

// CacheControlType is used to control how the LLM caches responses.
type CacheControlType string

const (
	CacheControlTypeEphemeral CacheControlType = "ephemeral"
)

func (c CacheControlType) String() string {
	return string(c)
}

// WebSearchConfig configures web search behavior for providers that support it
type WebSearchConfig struct {
	Enabled      bool          `json:"enabled"`
	Domains      []string      `json:"domains,omitempty"`
	ContextSize  string        `json:"context_size,omitempty"` // "low", "medium", "high"
	UserLocation *UserLocation `json:"user_location,omitempty"`
}

// UserLocation represents geographical information for web search
type UserLocation struct {
	Country string  `json:"country,omitempty"`
	Region  string  `json:"region,omitempty"`
	City    string  `json:"city,omitempty"`
	Lat     float64 `json:"lat,omitempty"`
	Lon     float64 `json:"lon,omitempty"`
}

// ImageGenerationConfig configures image generation behavior for providers that support it
type ImageGenerationConfig struct {
	Enabled       bool   `json:"enabled"`
	Size          string `json:"size,omitempty"`           // "1024x1024", "1024x1536", etc.
	Quality       string `json:"quality,omitempty"`        // "low", "medium", "high", "auto"
	Format        string `json:"format,omitempty"`         // "png", "jpeg", "webp"
	Compression   *int   `json:"compression,omitempty"`    // 0-100 for JPEG/WebP
	Background    string `json:"background,omitempty"`     // "transparent", "opaque", "auto"
	PartialImages *int   `json:"partial_images,omitempty"` // 1-3 for streaming
}

// Option is a function that is used to adjust LLM configuration.
type Option func(*Config)

// Config is used to configure LLM calls. Not all providers support all options.
// If a provider doesn't support a given option, it will be ignored.
type Config struct {
	Model             string            `json:"model,omitempty"`
	SystemPrompt      string            `json:"system_prompt,omitempty"`
	Endpoint          string            `json:"endpoint,omitempty"`
	APIKey            string            `json:"api_key,omitempty"`
	Prefill           string            `json:"prefill,omitempty"`
	PrefillClosingTag string            `json:"prefill_closing_tag,omitempty"`
	MaxTokens         *int              `json:"max_tokens,omitempty"`
	Temperature       *float64          `json:"temperature,omitempty"`
	PresencePenalty   *float64          `json:"presence_penalty,omitempty"`
	FrequencyPenalty  *float64          `json:"frequency_penalty,omitempty"`
	ReasoningBudget   *int              `json:"reasoning_budget,omitempty"`
	ReasoningEffort   string            `json:"reasoning_effort,omitempty"`
	Tools             []Tool            `json:"tools,omitempty"`
	ToolChoice        ToolChoice        `json:"tool_choice,omitempty"`
	ToolChoiceName    string            `json:"tool_choice_name,omitempty"`
	ParallelToolCalls *bool             `json:"parallel_tool_calls,omitempty"`
	Features          []string          `json:"features,omitempty"`
	RequestHeaders    http.Header       `json:"request_headers,omitempty"`
	MCPServers        []MCPServerConfig `json:"mcp_servers,omitempty"`
	Caching           *bool             `json:"caching,omitempty"`

	// New generic options promoted from provider-specific implementations
	TopP               *float64    `json:"top_p,omitempty"`
	User               string      `json:"user,omitempty"`
	Instructions       string      `json:"instructions,omitempty"`
	ServiceTier        string      `json:"service_tier,omitempty"`
	TruncationStrategy string      `json:"truncation_strategy,omitempty"`
	Background         *bool       `json:"background,omitempty"`
	JSONSchema         interface{} `json:"json_schema,omitempty"`

	// Structured configuration for complex features
	WebSearch       *WebSearchConfig       `json:"web_search,omitempty"`
	ImageGeneration *ImageGenerationConfig `json:"image_generation,omitempty"`

	// Provider-specific options that don't fit into generic categories
	ProviderOptions map[string]interface{} `json:"provider_options,omitempty"`

	Hooks    Hooks          `json:"-"`
	Client   *http.Client   `json:"-"`
	Logger   slogger.Logger `json:"-"`
	Messages Messages       `json:"-"`
}

// Apply applies the given options to the config.
func (c *Config) Apply(opts ...Option) {
	for _, opt := range opts {
		opt(c)
	}
}

// FireHooks fires the configured hooks for the matching hook type.
func (c *Config) FireHooks(ctx context.Context, hookCtx *HookContext) error {
	for _, hook := range c.Hooks {
		if hook.Type == hookCtx.Type {
			if err := hook.Func(ctx, hookCtx); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsFeatureEnabled returns true if the feature is enabled.
func (c *Config) IsFeatureEnabled(feature string) bool {
	for _, f := range c.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// WithModel sets the LLM model for the generation.
func WithModel(model string) Option {
	return func(config *Config) {
		config.Model = model
	}
}

// WithLogger sets the logger.
func WithLogger(logger slogger.Logger) Option {
	return func(config *Config) {
		config.Logger = logger
	}
}

// WithMaxTokens sets the max tokens.
func WithMaxTokens(maxTokens int) Option {
	return func(config *Config) {
		config.MaxTokens = &maxTokens
	}
}

// WithEndpoint sets the endpoint.
func WithEndpoint(endpoint string) Option {
	return func(config *Config) {
		config.Endpoint = endpoint
	}
}

// WithClient sets the client.
func WithClient(client *http.Client) Option {
	return func(config *Config) {
		config.Client = client
	}
}

// WithTemperature sets the temperature.
func WithTemperature(temperature float64) Option {
	return func(config *Config) {
		config.Temperature = &temperature
	}
}

// WithSystemPrompt sets the system prompt.
func WithSystemPrompt(systemPrompt string) Option {
	return func(config *Config) {
		config.SystemPrompt = systemPrompt
	}
}

// WithTools sets the tools for the interaction.
func WithTools(tools ...Tool) Option {
	return func(config *Config) {
		config.Tools = tools
	}
}

// WithToolChoice sets the tool choice for the interaction.
func WithToolChoice(toolChoice ToolChoice) Option {
	return func(config *Config) {
		config.ToolChoice = toolChoice
	}
}

// WithToolChoiceName sets the tool choice name for the interaction.
func WithToolChoiceName(toolChoiceName string) Option {
	return func(config *Config) {
		config.ToolChoiceName = toolChoiceName
	}
}

// WithParallelToolCalls sets whether to allow parallel tool calls.
func WithParallelToolCalls(parallelToolCalls bool) Option {
	return func(config *Config) {
		config.ParallelToolCalls = &parallelToolCalls
	}
}

// WithHook adds a callback for the specified hook type
func WithHook(hookType HookType, hookFunc HookFunc) Option {
	return func(config *Config) {
		config.Hooks = append(config.Hooks, Hook{
			Type: hookType,
			Func: hookFunc,
		})
	}
}

// WithHooks sets the hooks for the interaction.
func WithHooks(hooks Hooks) Option {
	return func(config *Config) {
		config.Hooks = hooks
	}
}

// WithAPIKey sets the API key.
func WithAPIKey(apiKey string) Option {
	return func(config *Config) {
		config.APIKey = apiKey
	}
}

// WithPrefill sets the prefilled assistant response for the interaction.
func WithPrefill(prefill, closingTag string) Option {
	return func(config *Config) {
		config.Prefill = prefill
		config.PrefillClosingTag = closingTag
	}
}

// WithPresencePenalty sets the presence penalty for the interaction.
func WithPresencePenalty(presencePenalty float64) Option {
	return func(config *Config) {
		config.PresencePenalty = &presencePenalty
	}
}

// WithFrequencyPenalty sets the frequency penalty for the interaction.
func WithFrequencyPenalty(frequencyPenalty float64) Option {
	return func(config *Config) {
		config.FrequencyPenalty = &frequencyPenalty
	}
}

// WithReasoningBudget sets the reasoning budget for the interaction.
func WithReasoningBudget(reasoningBudget int) Option {
	return func(config *Config) {
		config.ReasoningBudget = &reasoningBudget
	}
}

// WithReasoningEffort sets the reasoning effort for the interaction.
func WithReasoningEffort(reasoningEffort string) Option {
	return func(config *Config) {
		config.ReasoningEffort = reasoningEffort
	}
}

// WithFeatures sets the features for the interaction.
func WithFeatures(features ...string) Option {
	return func(config *Config) {
		config.Features = append(config.Features, features...)
	}
}

// WithMessages sets the messages for the interaction.
func WithMessages(messages ...*Message) Option {
	return func(config *Config) {
		config.Messages = messages
	}
}

// WithUserTextMessage sets a single user text message for the interaction.
func WithUserTextMessage(text string) Option {
	return func(config *Config) {
		config.Messages = Messages{NewUserTextMessage(text)}
	}
}

// WithRequestHeaders sets the request headers for the interaction.
func WithRequestHeaders(headers http.Header) Option {
	return func(config *Config) {
		config.RequestHeaders = headers
	}
}

// WithMCPServers sets the remote MCP servers for the interaction.
// Used to configure MCP servers that the LLM provider itself is going to call.
// Corresponds to the Anthropic "MCP connector" feature:
// https://docs.anthropic.com/en/docs/agents-and-tools/mcp-connector
func WithMCPServers(servers ...MCPServerConfig) Option {
	return func(config *Config) {
		config.MCPServers = append(config.MCPServers, servers...)
	}
}

// WithTopP sets the top-p sampling parameter for the interaction.
func WithTopP(topP float64) Option {
	return func(config *Config) {
		config.TopP = &topP
	}
}

// WithUser sets the user identifier for the interaction.
func WithUser(user string) Option {
	return func(config *Config) {
		config.User = user
	}
}

// WithInstructions sets additional instructions for the interaction.
func WithInstructions(instructions string) Option {
	return func(config *Config) {
		config.Instructions = instructions
	}
}

// WithServiceTier sets the service tier for the interaction.
func WithServiceTier(tier string) Option {
	return func(config *Config) {
		config.ServiceTier = tier
	}
}

// WithTruncationStrategy sets the truncation strategy for the interaction.
func WithTruncationStrategy(strategy string) Option {
	return func(config *Config) {
		config.TruncationStrategy = strategy
	}
}

// WithBackground sets whether to process the request in the background.
func WithBackground(background bool) Option {
	return func(config *Config) {
		config.Background = &background
	}
}

// WithJSONSchema sets the JSON schema for structured output.
func WithJSONSchema(schema interface{}) Option {
	return func(config *Config) {
		config.JSONSchema = schema
	}
}

// WithWebSearch enables web search with the specified configuration.
func WithWebSearch(webSearchConfig WebSearchConfig) Option {
	return func(config *Config) {
		config.WebSearch = &webSearchConfig
	}
}

// WithWebSearchEnabled enables web search with default configuration.
func WithWebSearchEnabled() Option {
	return func(config *Config) {
		config.WebSearch = &WebSearchConfig{Enabled: true}
	}
}

// WithImageGeneration enables image generation with the specified configuration.
func WithImageGeneration(imageConfig ImageGenerationConfig) Option {
	return func(config *Config) {
		config.ImageGeneration = &imageConfig
	}
}

// WithImageGenerationEnabled enables image generation with default configuration.
func WithImageGenerationEnabled() Option {
	return func(config *Config) {
		config.ImageGeneration = &ImageGenerationConfig{Enabled: true}
	}
}

// WithProviderOption sets a provider-specific option.
func WithProviderOption(key string, value interface{}) Option {
	return func(config *Config) {
		if config.ProviderOptions == nil {
			config.ProviderOptions = make(map[string]interface{})
		}
		config.ProviderOptions[key] = value
	}
}

// WithProviderOptions sets multiple provider-specific options.
func WithProviderOptions(options map[string]interface{}) Option {
	return func(config *Config) {
		if config.ProviderOptions == nil {
			config.ProviderOptions = make(map[string]interface{})
		}
		for k, v := range options {
			config.ProviderOptions[k] = v
		}
	}
}
