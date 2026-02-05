package groq

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	openaic "github.com/deepnoodle-ai/dive/providers/openaicompletions"
)

var (
	DefaultModel     = ModelLlama3370bVersatile
	DefaultEndpoint  = "https://api.groq.com/openai/v1/chat/completions"
	DefaultMaxTokens = 4096
	DefaultClient    = &http.Client{Timeout: 300 * time.Second}
)

var _ llm.StreamingLLM = &Provider{}

// Provider implements the Groq inference engine LLM provider.
type Provider struct {
	apiKey    string
	endpoint  string
	model     string
	maxTokens int
	client    *http.Client

	// Embedded OpenAI completions provider
	*openaic.Provider
}

// New creates a new Groq provider with the given options.
func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:    os.Getenv("GROQ_API_KEY"),
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

func (p *Provider) Name() string {
	return fmt.Sprintf("groq-%s", p.model)
}
