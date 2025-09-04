package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/deepnoodle-ai/dive/embedding"
	"github.com/stretchr/testify/require"
)

// MockRoundTripper is a mock HTTP transport for testing
type MockRoundTripper struct {
	Response *http.Response
	Err      error
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.Response, m.Err
}

func TestEmbeddingProvider_Name(t *testing.T) {
	provider := New()
	require.Equal(t, "openai", provider.Name())
}

func TestEmbeddingProvider_GenerateEmbedding_Success(t *testing.T) {
	// Mock successful API response
	apiResponse := response{
		Object: "list",
		Data: []embeddingObject{
			{
				Object:    "embedding",
				Index:     0,
				Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
			},
		},
		Model: "text-embedding-ada-002",
		Usage: usage{
			PromptTokens: 5,
			TotalTokens:  5,
		},
	}

	responseBody, err := json.Marshal(apiResponse)
	require.NoError(t, err)

	mockClient := &http.Client{
		Transport: &MockRoundTripper{
			Response: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(responseBody)),
				Header:     make(http.Header),
			},
		},
	}

	provider := New(WithEmbeddingClient(mockClient))

	ctx := context.Background()
	response, err := provider.GenerateEmbedding(ctx,
		embedding.WithEmbeddingInput("test input"),
		embedding.WithEmbeddingModel("text-embedding-ada-002"),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, "list", response.Object)
	require.Equal(t, "text-embedding-ada-002", response.Model)
	require.Len(t, response.Data, 1)

	embedding := response.Data[0]
	require.Equal(t, 0, embedding.Index)
	require.Equal(t, "embedding", embedding.Object)
	require.Equal(t, []float64{0.1, 0.2, 0.3, 0.4, 0.5}, embedding.Vector)

	require.Equal(t, 5, response.Usage.PromptTokens)
	require.Equal(t, 5, response.Usage.TotalTokens)
}

func TestEmbeddingProvider_GenerateEmbedding_EmptyInput(t *testing.T) {
	provider := New()

	ctx := context.Background()
	_, err := provider.GenerateEmbedding(ctx)

	require.Error(t, err)
	require.Contains(t, err.Error(), "input is required")
}

func TestEmbeddingProvider_GenerateEmbedding_HTTPError(t *testing.T) {
	mockClient := &http.Client{
		Transport: &MockRoundTripper{
			Response: &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error": "Bad request"}`))),
				Header:     make(http.Header),
			},
		},
	}

	provider := New(WithEmbeddingClient(mockClient))

	ctx := context.Background()
	_, err := provider.GenerateEmbedding(ctx,
		embedding.WithEmbeddingInput("test input"),
	)

	require.Error(t, err)
}

func TestEmbeddingProvider_GenerateEmbedding_WithOptions(t *testing.T) {
	// Mock successful API response
	apiResponse := response{
		Object: "list",
		Data: []embeddingObject{
			{
				Object:    "embedding",
				Index:     0,
				Embedding: []float64{0.1, 0.2, 0.3},
			},
		},
		Model: "text-embedding-3-small",
		Usage: usage{
			PromptTokens: 3,
			TotalTokens:  3,
		},
	}

	responseBody, err := json.Marshal(apiResponse)
	require.NoError(t, err)

	mockClient := &http.Client{
		Transport: &MockRoundTripper{
			Response: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(responseBody)),
				Header:     make(http.Header),
			},
		},
	}

	provider := New(WithEmbeddingClient(mockClient))

	ctx := context.Background()
	response, err := provider.GenerateEmbedding(ctx,
		embedding.WithEmbeddingInput("test input"),
		embedding.WithEmbeddingModel("text-embedding-3-small"),
		embedding.WithEncodingFormat("float"),
		embedding.WithDimensions(512),
		embedding.WithEmbeddingUser("test-user"),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, "list", response.Object)
	require.Equal(t, "text-embedding-3-small", response.Model)
	require.Len(t, response.Data, 1)
	require.Equal(t, []float64{0.1, 0.2, 0.3}, response.Data[0].Vector)
}

func TestEmbeddingProviderOptions(t *testing.T) {
	provider := New(
		WithEmbeddingAPIKey("test-key"),
		WithEmbeddingEndpoint("https://test.example.com/v1/embeddings"),
		WithEmbeddingDefaultModel("custom-model"),
		WithEmbeddingMaxRetries(10),
	)

	require.Equal(t, "test-key", provider.apiKey)
	require.Equal(t, "https://test.example.com/v1/embeddings", provider.endpoint)
	require.Equal(t, "custom-model", provider.model)
	require.Equal(t, 10, provider.maxRetries)
}

func TestEmbeddingProvider_GenerateEmbedding_MultipleInputs(t *testing.T) {
	// Mock successful API response for multiple inputs
	apiResponse := response{
		Object: "list",
		Data: []embeddingObject{
			{
				Object:    "embedding",
				Index:     0,
				Embedding: []float64{0.1, 0.2, 0.3},
			},
			{
				Object:    "embedding",
				Index:     1,
				Embedding: []float64{0.4, 0.5, 0.6},
			},
		},
		Model: "text-embedding-ada-002",
		Usage: usage{
			PromptTokens: 8,
			TotalTokens:  8,
		},
	}

	responseBody, err := json.Marshal(apiResponse)
	require.NoError(t, err)

	mockClient := &http.Client{
		Transport: &MockRoundTripper{
			Response: &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(responseBody)),
				Header:     make(http.Header),
			},
		},
	}

	provider := New(WithEmbeddingClient(mockClient))

	ctx := context.Background()
	response, err := provider.GenerateEmbedding(ctx,
		embedding.WithEmbeddingInputs([]string{"first input", "second input"}),
		embedding.WithEmbeddingModel("text-embedding-ada-002"),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, "list", response.Object)
	require.Equal(t, "text-embedding-ada-002", response.Model)
	require.Len(t, response.Data, 2)

	require.Equal(t, 0, response.Data[0].Index)
	require.Equal(t, []float64{0.1, 0.2, 0.3}, response.Data[0].Vector)
	require.Equal(t, 1, response.Data[1].Index)
	require.Equal(t, []float64{0.4, 0.5, 0.6}, response.Data[1].Vector)
}

func TestEmbeddingProvider_GenerateEmbedding_EmptyInputs(t *testing.T) {
	provider := New()

	ctx := context.Background()
	_, err := provider.GenerateEmbedding(ctx, embedding.WithEmbeddingInputs([]string{}))

	require.Error(t, err)
	require.Contains(t, err.Error(), "input array cannot be empty")
}

func TestEmbeddingProvider_GenerateEmbedding_TooManyInputs(t *testing.T) {
	provider := New()

	// Create 2049 inputs (exceeds OpenAI's limit)
	inputs := make([]string, 2049)
	for i := range inputs {
		inputs[i] = "test input"
	}

	ctx := context.Background()
	_, err := provider.GenerateEmbedding(ctx, embedding.WithEmbeddingInputs(inputs))

	require.Error(t, err)
	require.Contains(t, err.Error(), "input array cannot exceed 2048 elements")
}

func TestEmbeddingProvider_GenerateEmbedding_InvalidEncodingFormat(t *testing.T) {
	provider := New()

	ctx := context.Background()
	_, err := provider.GenerateEmbedding(ctx,
		embedding.WithEmbeddingInput("test input"),
		embedding.WithEncodingFormat("invalid"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "encoding_format must be 'float' or 'base64'")
}

func TestEmbeddingProvider_GenerateEmbedding_InvalidDimensions(t *testing.T) {
	provider := New()

	ctx := context.Background()
	_, err := provider.GenerateEmbedding(ctx,
		embedding.WithEmbeddingInput("test input"),
		embedding.WithDimensions(0))

	require.Error(t, err)
	require.Contains(t, err.Error(), "dimensions must be a positive integer")
}
