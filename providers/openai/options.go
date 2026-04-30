package openai

import (
	"net/http"

	"github.com/openai/openai-go/v3/option"
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

// WithClient sets the HTTP client used for every request to the OpenAI API.
// The constructor flows p.httpClient into the SDK client via
// option.WithHTTPClient — see New.
func WithClient(client *http.Client) Option {
	return func(p *Provider) {
		p.httpClient = client
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

// WithName overrides the provider name returned by Name(). This is used by
// providers that embed the OpenAI provider (e.g., Grok) to ensure the correct
// name is reported in contexts like ToolConfiguration.
func WithName(name string) Option {
	return func(p *Provider) {
		p.name = name
	}
}

// WithExtraRequestOptions adds additional SDK request options that are applied
// to every API call. This can be used to inject extra body fields (e.g., via
// option.WithJSONSet) or custom headers for provider-specific features.
func WithExtraRequestOptions(opts ...option.RequestOption) Option {
	return func(p *Provider) {
		p.extraRequestOptions = append(p.extraRequestOptions, opts...)
	}
}
