package openaicompletions

import (
	"net/http"
	"time"
)

// Option is a function that configures the Provider
type Option func(*Provider)

// WithName overrides the provider name used for logging and observability.
// It is intended for providers that embed the Chat Completions adapter.
func WithName(name string) Option {
	return func(p *Provider) {
		p.name = name
	}
}

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

// WithMaxRetries sets the maximum number of retry attempts.
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

// WithSystemRole sets the name of the system role for the provider
func WithSystemRole(systemRole string) Option {
	return func(p *Provider) {
		p.systemRole = systemRole
	}
}

// WithReportedUsageCost trusts the provider's usage.cost field as the
// authoritative account charge, denominated in currency. It is intended for
// OpenAI-compatible gateways such as OpenRouter that document this contract.
func WithReportedUsageCost(currency string) Option {
	return func(p *Provider) {
		p.reportedCostCurrency = currency
		p.reportedCostField = "cost"
	}
}

// WithReportedEstimatedUsageCost uses a provider's usage.estimated_cost field
// as its cost estimate. Missing estimates remain unknown if catalog cost is
// disabled. The amount is labeled as a provider estimate, not a final charge.
func WithReportedEstimatedUsageCost(currency string) Option {
	return func(p *Provider) {
		p.reportedCostCurrency = currency
		p.reportedCostField = "estimated_cost"
	}
}

// WithDisableCatalogCost leaves cost unknown when this endpoint may charge
// differently from another provider serving the same native model ID.
func WithDisableCatalogCost() Option {
	return func(p *Provider) {
		p.disableCatalogCost = true
	}
}

// WithPromptCacheKeySupport forwards llm.WithPromptCacheKey to compatible
// Chat Completions endpoints.
func WithPromptCacheKeySupport() Option {
	return func(p *Provider) { p.supportsPromptCacheKey = true }
}

// WithResponseFormatSupport enables llm.WithResponseFormat for endpoints that
// support the OpenAI Chat Completions response_format request shape.
func WithResponseFormatSupport() Option {
	return func(p *Provider) { p.supportsResponseFormat = true }
}
