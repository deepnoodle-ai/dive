package embedding

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmbeddingConfig_Apply(t *testing.T) {
	config := &Config{}

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

func TestEmbeddingConfig_Apply_WithMultipleInputs(t *testing.T) {
	config := &Config{}

	opts := []EmbeddingOption{
		WithEmbeddingInputs([]string{"input1", "input2", "input3"}),
		WithEmbeddingModel("text-embedding-ada-002"),
		WithEncodingFormat("float"),
		WithDimensions(1536),
		WithEmbeddingUser("test-user"),
	}

	config.Apply(opts)

	require.Equal(t, []string{"input1", "input2", "input3"}, config.Input)
	require.Equal(t, "text-embedding-ada-002", config.Model)
	require.Equal(t, "float", config.EncodingFormat)
	require.NotNil(t, config.Dimensions)
	require.Equal(t, 1536, *config.Dimensions)
	require.Equal(t, "test-user", config.User)
}

func TestWithEmbeddingInput(t *testing.T) {
	config := &Config{}
	opt := WithEmbeddingInput("test input")
	opt(config)

	require.Equal(t, "test input", config.Input)
}

func TestWithEmbeddingModel(t *testing.T) {
	config := &Config{}
	opt := WithEmbeddingModel("text-embedding-ada-002")
	opt(config)

	require.Equal(t, "text-embedding-ada-002", config.Model)
}

func TestWithEncodingFormat(t *testing.T) {
	config := &Config{}
	opt := WithEncodingFormat("float")
	opt(config)

	require.Equal(t, "float", config.EncodingFormat)
}

func TestWithDimensions(t *testing.T) {
	config := &Config{}
	opt := WithDimensions(1536)
	opt(config)

	require.NotNil(t, config.Dimensions)
	require.Equal(t, 1536, *config.Dimensions)
}

func TestWithEmbeddingUser(t *testing.T) {
	config := &Config{}
	opt := WithEmbeddingUser("test-user")
	opt(config)

	require.Equal(t, "test-user", config.User)
}

func TestWithEmbeddingInputs(t *testing.T) {
	config := &Config{}
	opt := WithEmbeddingInputs([]string{"input1", "input2"})
	opt(config)

	require.Equal(t, []string{"input1", "input2"}, config.Input)
}

func TestEmbeddingConfig_ValidateInput_ValidString(t *testing.T) {
	config := &Config{Input: "test input"}

	err := config.ValidateInput()
	require.NoError(t, err)
}

func TestEmbeddingConfig_ValidateInput_ValidArray(t *testing.T) {
	config := &Config{Input: []string{"input1", "input2"}}

	err := config.ValidateInput()
	require.NoError(t, err)
}

func TestEmbeddingConfig_ValidateInput_EmptyString(t *testing.T) {
	config := &Config{Input: ""}

	err := config.ValidateInput()
	require.Error(t, err)
	require.Contains(t, err.Error(), "input string cannot be empty")
}

func TestEmbeddingConfig_ValidateInput_EmptyArray(t *testing.T) {
	config := &Config{Input: []string{}}

	err := config.ValidateInput()
	require.Error(t, err)
	require.Contains(t, err.Error(), "input array cannot be empty")
}

func TestEmbeddingConfig_ValidateInput_ArrayWithEmptyString(t *testing.T) {
	config := &Config{Input: []string{"input1", "", "input3"}}

	err := config.ValidateInput()
	require.Error(t, err)
	require.Contains(t, err.Error(), "input string at index 1 cannot be empty")
}

func TestEmbeddingConfig_ValidateInput_TooManyInputs(t *testing.T) {
	inputs := make([]string, 2049)
	for i := range inputs {
		inputs[i] = "test"
	}
	config := &Config{Input: inputs}

	err := config.ValidateInput()
	require.Error(t, err)
	require.Contains(t, err.Error(), "input array cannot exceed 2048 elements")
}

func TestEmbeddingConfig_ValidateInput_InvalidType(t *testing.T) {
	config := &Config{Input: 123}

	err := config.ValidateInput()
	require.Error(t, err)
	require.Contains(t, err.Error(), "input must be a string or []string")
}

func TestEmbeddingConfig_GetInputAsSlice(t *testing.T) {
	t.Run("string input", func(t *testing.T) {
		config := &Config{Input: "test input"}
		result := config.GetInputAsSlice()
		require.Equal(t, []string{"test input"}, result)
	})

	t.Run("array input", func(t *testing.T) {
		config := &Config{Input: []string{"input1", "input2"}}
		result := config.GetInputAsSlice()
		require.Equal(t, []string{"input1", "input2"}, result)
	})

	t.Run("invalid input", func(t *testing.T) {
		config := &Config{Input: 123}
		result := config.GetInputAsSlice()
		require.Nil(t, result)
	})
}

func TestEmbeddingConfig_Validate_Valid(t *testing.T) {
	config := &Config{
		Input:          "test input",
		Model:          "text-embedding-ada-002",
		EncodingFormat: "float",
		Dimensions:     &[]int{1536}[0],
		User:           "test-user",
	}

	err := config.Validate()
	require.NoError(t, err)
}

func TestEmbeddingConfig_Validate_InvalidEncodingFormat(t *testing.T) {
	config := &Config{
		Input:          "test input",
		EncodingFormat: "invalid",
	}

	err := config.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "encoding_format must be 'float' or 'base64'")
}

func TestEmbeddingConfig_Validate_InvalidDimensions(t *testing.T) {
	dimensions := 0
	config := &Config{
		Input:      "test input",
		Dimensions: &dimensions,
	}

	err := config.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "dimensions must be a positive integer")
}

func TestEmbeddingConfig_Validate_Base64Encoding(t *testing.T) {
	config := &Config{
		Input:          "test input",
		EncodingFormat: "base64",
	}

	err := config.Validate()
	require.NoError(t, err)
}
