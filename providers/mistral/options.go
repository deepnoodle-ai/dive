package mistral

import (
	"net/http"
	"time"
)

// Option is a function that configures the Provider
type Option func(*Provider)

// WithAPIKey sets the API key for the provider
func WithAPIKey(apiKey string) Option {
	return func(p *Provider) {
		p.apiKey = apiKey
	}
}

// WithEndpoint sets the API endpoint URL for the provider
func WithEndpoint(endpoint string) Option {
	return func(p *Provider) {
		p.endpoint = endpoint
	}
}

// WithClient sets the HTTP client used for all API requests
func WithClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

// WithMaxTokens sets the maximum number of tokens to generate
func WithMaxTokens(maxTokens int) Option {
	return func(p *Provider) {
		p.maxTokens = maxTokens
	}
}

// WithMaxRetries sets the maximum number of retries for transient generation
// failures (total attempts = maxRetries + 1).
func WithMaxRetries(maxRetries int) Option {
	return func(p *Provider) {
		p.maxRetries = maxRetries
	}
}

// WithBaseWait sets the base wait duration between retries.
func WithBaseWait(baseWait time.Duration) Option {
	return func(p *Provider) {
		p.retryBaseWait = baseWait
	}
}

// WithModel sets the LLM model name to use for the provider
func WithModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}

// WithoutToolResultImages keeps images that tools return out of requests,
// replacing each with the text "[image content omitted]", so the model cannot
// see them. Use it with text-only models, which reject a request carrying a
// tool-result image, as does every later request once the image is in the
// conversation history. See openaicompletions.WithoutToolResultImages.
func WithoutToolResultImages() Option {
	return func(p *Provider) {
		p.noToolImages = true
	}
}
