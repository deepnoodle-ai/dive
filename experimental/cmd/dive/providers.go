package main

import (
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"

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

// createModel creates an LLM provider using the global registry.
// Providers are registered via init() when imported above.
func createModel(modelName, apiEndpoint string) llm.LLM {
	return providers.CreateModel(modelName, apiEndpoint)
}
