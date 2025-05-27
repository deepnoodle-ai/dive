package ollama

import (
	"fmt"
	"net/http"
	"os"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/openai"
)

var (
	DefaultModel     = "llama3.2"
	DefaultEndpoint  = "http://localhost:11434/v1/chat/completions"
	DefaultMaxTokens = 4096
)

var _ llm.StreamingLLM = &Provider{}

type Provider struct {
	apiKey    string
	endpoint  string
	model     string
	maxTokens int
	client    *http.Client

	// Embedded OpenAI provider
	*openai.Provider
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:   getAPIKey(),
		endpoint: DefaultEndpoint,
		client:   http.DefaultClient,
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.model == "" {
		p.model = DefaultModel
	}
	if p.maxTokens == 0 {
		p.maxTokens = DefaultMaxTokens
	}

	// Pass the options through to the wrapped OpenAI provider
	oai := openai.New(
		openai.WithAPIKey(p.apiKey),
		openai.WithClient(p.client),
		openai.WithEndpoint(p.endpoint),
		openai.WithMaxTokens(p.maxTokens),
		openai.WithModel(p.model),
		openai.WithSystemRole("system"),
	)
	p.Provider = oai
	return p
}

func getAPIKey() string {
	if key := os.Getenv("OLLAMA_API_KEY"); key != "" {
		return key
	}
	// Ollama doesn't require an API key for local instances, but OpenAI-compatible API expects one
	return "ollama"
}

func (p *Provider) Name() string {
	return fmt.Sprintf("ollama-%s", p.model)
}
