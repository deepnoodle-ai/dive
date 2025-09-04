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
	apiResponse := Response{
		Object: "list",
		Data: []Object{
			{
				Object:    "embedding",
				Index:     0,
				Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
			},
		},
		Model: "text-embedding-ada-002",
		Usage: Usage{
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

	provider := New().WithClient(mockClient)

	ctx := context.Background()
	response, err := provider.Embed(ctx,
		embedding.WithInput("test input"),
		embedding.WithModel("text-embedding-ada-002"),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, "text-embedding-ada-002", response.Model)
	require.Len(t, response.Floats, 1)

	vector := response.Floats[0]
	require.Equal(t, embedding.FloatVector{0.1, 0.2, 0.3, 0.4, 0.5}, vector)

	require.Equal(t, 5, response.Usage.PromptTokens)
	require.Equal(t, 5, response.Usage.TotalTokens)
}

func TestEmbeddingProvider_GenerateEmbedding_EmptyInput(t *testing.T) {
	provider := New()

	ctx := context.Background()
	_, err := provider.Embed(ctx)

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

	provider := New().WithClient(mockClient)

	ctx := context.Background()
	_, err := provider.Embed(ctx,
		embedding.WithInput("test input"),
	)

	require.Error(t, err)
}
