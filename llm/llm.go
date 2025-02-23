package llm

import (
	"context"
	"strings"
)

type LLM interface {
	// Generate a response from the LLM by passing messages.
	Generate(ctx context.Context, messages []*Message, opts ...GenerateOption) (*Response, error)

	// Stream a response from the LLM by passing messages.
	Stream(ctx context.Context, messages []*Message, opts ...GenerateOption) (Stream, error)

	// SupportsStreaming returns true if the LLM supports streaming.
	SupportsStreaming() bool
}

// GenerateOption is a function that configures the generation.
type GenerateOption func(*GenerateConfig)

// GenerateConfig holds configuration parameters for LLM generation.
type GenerateConfig struct {
	Model        string
	SystemPrompt string
	CacheControl string
	MaxTokens    *int
	Temperature  *float64
	Tools        []Tool
	ToolChoice   ToolChoice
	LogLevel     string
	Hooks        Hooks
}

// WithModel sets the LLM model for the generation.
func WithModel(model string) GenerateOption {
	return func(config *GenerateConfig) {
		config.Model = model
	}
}

// WithLogLevel sets the log level.
func WithLogLevel(logLevel string) GenerateOption {
	return func(config *GenerateConfig) {
		value := strings.ToUpper(logLevel)
		switch value {
		case "DEBUG", "INFO", "WARN", "ERROR":
		default:
			value = "INFO"
		}
		config.LogLevel = value
	}
}

// WithMaxTokens sets the max tokens.
func WithMaxTokens(maxTokens int) GenerateOption {
	return func(config *GenerateConfig) {
		config.MaxTokens = &maxTokens
	}
}

// WithTemperature sets the temperature.
func WithTemperature(temperature float64) GenerateOption {
	return func(config *GenerateConfig) {
		config.Temperature = &temperature
	}
}

// WithSystemPrompt sets the system prompt.
func WithSystemPrompt(systemPrompt string) GenerateOption {
	return func(config *GenerateConfig) {
		config.SystemPrompt = systemPrompt
	}
}

// WithTools sets the tools for the interaction.
func WithTools(tools ...Tool) GenerateOption {
	return func(config *GenerateConfig) {
		config.Tools = tools
	}
}

// WithToolChoice sets the tool choice for the interaction.
func WithToolChoice(toolChoice ToolChoice) GenerateOption {
	return func(config *GenerateConfig) {
		config.ToolChoice = toolChoice
	}
}

// WithCacheControl sets the cache control for the interaction.
func WithCacheControl(cacheControl string) GenerateOption {
	return func(config *GenerateConfig) {
		config.CacheControl = cacheControl
	}
}

// WithHook adds a hook for the specified event type
func WithHook(hookType HookType, hook Hook) GenerateOption {
	return func(config *GenerateConfig) {
		if config.Hooks == nil {
			config.Hooks = make(Hooks)
		}
		config.Hooks[hookType] = hook
	}
}

// WithHooks sets the hooks for the interaction.
func WithHooks(hooks Hooks) GenerateOption {
	return func(config *GenerateConfig) {
		config.Hooks = hooks
	}
}
