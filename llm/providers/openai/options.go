package openai

import (
	"net/http"

	"github.com/openai/openai-go/option"
)

// Option is a function that configures the Provider
type Option func(*Provider)

func WithAPIKey(apiKey string) Option {
	return func(p *Provider) {
		p.options = append(p.options, option.WithAPIKey(apiKey))
	}
}

func WithEndpoint(endpoint string) Option {
	return func(p *Provider) {
		p.options = append(p.options, option.WithBaseURL(endpoint))
	}
}

func WithClient(client *http.Client) Option {
	return func(p *Provider) {
		p.options = append(p.options, option.WithHTTPClient(client))
	}
}

func WithModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}

func WithMaxTokens(maxTokens int) Option {
	return func(p *Provider) {
		p.maxTokens = maxTokens
	}
}

func WithMaxRetries(maxRetries int) Option {
	return func(p *Provider) {
		p.options = append(p.options, option.WithMaxRetries(maxRetries))
	}
}
