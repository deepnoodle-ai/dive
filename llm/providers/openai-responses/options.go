package openairesponses

import (
	"net/http"
	"time"
)

// Provider creation options (set once when creating the provider)
type Option func(*Provider)

// Infrastructure options - set once per provider instance
func WithAPIKey(apiKey string) Option {
	return func(p *Provider) {
		p.apiKey = apiKey
	}
}

func WithEndpoint(endpoint string) Option {
	return func(p *Provider) {
		p.endpoint = endpoint
	}
}

func WithClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

func WithModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}

// Retry configuration
func WithMaxRetries(maxRetries int) Option {
	return func(p *Provider) {
		p.maxRetries = maxRetries
	}
}

func WithBaseWait(baseWait time.Duration) Option {
	return func(p *Provider) {
		p.baseWait = baseWait
	}
}
