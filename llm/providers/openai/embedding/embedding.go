package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/deepnoodle-ai/dive/embedding"
	"github.com/deepnoodle-ai/dive/llm/providers"
	"github.com/deepnoodle-ai/dive/retry"
)

const (
	DefaultEmbeddingModel    = "text-embedding-3-small"
	DefaultEmbeddingEndpoint = "https://api.openai.com/v1/embeddings"
)

var _ embedding.EmbeddingProvider = &EmbeddingProvider{}

// EmbeddingProvider implements the dive.EmbeddingProvider interface for OpenAI embeddings.
type EmbeddingProvider struct {
	client        *http.Client
	apiKey        string
	endpoint      string
	model         string
	maxRetries    int
	retryBaseWait time.Duration
}

// request represents the OpenAI embeddings API request structure.
type request struct {
	Input          interface{} `json:"input"`
	Model          string      `json:"model"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     *int        `json:"dimensions,omitempty"`
	User           string      `json:"user,omitempty"`
}

// response represents the OpenAI embeddings API response structure.
type response struct {
	Object string            `json:"object"`
	Data   []embeddingObject `json:"data"`
	Model  string            `json:"model"`
	Usage  usage             `json:"usage"`
}

// embeddingObject represents a single embedding object from the API.
// The Embedding field can be either []float64 (for "float" encoding) or string (for "base64" encoding).
type embeddingObject struct {
	Object    string      `json:"object"`
	Index     int         `json:"index"`
	Embedding interface{} `json:"embedding"` // Can be []float64 or string
}

// usage represents usage information from the API.
type usage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// EmbeddingOption configures the embedding provider.
type EmbeddingOption func(*EmbeddingProvider)

// New creates a new OpenAI embedding provider.
func New(opts ...EmbeddingOption) *EmbeddingProvider {
	p := &EmbeddingProvider{
		apiKey:        os.Getenv("OPENAI_API_KEY"),
		endpoint:      DefaultEmbeddingEndpoint,
		client:        &http.Client{Timeout: 60 * time.Second},
		model:         DefaultEmbeddingModel,
		maxRetries:    6,
		retryBaseWait: 2 * time.Second,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// WithEmbeddingAPIKey sets the OpenAI API key.
func WithEmbeddingAPIKey(apiKey string) EmbeddingOption {
	return func(p *EmbeddingProvider) {
		p.apiKey = apiKey
	}
}

// WithEmbeddingEndpoint sets the API endpoint.
func WithEmbeddingEndpoint(endpoint string) EmbeddingOption {
	return func(p *EmbeddingProvider) {
		p.endpoint = endpoint
	}
}

// WithEmbeddingDefaultModel sets the default model.
func WithEmbeddingDefaultModel(model string) EmbeddingOption {
	return func(p *EmbeddingProvider) {
		p.model = model
	}
}

// WithEmbeddingClient sets the HTTP client.
func WithEmbeddingClient(client *http.Client) EmbeddingOption {
	return func(p *EmbeddingProvider) {
		p.client = client
	}
}

// WithEmbeddingMaxRetries sets the maximum number of retries.
func WithEmbeddingMaxRetries(maxRetries int) EmbeddingOption {
	return func(p *EmbeddingProvider) {
		p.maxRetries = maxRetries
	}
}

// WithEmbeddingRetryBaseWait sets the base wait time for retries.
func WithEmbeddingRetryBaseWait(baseWait time.Duration) EmbeddingOption {
	return func(p *EmbeddingProvider) {
		p.retryBaseWait = baseWait
	}
}

// Name returns the name of the embedding provider.
func (p *EmbeddingProvider) Name() string {
	return "openai"
}

// GenerateEmbedding creates an embedding vector from the input text.
func (p *EmbeddingProvider) GenerateEmbedding(ctx context.Context, opts ...embedding.EmbeddingOption) (*embedding.Response, error) {
	config := &embedding.Config{}
	config.Apply(opts)

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Set defaults
	model := config.Model
	if model == "" {
		model = p.model
	}

	encodingFormat := config.EncodingFormat
	if encodingFormat == "" {
		encodingFormat = "float"
	}

	// Build request
	request := request{
		Input:          config.Input,
		Model:          model,
		EncodingFormat: encodingFormat,
		Dimensions:     config.Dimensions,
		User:           config.User,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	var result response
	err = retry.Do(ctx, func() error {
		req, err := p.createRequest(ctx, body)
		if err != nil {
			return err
		}

		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("error making request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return providers.NewError(resp.StatusCode, string(body))
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("error decoding response: %w", err)
		}
		return nil
	}, retry.WithMaxRetries(p.maxRetries), retry.WithBaseWait(p.retryBaseWait))

	if err != nil {
		return nil, err
	}

	// Convert API response to dive.EmbeddingResponse
	embeddings := make([]embedding.Embedding, len(result.Data))
	for i, data := range result.Data {
		var vector []float64

		// Handle different encoding formats
		switch embedding := data.Embedding.(type) {
		case []interface{}: // JSON unmarshals []float64 as []interface{}
			vector = make([]float64, len(embedding))
			for j, v := range embedding {
				if f, ok := v.(float64); ok {
					vector[j] = f
				} else {
					return nil, fmt.Errorf("invalid float value in embedding at index %d, position %d", i, j)
				}
			}
		default:
			return nil, fmt.Errorf("unsupported embedding format for index %d: %T", i, data.Embedding)
		}
		embeddings[i] = embedding.Embedding{
			Index:  data.Index,
			Vector: vector,
			Object: data.Object,
		}
	}

	response := &embedding.Response{
		Object: result.Object,
		Data:   embeddings,
		Model:  result.Model,
		Usage: embedding.Usage{
			PromptTokens: result.Usage.PromptTokens,
			TotalTokens:  result.Usage.TotalTokens,
		},
	}
	return response, nil
}

// createRequest creates an HTTP request for the OpenAI embeddings API.
func (p *EmbeddingProvider) createRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}
