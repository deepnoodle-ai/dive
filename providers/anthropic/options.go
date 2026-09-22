package anthropic

import (
	"net/http"
	"time"
)

// Option configures the Anthropic provider.
type Option func(*Provider)

// WithName overrides the provider name used for logging and observability.
// It is intended for providers that embed the Anthropic-compatible adapter.
func WithName(name string) Option {
	return func(p *Provider) {
		p.name = name
	}
}

// WithAPIKey sets the Anthropic API key.
func WithAPIKey(apiKey string) Option {
	return func(p *Provider) {
		p.apiKey = apiKey
	}
}

// WithEndpoint sets the API endpoint URL.
func WithEndpoint(endpoint string) Option {
	return func(p *Provider) {
		p.endpoint = endpoint
	}
}

// WithClient sets the HTTP client.
func WithClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

// WithMaxTokens sets the maximum number of tokens to generate.
func WithMaxTokens(maxTokens int) Option {
	return func(p *Provider) {
		p.maxTokens = maxTokens
	}
}

// WithModel sets the model name.
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

// WithVersion sets the Anthropic API version string.
func WithVersion(version string) Option {
	return func(p *Provider) {
		p.version = version
	}
}

// WithPrefixMismatchBehavior sets thinking.block_binding.prefix_mismatch_behavior
// on every request that configures thinking, and sends the
// thinking-binding-controls beta header. Use PrefixMismatchDropBlock to keep a
// conversation running after an edit invalidates its earlier thinking blocks,
// instead of receiving a 400. Anthropic recommends setting it for the life of a
// session rather than per request.
func WithPrefixMismatchBehavior(behavior PrefixMismatchBehavior) Option {
	return func(p *Provider) {
		p.prefixMismatchBehavior = behavior
	}
}
