package llm

import (
	"net/http"

	"github.com/getstingrai/dive/slogger"
)

const CacheControlEphemeral = "ephemeral"

// Option is a function that configures LLM calls.
type Option func(*Config)

type Config struct {
	Model             string
	SystemPrompt      string
	CacheControl      string
	Endpoint          string
	APIKey            string
	Prefill           string
	PrefillClosingTag string
	MaxTokens         *int
	Temperature       *float64
	PresencePenalty   *float64
	FrequencyPenalty  *float64
	ReasoningFormat   string
	ReasoningEffort   string
	Tools             []Tool
	ToolChoice        ToolChoice
	Hooks             Hooks
	Client            *http.Client
	Logger            slogger.Logger
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

// WithCacheControl sets the cache control for the interaction.
func WithCacheControl(cacheControl string) Option {
	return func(config *Config) {
		config.CacheControl = cacheControl
	}
}

// WithHook adds a hook for the specified event type
func WithHook(hookType HookType, hook Hook) Option {
	return func(config *Config) {
		if config.Hooks == nil {
			config.Hooks = make(Hooks)
		}
		config.Hooks[hookType] = hook
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

// WithReasoningFormat sets the reasoning format for the interaction.
func WithReasoningFormat(reasoningFormat string) Option {
	return func(config *Config) {
		config.ReasoningFormat = reasoningFormat
	}
}

// WithReasoningEffort sets the reasoning effort for the interaction.
func WithReasoningEffort(reasoningEffort string) Option {
	return func(config *Config) {
		config.ReasoningEffort = reasoningEffort
	}
}
