package openaicompletions

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

func WithSystemRole(systemRole string) Option {
	return func(p *Provider) {
		p.systemRole = systemRole
	}
}

func WithCorePrompt(corePrompt string) Option {
	return func(p *Provider) {
		p.corePrompt = corePrompt
	}
}
