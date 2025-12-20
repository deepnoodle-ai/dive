package openrouter

import (
	"net/http"
	"os"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	openaic "github.com/deepnoodle-ai/dive/providers/openaicompletions"
)

var (
	DefaultModel     = ModelClaudeOpus45
	DefaultEndpoint  = "https://openrouter.ai/api/v1/chat/completions"
	DefaultMaxTokens = 4096
	DefaultClient    = &http.Client{Timeout: 300 * time.Second}
)

var _ llm.StreamingLLM = &Provider{}

type Provider struct {
	apiKey    string
	endpoint  string
	model     string
	maxTokens int
	client    *http.Client
	siteURL   string
	siteName  string

	// Embedded OpenAI completions provider
	*openaic.Provider
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:    getAPIKey(),
		endpoint:  DefaultEndpoint,
		client:    DefaultClient,
		model:     DefaultModel,
		maxTokens: DefaultMaxTokens,
	}
	for _, opt := range opts {
		opt(p)
	}

	if p.siteURL == "" {
		p.siteURL = "https://deepnoodle.ai"
	}
	if p.siteName == "" {
		p.siteName = "Deep Noodle"
	}

	// Create a custom client that adds OpenRouter-specific headers
	customClient := &http.Client{
		Timeout: p.client.Timeout,
		Transport: &openRouterTransport{
			underlying: p.client.Transport,
			siteURL:    p.siteURL,
			siteName:   p.siteName,
		},
	}

	// Pass the options through to the wrapped OpenAI provider
	p.Provider = openaic.New(
		openaic.WithAPIKey(p.apiKey),
		openaic.WithClient(customClient),
		openaic.WithEndpoint(p.endpoint),
		openaic.WithMaxTokens(p.maxTokens),
		openaic.WithModel(p.model),
		openaic.WithSystemRole("system"),
	)
	return p
}

func getAPIKey() string {
	if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
		return key
	}
	return ""
}

func (p *Provider) Name() string {
	return "openrouter"
}

// openRouterTransport is a custom http.RoundTripper that adds OpenRouter-specific headers
type openRouterTransport struct {
	underlying http.RoundTripper
	siteURL    string
	siteName   string
}

func (t *openRouterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add OpenRouter-specific headers
	if t.siteURL != "" {
		req.Header.Set("HTTP-Referer", t.siteURL)
	}
	if t.siteName != "" {
		req.Header.Set("X-Title", t.siteName)
	}

	// Use the underlying transport or default if none provided
	transport := t.underlying
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(req)
}
