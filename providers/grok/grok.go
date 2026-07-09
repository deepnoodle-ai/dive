package grok

import (
	"os"

	"github.com/deepnoodle-ai/dive/llm"
	openaiProvider "github.com/deepnoodle-ai/dive/providers/openai"
)

var (
	DefaultModel     = ModelGrok45
	DefaultEndpoint  = "https://api.x.ai/v1"
	DefaultMaxTokens = 32768
)

var _ llm.StreamingLLM = &Provider{}

// Provider implements the X.AI Grok LLM provider using the Responses API.
type Provider struct {
	// Embedded OpenAI Responses API provider
	*openaiProvider.Provider
}

// New creates a new Grok provider with the given options.
func New(opts ...Option) *Provider {
	cfg := &config{
		apiKey:    getAPIKey(),
		endpoint:  DefaultEndpoint,
		model:     DefaultModel,
		maxTokens: DefaultMaxTokens,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	openaiOpts := []openaiProvider.Option{
		openaiProvider.WithName("grok"),
		openaiProvider.WithAPIKey(cfg.apiKey),
		openaiProvider.WithEndpoint(cfg.endpoint),
		openaiProvider.WithModel(cfg.model),
		openaiProvider.WithMaxTokens(cfg.maxTokens),
	}
	if len(cfg.extraRequestOptions) > 0 {
		openaiOpts = append(openaiOpts,
			openaiProvider.WithExtraRequestOptions(cfg.extraRequestOptions...))
	}
	p := &Provider{
		Provider: openaiProvider.New(openaiOpts...),
	}
	return p
}

func getAPIKey() string {
	if key := os.Getenv("XAI_API_KEY"); key != "" {
		return key
	}
	if key := os.Getenv("GROK_API_KEY"); key != "" {
		return key
	}
	return ""
}

func (p *Provider) Name() string {
	return "grok"
}
