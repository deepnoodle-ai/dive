package openai

import (
	"github.com/deepnoodle-ai/dive/embedding"
	openai "github.com/deepnoodle-ai/dive/llm/providers/openai/embedding"
)

// NewEmbeddingProvider creates a new OpenAI Embedder.
func NewEmbeddingProvider() embedding.Embedder {
	return openai.New()
}
