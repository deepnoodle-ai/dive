package groq

import (
	"fmt"
	"net/http"
	"os"

	"github.com/diveagents/dive/llm"
	openaic "github.com/diveagents/dive/llm/providers/openaicompletions"
)

var (
	DefaultModel     = ModelLlama3370bVersatile
	DefaultEndpoint  = "https://api.groq.com/openai/v1/chat/completions"
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
	*openaic.Provider
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:   os.Getenv("GROQ_API_KEY"),
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
	oai := openaic.New(
		openaic.WithAPIKey(p.apiKey),
		openaic.WithClient(p.client),
		openaic.WithEndpoint(p.endpoint),
		openaic.WithMaxTokens(p.maxTokens),
		openaic.WithModel(p.model),
		openaic.WithSystemRole("system"),
		openaic.WithCorePrompt(CorePrompt),
	)
	p.Provider = oai
	return p
}

func (p *Provider) Name() string {
	return fmt.Sprintf("groq-%s", p.model)
}
