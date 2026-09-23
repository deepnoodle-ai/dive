package deepinfra

import (
	"net/http"
	"time"
)

// Option configures a DeepInfra provider.
type Option func(*config)

func WithAPIKey(key string) Option {
	return func(c *config) { c.apiKey = key }
}

// WithEndpoint sets the full Chat Completions URL, including /chat/completions.
func WithEndpoint(endpoint string) Option {
	return func(c *config) { c.endpoint = endpoint }
}

func WithModel(model string) Option {
	return func(c *config) { c.model = model }
}

func WithClient(client *http.Client) Option {
	return func(c *config) { c.client = client }
}

func WithMaxTokens(tokens int) Option {
	return func(c *config) { c.maxTokens = tokens }
}

func WithMaxRetries(retries int) Option {
	return func(c *config) { c.maxRetries = retries }
}

func WithBaseWait(wait time.Duration) Option {
	return func(c *config) { c.baseWait = wait }
}
