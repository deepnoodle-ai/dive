package ollama

import (
	"context"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
)

// Generate sends the request to Ollama's Anthropic-compatible endpoint, after
// removing server tool blocks from the history (see withoutServerTools).
func (p *Provider) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return p.Provider.Generate(ctx, withoutServerTools(opts)...)
}

// Stream streams the request from Ollama's Anthropic-compatible endpoint,
// after removing server tool blocks from the history (see
// withoutServerTools).
func (p *Provider) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
	return p.Provider.Stream(ctx, withoutServerTools(opts)...)
}

// withoutServerTools removes tool calls and results that a provider ran on
// its own servers (for example Anthropic web search) from the request
// messages. A session that used them on Anthropic and then switches to
// Ollama would otherwise send blocks that Ollama never produced and cannot
// accept. The assistant text around them still carries what the model
// concluded. The caller's messages are not modified.
func withoutServerTools(opts []llm.Option) []llm.Option {
	config := &llm.Config{}
	config.Apply(opts...)
	filtered := make([]*llm.Message, 0, len(config.Messages))
	changed := false
	for _, message := range config.Messages {
		if message == nil {
			filtered = append(filtered, message)
			continue
		}
		var kept []llm.Content
		for _, content := range message.Content {
			if providers.IsServerToolContent(content) {
				changed = true
				continue
			}
			kept = append(kept, content)
		}
		if len(kept) == len(message.Content) {
			filtered = append(filtered, message)
			continue
		}
		// A message left empty is dropped, except an effort message
		// (llm.NewEffortMessage), which never had content: its Effort is
		// what it carries, and the anthropic encoder decides whether the
		// model takes it.
		if len(kept) == 0 && message.Effort == "" {
			continue
		}
		copied := *message
		copied.Content = kept
		filtered = append(filtered, &copied)
	}
	if !changed {
		return opts
	}
	return append(opts[:len(opts):len(opts)], llm.WithMessages(filtered...))
}
