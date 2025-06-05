package openaicompletions

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

// WithModel sets the LLM model name to use for the provider
func WithModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}

func WithMaxRetries(maxRetries int) Option {
	return func(p *Provider) {
		p.maxRetries = maxRetries
	}
}

func WithBaseWait(baseWait time.Duration) Option {
	return func(p *Provider) {
		p.retryBaseWait = baseWait
	}
}

// WithSystemRole sets the name of the system role for the provider
func WithSystemRole(systemRole string) Option {
	return func(p *Provider) {
		p.systemRole = systemRole
	}
}
