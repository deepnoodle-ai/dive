package openai

import (
	"net/http"

	"github.com/openai/openai-go/option"
)

// Option is a function that configures the Provider
type Option func(*Provider)

// WithAPIKey sets the OpenAI API key.
func WithAPIKey(apiKey string) Option {
	return func(p *Provider) {
		p.options = append(p.options, option.WithAPIKey(apiKey))
	}
}

// WithEndpoint sets the API endpoint URL.
func WithEndpoint(endpoint string) Option {
	return func(p *Provider) {
		p.options = append(p.options, option.WithBaseURL(endpoint))
	}
}

// WithClient sets the HTTP client.
func WithClient(client *http.Client) Option {
	return func(p *Provider) {
		p.options = append(p.options, option.WithHTTPClient(client))
	}
}

// WithModel sets the model name.
func WithModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}

// WithMaxTokens sets the maximum number of tokens to generate.
func WithMaxTokens(maxTokens int) Option {
	return func(p *Provider) {
		p.maxTokens = maxTokens
	}
}

// WithMaxRetries sets the maximum number of retry attempts.
func WithMaxRetries(maxRetries int) Option {
	return func(p *Provider) {
		p.options = append(p.options, option.WithMaxRetries(maxRetries))
	}
}
