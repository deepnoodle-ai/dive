package embedding

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddingConfig_Apply(t *testing.T) {
	config := &Config{}
	opts := []Option{
		WithInput("test-input"),
		WithModel("the-model-name"),
		WithDimensions(1536),
		WithUser("test-user"),
	}
	config.Apply(opts)

	require.Equal(t, "test-input", config.Input)
	require.Equal(t, "the-model-name", config.Model)
	require.NotNil(t, config.Dimensions)
	require.Equal(t, 1536, config.Dimensions)
	require.Equal(t, "test-user", config.User)
}

func TestEmbeddingConfig_Validate_Valid(t *testing.T) {
	config := &Config{
		Input:      "test input",
		Model:      "text-embedding-ada-002",
		Dimensions: 1536,
		User:       "test-user",
	}
	err := config.Validate()
	require.NoError(t, err)
}

func TestEmbeddingConfig_Validate_InvalidEncodingFormat(t *testing.T) {
	config := &Config{
		Input:      "test input",
		Dimensions: -1,
	}
	err := config.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "dimensions must not be negative")
}
