package groq

import (
	"fmt"
	"net/http"
	"os"

	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/providers/openai"
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
	*openai.Provider
}

func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:    os.Getenv("GROQ_API_KEY"),
		endpoint:  DefaultEndpoint,
		model:     DefaultModel,
		maxTokens: DefaultMaxTokens,
		client:    http.DefaultClient,
	}
	for _, opt := range opts {
		opt(p)
	}
	// Pass the options through to the wrapped OpenAI provider
	oai := openai.New(
		openai.WithAPIKey(p.apiKey),
		openai.WithClient(p.client),
		openai.WithEndpoint(p.endpoint),
		openai.WithMaxTokens(p.maxTokens),
		openai.WithModel(p.model),
		openai.WithSystemRole("system"),
		openai.WithCorePrompt(CorePrompt),
	)
	p.Provider = oai
	return p
}

func (p *Provider) Name() string {
	return fmt.Sprintf("groq-%s", p.model)
}
