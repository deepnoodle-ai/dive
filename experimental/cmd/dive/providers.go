package main

import (
	"context"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/dive/providers/grok"
	"github.com/deepnoodle-ai/dive/providers/meta"
	"github.com/deepnoodle-ai/dive/providers/mistral"

	// Import providers to trigger their init() registration
	_ "github.com/deepnoodle-ai/dive/providers/anthropic"
	_ "github.com/deepnoodle-ai/dive/providers/google"
	_ "github.com/deepnoodle-ai/dive/providers/grok"
	_ "github.com/deepnoodle-ai/dive/providers/meta"
	_ "github.com/deepnoodle-ai/dive/providers/mistral"
	_ "github.com/deepnoodle-ai/dive/providers/ollama"
	_ "github.com/deepnoodle-ai/dive/providers/openai"
	_ "github.com/deepnoodle-ai/dive/providers/openaicompletions"
	_ "github.com/deepnoodle-ai/dive/providers/openrouter"
)

// defaultGrokModel is the default model used when a Grok API key is detected.
var defaultGrokModel = grok.DefaultModel

// defaultMistralModel is the default model used when a Mistral API key is detected.
var defaultMistralModel = mistral.DefaultModel

// defaultMetaModel is the default model used when a Meta Model API key is detected.
var defaultMetaModel = meta.DefaultModel

// createModel creates an LLM provider using the global registry.
// Providers are registered via init() when imported above.
func createModel(modelName, apiEndpoint string) llm.LLM {
	return providers.CreateModel(modelName, apiEndpoint)
}

// grokServerSideTools returns the Grok server-side tools (web search, X search)
// if the model is a Grok model, or nil otherwise.
func grokServerSideTools(modelName string) []dive.Tool {
	if !strings.HasPrefix(modelName, "grok-") {
		return nil
	}
	var tools []dive.Tool
	if ws, err := grok.NewWebSearchTool(grok.WebSearchToolOptions{}); err == nil {
		tools = append(tools, ws)
	}
	if xs, err := grok.NewXSearchTool(grok.XSearchToolOptions{}); err == nil {
		tools = append(tools, xs)
	}
	return tools
}

// providerServerToolset resolves provider-native tools for the current model
// before every request. The tool instances are cached; only their visibility
// changes. This keeps /model switches from retaining tools that the new
// provider cannot accept or omitting tools that it supports.
func providerServerToolset(modelName func() string) dive.Toolset {
	grokTools := grokServerSideTools("grok-")
	return &dive.ToolsetFunc{
		ToolsetName: "provider-server-tools",
		Resolve: func(context.Context) ([]dive.Tool, error) {
			if modelName != nil && strings.HasPrefix(modelName(), "grok-") {
				return grokTools, nil
			}
			return nil, nil
		},
	}
}
