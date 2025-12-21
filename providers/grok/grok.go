package grok

import (
	"net/http"
	"os"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	openaic "github.com/deepnoodle-ai/dive/providers/openaicompletions"
)

var (
	DefaultModel     = ModelGrok41FastReasoning
	DefaultEndpoint  = "https://api.x.ai/v1/chat/completions"
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
	// Pass the options through to the wrapped OpenAI provider
	p.Provider = openaic.New(
		openaic.WithAPIKey(p.apiKey),
		openaic.WithClient(p.client),
		openaic.WithEndpoint(p.endpoint),
		openaic.WithMaxTokens(p.maxTokens),
		openaic.WithModel(p.model),
		openaic.WithSystemRole("system"),
	)
	return p
}

func getAPIKey() string {
	if key := os.Getenv("XAI_API_KEY"); key != "" {
		return key
	}
	// Also check for GROK_API_KEY as an alternative
	if key := os.Getenv("GROK_API_KEY"); key != "" {
		return key
	}
	return ""
}

func (p *Provider) Name() string {
	return "grok"
}
