package openai

import (
	"github.com/deepnoodle-ai/dive/embedding"
	openai "github.com/deepnoodle-ai/dive/llm/providers/openai/embedding"
)

// NewEmbeddingProvider creates a new OpenAI embedding provider.
func NewEmbeddingProvider() embedding.EmbeddingProvider {
	return openai.New()
}
