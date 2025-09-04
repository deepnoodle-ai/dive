package openrouter

import "net/http"

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

// WithSiteURL sets the site URL for OpenRouter rankings (HTTP-Referer header)
func WithSiteURL(siteURL string) Option {
	return func(p *Provider) {
		p.siteURL = siteURL
	}
}

// WithSiteName sets the site name for OpenRouter rankings (X-Title header)
func WithSiteName(siteName string) Option {
	return func(p *Provider) {
		p.siteName = siteName
	}
}