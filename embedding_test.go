package dive

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddingConfig_Apply(t *testing.T) {
	config := &EmbeddingConfig{}
	
	opts := []EmbeddingOption{
		WithEmbeddingInput("test input"),
		WithEmbeddingModel("text-embedding-ada-002"),
		WithEncodingFormat("float"),
		WithDimensions(1536),
		WithEmbeddingUser("test-user"),
	}
	
	config.Apply(opts)
	
	require.Equal(t, "test input", config.Input)
	require.Equal(t, "text-embedding-ada-002", config.Model)
	require.Equal(t, "float", config.EncodingFormat)
	require.NotNil(t, config.Dimensions)
	require.Equal(t, 1536, *config.Dimensions)
	require.Equal(t, "test-user", config.User)
}

func TestWithEmbeddingInput(t *testing.T) {
	config := &EmbeddingConfig{}
	opt := WithEmbeddingInput("test input")
	opt(config)
	
	require.Equal(t, "test input", config.Input)
}

func TestWithEmbeddingModel(t *testing.T) {
	config := &EmbeddingConfig{}
	opt := WithEmbeddingModel("text-embedding-ada-002")
	opt(config)
	
	require.Equal(t, "text-embedding-ada-002", config.Model)
}

func TestWithEncodingFormat(t *testing.T) {
	config := &EmbeddingConfig{}
	opt := WithEncodingFormat("float")
	opt(config)
	
	require.Equal(t, "float", config.EncodingFormat)
}

func TestWithDimensions(t *testing.T) {
	config := &EmbeddingConfig{}
	opt := WithDimensions(1536)
	opt(config)
	
	require.NotNil(t, config.Dimensions)
	require.Equal(t, 1536, *config.Dimensions)
}

func TestWithEmbeddingUser(t *testing.T) {
	config := &EmbeddingConfig{}
	opt := WithEmbeddingUser("test-user")
	opt(config)
	
	require.Equal(t, "test-user", config.User)
}