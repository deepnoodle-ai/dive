package dive

import (
	"context"
)

// EmbeddingProvider represents a service that can generate embeddings from text.
type EmbeddingProvider interface {
	// Name returns the name of the embedding provider
	Name() string

	// GenerateEmbedding creates an embedding vector from the input text
	GenerateEmbedding(ctx context.Context, opts ...EmbeddingOption) (*EmbeddingResponse, error)
}

// EmbeddingResponse represents the result of an embedding generation request.
type EmbeddingResponse struct {
	// Embeddings contains the generated embedding vectors
	Embeddings []Embedding `json:"embeddings"`

	// Model is the name of the model used to generate the embeddings
	Model string `json:"model"`

	// Usage contains token usage information
	Usage EmbeddingUsage `json:"usage"`
}

// Embedding represents a single embedding vector.
type Embedding struct {
	// Index is the index of this embedding in the request
	Index int `json:"index"`

	// Vector is the embedding vector (array of floats)
	Vector []float64 `json:"vector"`

	// Object type, always "embedding"
	Object string `json:"object"`
}

// EmbeddingUsage represents token usage for embedding generation.
type EmbeddingUsage struct {
	// PromptTokens is the number of tokens in the input
	PromptTokens int `json:"prompt_tokens"`

	// TotalTokens is the total number of tokens used
	TotalTokens int `json:"total_tokens"`
}

// EmbeddingConfig contains configuration for embedding generation.
type EmbeddingConfig struct {
	// Input text to embed (required)
	Input string

	// Model to use for embedding generation
	Model string

	// EncodingFormat specifies the format of the returned embeddings (float or base64)
	EncodingFormat string

	// Dimensions specifies the number of dimensions for the output embeddings (optional)
	Dimensions *int

	// User identifier for tracking purposes (optional)
	User string
}

// EmbeddingOption is a function that configures embedding generation.
type EmbeddingOption func(*EmbeddingConfig)

// Apply applies the given options to the config.
func (c *EmbeddingConfig) Apply(opts []EmbeddingOption) {
	for _, opt := range opts {
		opt(c)
	}
}

// WithInput sets the input text for embedding generation.
func WithEmbeddingInput(input string) EmbeddingOption {
	return func(c *EmbeddingConfig) {
		c.Input = input
	}
}

// WithEmbeddingModel sets the model for embedding generation.
func WithEmbeddingModel(model string) EmbeddingOption {
	return func(c *EmbeddingConfig) {
		c.Model = model
	}
}

// WithEncodingFormat sets the encoding format for the embeddings.
func WithEncodingFormat(format string) EmbeddingOption {
	return func(c *EmbeddingConfig) {
		c.EncodingFormat = format
	}
}

// WithDimensions sets the number of dimensions for the output embeddings.
func WithDimensions(dimensions int) EmbeddingOption {
	return func(c *EmbeddingConfig) {
		c.Dimensions = &dimensions
	}
}

// WithEmbeddingUser sets the user identifier for tracking purposes.
func WithEmbeddingUser(user string) EmbeddingOption {
	return func(c *EmbeddingConfig) {
		c.User = user
	}
}