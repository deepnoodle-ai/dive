package embedding

import (
	"context"
	"fmt"
)

// Embedder represents a service that can generate embeddings from text.
type Embedder interface {
	// Name returns the name of the embedding provider
	Name() string

	// Embed creates an embedding vector from the input text
	Embed(ctx context.Context, opts ...Option) (*Response, error)
}

// FloatVector represents a single embedding vector of floats.
type FloatVector []float64

// IntVector represents a single embedding vector of integers.
type IntVector []int

// Response represents the result of an embedding generation request.
type Response struct {
	// Floats contains the generated embedding vectors of floats
	Floats []FloatVector `json:"floats,omitempty"`

	// Ints contains the generated embedding vectors of integers
	Ints []IntVector `json:"ints,omitempty"`

	// Model is the name of the model used to generate the embeddings
	Model string `json:"model,omitempty"`

	// Usage contains token usage information
	Usage Usage `json:"usage,omitempty"`

	// Metadata holds any additional metadata about the embedding
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Usage represents token usage for embedding generation.
type Usage struct {
	// PromptTokens is the number of tokens in the input
	PromptTokens int `json:"prompt_tokens"`

	// TotalTokens is the total number of tokens used
	TotalTokens int `json:"total_tokens"`
}

// Config contains configuration for embedding generation.
// Either Input or Inputs is required.
type Config struct {
	// Input text to embed (Input or Inputs is required)
	Input string

	// Inputs to embed, an array of strings (Input or Inputs is required)
	Inputs []string

	// Model to use for embedding generation (optional)
	Model string

	// Dimensions specifies the number of dimensions for the output embeddings (optional)
	Dimensions int

	// User identifier for tracking purposes (optional)
	User string
}

// Option is a function that configures embedding generation.
type Option func(*Config)

// Apply applies the given options to the config.
func (c *Config) Apply(opts []Option) {
	for _, opt := range opts {
		opt(c)
	}
}

// WithInput sets the input text for embedding generation (single string).
func WithInput(input string) Option {
	return func(c *Config) {
		c.Input = input
	}
}

// WithInputs sets multiple input texts for embedding generation (array of strings).
func WithInputs(inputs []string) Option {
	return func(c *Config) {
		c.Inputs = inputs
	}
}

// WithModel sets the model for embedding generation.
func WithModel(model string) Option {
	return func(c *Config) {
		c.Model = model
	}
}

// WithDimensions sets the number of dimensions for the output embeddings.
func WithDimensions(dimensions int) Option {
	return func(c *Config) {
		c.Dimensions = dimensions
	}
}

// WithUser sets the user identifier for tracking purposes.
func WithUser(user string) Option {
	return func(c *Config) {
		c.User = user
	}
}

// ValidateInput validates the input field according to OpenAI's requirements
func (c *Config) ValidateInputs() error {
	if c.Input == "" && len(c.Inputs) == 0 {
		return fmt.Errorf("input is required")
	}
	if c.Input != "" && len(c.Inputs) > 0 {
		return fmt.Errorf("input and inputs cannot both be set")
	}
	if len(c.Inputs) > 2048 {
		return fmt.Errorf("inputs cannot exceed 2048 elements")
	}
	return nil
}

// Validate validates all configuration according to OpenAI's requirements
func (c *Config) Validate() error {
	if err := c.ValidateInputs(); err != nil {
		return err
	}
	if c.Dimensions < 0 {
		return fmt.Errorf("dimensions must not be negative")
	}
	return nil
}

// GetInputAsSlice returns the input as a string slice, converting single strings to single-element slices
func (c *Config) GetInputAsSlice() []string {
	if c.Input != "" {
		return []string{c.Input}
	}
	return c.Inputs
}

// InputCount returns the number of inputs
func (c *Config) InputCount() int {
	if c.Input != "" {
		return 1
	}
	return len(c.Inputs)
}
