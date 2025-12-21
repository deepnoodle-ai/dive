package google

import (
	"time"
)

// Option is a function that configures the Google provider.
type Option func(*Provider)

// WithProjectID sets the Google Cloud project ID.
func WithProjectID(projectID string) Option {
	return func(p *Provider) {
		p.projectID = projectID
	}
}

// WithLocation sets the Google Cloud location/region.
func WithLocation(location string) Option {
	return func(p *Provider) {
		p.location = location
	}
}

// WithModel sets the default model.
func WithModel(model string) Option {
	return func(p *Provider) {
		p.model = model
	}
}

// WithMaxTokens sets the default maximum tokens.
func WithMaxTokens(maxTokens int) Option {
	return func(p *Provider) {
		p.maxTokens = maxTokens
	}
}

// WithMaxRetries sets the maximum number of retries.
func WithMaxRetries(maxRetries int) Option {
	return func(p *Provider) {
		p.maxRetries = maxRetries
	}
}

// WithRetryBaseWait sets the base wait time for retries.
func WithRetryBaseWait(retryBaseWait time.Duration) Option {
	return func(p *Provider) {
		p.retryBaseWait = retryBaseWait
	}
}

// WithVersion sets the API version.
func WithVersion(version string) Option {
	return func(p *Provider) {
		p.version = version
	}
}

// WithAPIKey sets the API key for the provider.
func WithAPIKey(apiKey string) Option {
	return func(p *Provider) {
		p.apiKey = apiKey
	}
}
