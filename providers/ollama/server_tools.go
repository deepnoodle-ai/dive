package ollama

import (
	"context"

	"github.com/deepnoodle-ai/dive/llm"
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
			if isServerToolContent(content) {
				changed = true
				continue
			}
			kept = append(kept, content)
		}
		if len(kept) == len(message.Content) {
			filtered = append(filtered, message)
			continue
		}
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

func isServerToolContent(content llm.Content) bool {
	switch content.(type) {
	case *llm.ServerToolUseContent, *llm.WebSearchToolResultContent,
		*llm.CodeExecutionToolResultContent, *llm.BashCodeExecutionToolResultContent,
		*llm.TextEditorCodeExecutionToolResultContent,
		*llm.MCPToolUseContent, *llm.MCPToolResultContent:
		return true
	}
	return false
}
