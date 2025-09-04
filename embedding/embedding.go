package embedding

import (
	"context"
	"fmt"
)

// EmbeddingProvider represents a service that can generate embeddings from text.
type EmbeddingProvider interface {
	// Name returns the name of the embedding provider
	Name() string

	// GenerateEmbedding creates an embedding vector from the input text
	GenerateEmbedding(ctx context.Context, opts ...EmbeddingOption) (*Response, error)
}

// Response represents the result of an embedding generation request.
type Response struct {
	// Object type, always "list"
	Object string `json:"object"`

	// Data contains the generated embedding vectors (renamed from Embeddings for OpenAI compatibility)
	Data []Embedding `json:"data"`

	// Model is the name of the model used to generate the embeddings
	Model string `json:"model"`

	// Usage contains token usage information
	Usage Usage `json:"usage"`
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

// Usage represents token usage for embedding generation.
type Usage struct {
	// PromptTokens is the number of tokens in the input
	PromptTokens int `json:"prompt_tokens"`

	// TotalTokens is the total number of tokens used
	TotalTokens int `json:"total_tokens"`
}

// Config contains configuration for embedding generation.
type Config struct {
	// Input text to embed, either a string or array of strings (required)
	Input interface{} // Can be string or []string

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
type EmbeddingOption func(*Config)

// Apply applies the given options to the config.
func (c *Config) Apply(opts []EmbeddingOption) {
	for _, opt := range opts {
		opt(c)
	}
}

// WithEmbeddingInput sets the input text for embedding generation (single string).
func WithEmbeddingInput(input string) EmbeddingOption {
	return func(c *Config) {
		c.Input = input
	}
}

// WithEmbeddingInputs sets multiple input texts for embedding generation (array of strings).
func WithEmbeddingInputs(inputs []string) EmbeddingOption {
	return func(c *Config) {
		c.Input = inputs
	}
}

// WithEmbeddingModel sets the model for embedding generation.
func WithEmbeddingModel(model string) EmbeddingOption {
	return func(c *Config) {
		c.Model = model
	}
}

// WithEncodingFormat sets the encoding format for the embeddings.
func WithEncodingFormat(format string) EmbeddingOption {
	return func(c *Config) {
		c.EncodingFormat = format
	}
}

// WithDimensions sets the number of dimensions for the output embeddings.
func WithDimensions(dimensions int) EmbeddingOption {
	return func(c *Config) {
		c.Dimensions = &dimensions
	}
}

// WithEmbeddingUser sets the user identifier for tracking purposes.
func WithEmbeddingUser(user string) EmbeddingOption {
	return func(c *Config) {
		c.User = user
	}
}

// ValidateInput validates the input field according to OpenAI's requirements
func (c *Config) ValidateInput() error {
	if c.Input == nil {
		return fmt.Errorf("input is required")
	}

	switch input := c.Input.(type) {
	case string:
		if input == "" {
			return fmt.Errorf("input string cannot be empty")
		}
		// Note: OpenAI has a per-input token limit of 8192 tokens, but we can't easily validate that here
		// without tokenization. The API will handle this validation.
	case []string:
		if len(input) == 0 {
			return fmt.Errorf("input array cannot be empty")
		}
		for i, s := range input {
			if s == "" {
				return fmt.Errorf("input string at index %d cannot be empty", i)
			}
		}
		if len(input) > 2048 {
			return fmt.Errorf("input array cannot exceed 2048 elements")
		}
	default:
		return fmt.Errorf("input must be a string or []string")
	}

	return nil
}

// Validate validates all configuration according to OpenAI's requirements
func (c *Config) Validate() error {
	if err := c.ValidateInput(); err != nil {
		return err
	}

	// Validate encoding format
	if c.EncodingFormat != "" && c.EncodingFormat != "float" && c.EncodingFormat != "base64" {
		return fmt.Errorf("encoding_format must be 'float' or 'base64'")
	}

	// Validate dimensions (only for text-embedding-3 and later models, but we allow it for all)
	if c.Dimensions != nil && *c.Dimensions <= 0 {
		return fmt.Errorf("dimensions must be a positive integer")
	}

	return nil
}

// GetInputAsSlice returns the input as a string slice, converting single strings to single-element slices
func (c *Config) GetInputAsSlice() []string {
	switch input := c.Input.(type) {
	case string:
		return []string{input}
	case []string:
		return input
	default:
		return nil
	}
}
