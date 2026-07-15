package grok

import "github.com/openai/openai-go/v3/option"

// config holds the Grok provider configuration before passing to the
// embedded OpenAI Responses API provider.
type config struct {
	apiKey              string
	endpoint            string
	model               string
	maxTokens           int
	maxRetries          int
	extraRequestOptions []option.RequestOption
}

// Option is a function that configures the Grok Provider.
type Option func(*config)

// WithAPIKey sets the API key for the provider.
func WithAPIKey(apiKey string) Option {
	return func(c *config) {
		c.apiKey = apiKey
	}
}

// WithEndpoint sets the API base URL for the provider.
func WithEndpoint(endpoint string) Option {
	return func(c *config) {
		c.endpoint = endpoint
	}
}

// WithMaxTokens sets the maximum number of tokens to generate.
func WithMaxTokens(maxTokens int) Option {
	return func(c *config) {
		c.maxTokens = maxTokens
	}
}

// WithMaxRetries sets the maximum number of retry attempts performed by Dive
// for transient generation failures (total attempts = maxRetries + 1).
func WithMaxRetries(maxRetries int) Option {
	return func(c *config) {
		c.maxRetries = maxRetries
	}
}

// WithModel sets the LLM model name to use for the provider.
func WithModel(model string) Option {
	return func(c *config) {
		c.model = model
	}
}

// WithPromptCacheKey sets a cache key that routes requests to the same server
// for prompt cache reuse. Use the same key across requests in a conversation
// to maximize cache hits. The key can be any string (e.g., a UUID).
func WithPromptCacheKey(key string) Option {
	return func(c *config) {
		c.extraRequestOptions = append(c.extraRequestOptions,
			option.WithJSONSet("prompt_cache_key", key))
	}
}
