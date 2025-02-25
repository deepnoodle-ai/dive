package llm

import (
	"net/http"
	"strings"

	"github.com/getstingrai/dive/logger"
)

const CacheControlEphemeral = "ephemeral"

// Option is a function that configures LLM calls.
type Option func(*Config)

type Config struct {
	Model        string
	SystemPrompt string
	CacheControl string
	Endpoint     string
	APIKey       string
	MaxTokens    *int
	Temperature  *float64
	Tools        []Tool
	ToolChoice   ToolChoice
	LogLevel     string
	Hooks        Hooks
	Client       *http.Client
	Logger       logger.Logger
}

// WithModel sets the LLM model for the generation.
func WithModel(model string) Option {
	return func(config *Config) {
		config.Model = model
	}
}

// WithLogLevel sets the log level.
func WithLogLevel(logLevel string) Option {
	return func(config *Config) {
		value := strings.ToUpper(logLevel)
		switch value {
		case "DEBUG", "INFO", "WARN", "ERROR":
		default:
			value = "INFO"
		}
		config.LogLevel = value
	}
}

// WithLogger sets the logger.
func WithLogger(logger logger.Logger) Option {
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
