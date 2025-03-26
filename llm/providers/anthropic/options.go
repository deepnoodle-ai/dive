package anthropic

import "net/http"

type Option func(*Provider)

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

func WithVersion(version string) Option {
	return func(p *Provider) {
		p.version = version
	}
}

func WithMaxTokens(maxTokens int) Option {
	return func(p *Provider) {
		p.maxTokens = maxTokens
	}
}

func WithModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}
