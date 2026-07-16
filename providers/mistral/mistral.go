package mistral

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	openaic "github.com/deepnoodle-ai/dive/providers/openaicompletions"
)

var (
	DefaultModel         = ModelMistralLarge3
	DefaultEndpoint      = "https://api.mistral.ai/v1/chat/completions"
	DefaultMaxTokens     = 32768
	DefaultMaxRetries    = openaic.DefaultMaxRetries
	DefaultRetryBaseWait = openaic.DefaultRetryBaseWait
	DefaultClient        = &http.Client{Timeout: 300 * time.Second}
)

var _ llm.StreamingLLM = &Provider{}

// Provider implements the Mistral LLM provider.
type Provider struct {
	apiKey        string
	endpoint      string
	model         string
	maxTokens     int
	maxRetries    int
	retryBaseWait time.Duration
	client        *http.Client

	// Embedded OpenAI completions provider
	*openaic.Provider
}

// New creates a new Mistral provider with the given options.
func New(opts ...Option) *Provider {
	p := &Provider{
		apiKey:        os.Getenv("MISTRAL_API_KEY"),
		endpoint:      DefaultEndpoint,
		client:        DefaultClient,
		model:         DefaultModel,
		maxTokens:     DefaultMaxTokens,
		maxRetries:    DefaultMaxRetries,
		retryBaseWait: DefaultRetryBaseWait,
	}
	for _, opt := range opts {
		opt(p)
	}
	// Pass the options through to the wrapped OpenAI provider
	p.Provider = openaic.New(
		openaic.WithName(fmt.Sprintf("mistral-%s", p.model)),
		openaic.WithAPIKey(p.apiKey),
		openaic.WithClient(p.client),
		openaic.WithEndpoint(p.endpoint),
		openaic.WithMaxTokens(p.maxTokens),
		openaic.WithMaxRetries(p.maxRetries),
		openaic.WithBaseWait(p.retryBaseWait),
		openaic.WithModel(p.model),
		openaic.WithSystemRole("system"),
	)
	return p
}

func (p *Provider) Name() string {
	return fmt.Sprintf("mistral-%s", p.model)
}
