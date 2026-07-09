package main

import (
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/dive/providers/grok"

	// Import providers to trigger their init() registration
	_ "github.com/deepnoodle-ai/dive/providers/anthropic"
	_ "github.com/deepnoodle-ai/dive/providers/google"
	_ "github.com/deepnoodle-ai/dive/providers/grok"
	_ "github.com/deepnoodle-ai/dive/providers/mistral"
	_ "github.com/deepnoodle-ai/dive/providers/ollama"
	_ "github.com/deepnoodle-ai/dive/providers/openai"
	_ "github.com/deepnoodle-ai/dive/providers/openaicompletions"
	_ "github.com/deepnoodle-ai/dive/providers/openrouter"
)

// defaultGrokModel is the default model used when a Grok API key is detected.
var defaultGrokModel = grok.ModelGrok45

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
