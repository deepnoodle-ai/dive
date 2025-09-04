package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/deepnoodle-ai/dive"
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
	provider := NewEmbeddingProvider()
	require.Equal(t, "openai-embeddings", provider.Name())
}

func TestEmbeddingProvider_GenerateEmbedding_Success(t *testing.T) {
	// Mock successful API response
	apiResponse := EmbeddingAPIResponse{
		Object: "list",
		Data: []EmbeddingAPIObject{
			{
				Object:    "embedding",
				Index:     0,
				Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
			},
		},
		Model: "text-embedding-ada-002",
		Usage: EmbeddingAPIUsage{
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

	provider := NewEmbeddingProvider(WithEmbeddingClient(mockClient))
	
	ctx := context.Background()
	response, err := provider.GenerateEmbedding(ctx,
		dive.WithEmbeddingInput("test input"),
		dive.WithEmbeddingModel("text-embedding-ada-002"),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, "text-embedding-ada-002", response.Model)
	require.Len(t, response.Embeddings, 1)
	
	embedding := response.Embeddings[0]
	require.Equal(t, 0, embedding.Index)
	require.Equal(t, "embedding", embedding.Object)
	require.Equal(t, []float64{0.1, 0.2, 0.3, 0.4, 0.5}, embedding.Vector)
	
	require.Equal(t, 5, response.Usage.PromptTokens)
	require.Equal(t, 5, response.Usage.TotalTokens)
}

func TestEmbeddingProvider_GenerateEmbedding_EmptyInput(t *testing.T) {
	provider := NewEmbeddingProvider()
	
	ctx := context.Background()
	_, err := provider.GenerateEmbedding(ctx)

	require.Error(t, err)
	require.Contains(t, err.Error(), "input text is required")
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

	provider := NewEmbeddingProvider(WithEmbeddingClient(mockClient))
	
	ctx := context.Background()
	_, err := provider.GenerateEmbedding(ctx,
		dive.WithEmbeddingInput("test input"),
	)

	require.Error(t, err)
}

func TestEmbeddingProvider_GenerateEmbedding_WithOptions(t *testing.T) {
	// Mock successful API response
	apiResponse := EmbeddingAPIResponse{
		Object: "list",
		Data: []EmbeddingAPIObject{
			{
				Object:    "embedding",
				Index:     0,
				Embedding: []float64{0.1, 0.2, 0.3},
			},
		},
		Model: "text-embedding-3-small",
		Usage: EmbeddingAPIUsage{
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

	provider := NewEmbeddingProvider(WithEmbeddingClient(mockClient))
	
	ctx := context.Background()
	response, err := provider.GenerateEmbedding(ctx,
		dive.WithEmbeddingInput("test input"),
		dive.WithEmbeddingModel("text-embedding-3-small"),
		dive.WithEncodingFormat("float"),
		dive.WithDimensions(512),
		dive.WithEmbeddingUser("test-user"),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, "text-embedding-3-small", response.Model)
	require.Len(t, response.Embeddings, 1)
	require.Equal(t, []float64{0.1, 0.2, 0.3}, response.Embeddings[0].Vector)
}

func TestEmbeddingProviderOptions(t *testing.T) {
	provider := NewEmbeddingProvider(
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