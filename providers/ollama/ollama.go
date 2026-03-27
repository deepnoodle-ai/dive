package ollama

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
)

var (
	DefaultModel     = "llama3.2:3b"
	DefaultEndpoint  = "http://localhost:11434/v1/messages"
	DefaultMaxTokens = 32768
	DefaultClient    = &http.Client{Timeout: 300 * time.Second}
)

var _ llm.StreamingLLM = &Provider{}

// Provider implements the Ollama LLM provider for local model serving.
// It uses Ollama's Anthropic-compatible Messages API endpoint.
type Provider struct {
	apiKey    string
	endpoint  string
	model     string
	maxTokens int
	client    *http.Client

	// Embedded Anthropic provider
	*anthropic.Provider
}

// New creates a new Ollama provider with the given options.
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
	// Pass the options through to the wrapped Anthropic provider
	p.Provider = anthropic.New(
		anthropic.WithAPIKey(p.apiKey),
		anthropic.WithClient(p.client),
		anthropic.WithEndpoint(p.endpoint),
		anthropic.WithMaxTokens(p.maxTokens),
		anthropic.WithModel(p.model),
	)
	return p
}

func getAPIKey() string {
	if key := os.Getenv("OLLAMA_API_KEY"); key != "" {
		return key
	}
	// Ollama doesn't require an API key for local instances, but
	// the Anthropic-compatible API expects one
	return "ollama"
}

func (p *Provider) Name() string {
	return fmt.Sprintf("ollama-%s", p.model)
}
