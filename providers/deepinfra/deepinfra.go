// Package deepinfra provides DeepInfra's OpenAI-compatible Chat Completions API.
package deepinfra

import (
	"net/http"
	"os"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	openaic "github.com/deepnoodle-ai/dive/providers/openaicompletions"
)

const DefaultEndpoint = "https://api.deepinfra.com/v1/openai/chat/completions"

var _ llm.StreamingLLM = (*Provider)(nil)

// Provider uses DeepInfra's model IDs and credentials with Dive's shared Chat
// Completions implementation. Model IDs are passed through unchanged.
type Provider struct {
	*openaic.Provider
}

type config struct {
	apiKey     string
	endpoint   string
	model      string
	client     *http.Client
	maxTokens  int
	maxRetries int
	baseWait   time.Duration
}

// New creates a DeepInfra provider. The CLI and registry select it with the
// "deepinfra/" prefix; library callers pass native DeepInfra model IDs.
func New(opts ...Option) *Provider {
	c := config{
		apiKey:     APIKey(),
		endpoint:   DefaultEndpoint,
		model:      DefaultModel,
		client:     openaic.DefaultClient,
		maxTokens:  openaic.DefaultMaxTokens,
		maxRetries: openaic.DefaultMaxRetries,
		baseWait:   openaic.DefaultRetryBaseWait,
	}
	for _, opt := range opts {
		opt(&c)
	}
	return &Provider{Provider: openaic.New(
		openaic.WithName("deepinfra"),
		openaic.WithAPIKey(c.apiKey),
		openaic.WithEndpoint(c.endpoint),
		openaic.WithModel(c.model),
		openaic.WithSystemRole("system"),
		openaic.WithClient(c.client),
		openaic.WithMaxTokens(c.maxTokens),
		openaic.WithMaxRetries(c.maxRetries),
		openaic.WithBaseWait(c.baseWait),
		openaic.WithReportedEstimatedUsageCost("USD"),
		openaic.WithDisableCatalogCost(),
		openaic.WithPromptCacheKeySupport(),
		openaic.WithResponseFormatSupport(),
	)}
}

// APIKey returns the first configured DeepInfra credential. The first name
// matches Dive's usual provider convention; the other names are common in
// DeepInfra examples and integrations.
func APIKey() string {
	for _, name := range []string{"DEEP_INFRA_API_KEY", "DEEPINFRA_API_KEY", "DEEPINFRA_TOKEN"} {
		if key := os.Getenv(name); key != "" {
			return key
		}
	}
	return ""
}
