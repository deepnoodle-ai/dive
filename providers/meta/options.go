package meta

import (
	"net/http"
	"time"

	"github.com/openai/openai-go/v3/option"
)

// config holds the settings an Option can set before the embedded OpenAI
// Responses provider is constructed.
type config struct {
	apiKey              string
	endpoint            string
	model               string
	maxTokens           int
	maxRetries          int
	retryBaseWait       time.Duration
	client              *http.Client
	extraRequestOptions []option.RequestOption
}

// Option is a function that configures the Provider.
type Option func(*config)

// WithAPIKey sets the Meta Model API key.
func WithAPIKey(apiKey string) Option {
	return func(c *config) { c.apiKey = apiKey }
}

// WithEndpoint sets the API endpoint URL.
func WithEndpoint(endpoint string) Option {
	return func(c *config) { c.endpoint = endpoint }
}

// WithModel sets the model name.
func WithModel(model string) Option {
	return func(c *config) { c.model = model }
}

// WithMaxTokens sets the maximum number of tokens to generate. On Muse Spark
// this budget covers reasoning tokens plus visible output, so a low ceiling
// truncates the visible answer rather than the thinking.
func WithMaxTokens(maxTokens int) Option {
	return func(c *config) { c.maxTokens = maxTokens }
}

// WithMaxRetries sets the maximum number of retries for transient generation
// failures (total attempts = maxRetries + 1).
func WithMaxRetries(maxRetries int) Option {
	return func(c *config) { c.maxRetries = maxRetries }
}

// WithRetryBaseWait sets the base wait duration between retries.
func WithRetryBaseWait(retryBaseWait time.Duration) Option {
	return func(c *config) { c.retryBaseWait = retryBaseWait }
}

// WithClient sets the HTTP client used for all API requests.
func WithClient(client *http.Client) Option {
	return func(c *config) { c.client = client }
}

// WithExtraRequestOptions adds additional SDK request options applied to every
// API call, for injecting body fields or headers Dive does not model natively.
func WithExtraRequestOptions(opts ...option.RequestOption) Option {
	return func(c *config) { c.extraRequestOptions = append(c.extraRequestOptions, opts...) }
}
