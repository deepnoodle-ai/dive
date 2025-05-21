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

// Option is a function that is used to adjust LLM configuration.
type Option func(*Config)

// Config is used to configure LLM calls. Not all providers support all options.
// If a provider doesn't support a given option, it will be ignored.
type Config struct {
	Model             string         `json:"model,omitempty"`
	SystemPrompt      string         `json:"system_prompt,omitempty"`
	Endpoint          string         `json:"endpoint,omitempty"`
	APIKey            string         `json:"api_key,omitempty"`
	Prefill           string         `json:"prefill,omitempty"`
	PrefillClosingTag string         `json:"prefill_closing_tag,omitempty"`
	MaxTokens         *int           `json:"max_tokens,omitempty"`
	Temperature       *float64       `json:"temperature,omitempty"`
	PresencePenalty   *float64       `json:"presence_penalty,omitempty"`
	FrequencyPenalty  *float64       `json:"frequency_penalty,omitempty"`
	ReasoningBudget   *int           `json:"reasoning_budget,omitempty"`
	ReasoningEffort   string         `json:"reasoning_effort,omitempty"`
	Tools             []Tool         `json:"tools,omitempty"`
	ToolChoice        ToolChoice     `json:"tool_choice,omitempty"`
	ToolChoiceName    string         `json:"tool_choice_name,omitempty"`
	ParallelToolCalls *bool          `json:"parallel_tool_calls,omitempty"`
	Features          []string       `json:"features,omitempty"`
	Hooks             Hooks          `json:"-"`
	Client            *http.Client   `json:"-"`
	Logger            slogger.Logger `json:"-"`
	Messages          Messages       `json:"-"`
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
