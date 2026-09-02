// Package meta implements the Meta Model API provider for Dive, serving the
// Muse Spark family (muse-spark-1.3 and its siblings).
//
// Meta exposes the same models over three wire formats — Responses, Chat
// Completions, and Messages. Dive targets the Responses API because it is the
// only one of the three that carries reasoning across turns for external API
// keys: on Chat Completions the assistant's `reasoning_content` is redacted to
// empty before the response leaves Meta, so every tool-loop turn would re-reason
// from scratch. Agent loops are exactly the workload that regression hurts, so
// this provider embeds the OpenAI Responses provider and points it at
// https://api.meta.ai/v1, inheriting encrypted-reasoning replay unchanged.
package meta

import (
	"os"

	"github.com/deepnoodle-ai/dive/llm"
	openaiProvider "github.com/deepnoodle-ai/dive/providers/openai"
)

var (
	DefaultEndpoint = "https://api.meta.ai/v1"
	// Muse Spark bills reasoning tokens against the same budget as visible
	// output, so this ceiling has to leave room for both.
	DefaultMaxTokens     = 32768
	DefaultMaxRetries    = openaiProvider.DefaultMaxRetries
	DefaultRetryBaseWait = openaiProvider.DefaultRetryBaseWait
)

var _ llm.StreamingLLM = &Provider{}

// Provider implements the Meta Model API provider using the Responses API.
type Provider struct {
	// Embedded OpenAI Responses API provider
	*openaiProvider.Provider
}

// New creates a new Meta provider with the given options.
func New(opts ...Option) *Provider {
	cfg := &config{
		apiKey:        getAPIKey(),
		endpoint:      DefaultEndpoint,
		model:         DefaultModel,
		maxTokens:     DefaultMaxTokens,
		maxRetries:    DefaultMaxRetries,
		retryBaseWait: DefaultRetryBaseWait,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	openaiOpts := []openaiProvider.Option{
		openaiProvider.WithName("meta"),
		// Meta accepts prompt_cache_key on Responses and Chat Completions, and
		// documents it as the replacement for the deprecated `user` field.
		openaiProvider.WithPromptCacheKeySupport(),
		openaiProvider.WithAPIKey(cfg.apiKey),
		openaiProvider.WithEndpoint(cfg.endpoint),
		openaiProvider.WithModel(cfg.model),
		openaiProvider.WithMaxTokens(cfg.maxTokens),
		openaiProvider.WithMaxRetries(cfg.maxRetries),
		openaiProvider.WithRetryBaseWait(cfg.retryBaseWait),
	}
	if cfg.client != nil {
		openaiOpts = append(openaiOpts, openaiProvider.WithClient(cfg.client))
	}
	if len(cfg.extraRequestOptions) > 0 {
		openaiOpts = append(openaiOpts,
			openaiProvider.WithExtraRequestOptions(cfg.extraRequestOptions...))
	}
	return &Provider{Provider: openaiProvider.New(openaiOpts...)}
}

// getAPIKey reads the key Meta's own docs and SDK samples use, MODEL_API_KEY,
// falling back to a namespaced alias for processes that already hold keys for
// several vendors and cannot spend a name as generic as MODEL_API_KEY.
func getAPIKey() string {
	if key := os.Getenv("MODEL_API_KEY"); key != "" {
		return key
	}
	return os.Getenv("META_API_KEY")
}

func (p *Provider) Name() string {
	return "meta"
}
