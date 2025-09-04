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

var (
	DefaultModel    = "text-embedding-3-small"
	DefaultEndpoint = "https://api.openai.com/v1/embeddings"
)

var _ embedding.Embedder = &Embedder{}

// Embedder implements the dive.Embedder interface for OpenAI embeddings.
type Embedder struct {
	client        *http.Client
	endpoint      string
	apiKey        string
	maxRetries    int
	retryBaseWait time.Duration
}

// Request represents the OpenAI embeddings API request structure.
type Request struct {
	Input      any    `json:"input"`
	Model      string `json:"model"`
	Dimensions *int   `json:"dimensions,omitempty"`
	User       string `json:"user,omitempty"`
}

// Response represents the OpenAI embeddings API response structure.
type Response struct {
	Object string   `json:"object"`
	Data   []Object `json:"data"`
	Model  string   `json:"model"`
	Usage  Usage    `json:"usage"`
}

// Object represents a single embedding object from the API.
type Object struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// Usage represents usage information from the API.
type Usage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// New creates a new OpenAI embedding provider.
func New() *Embedder {
	return &Embedder{
		client:        &http.Client{Timeout: 10 * time.Second},
		endpoint:      DefaultEndpoint,
		apiKey:        os.Getenv("OPENAI_API_KEY"),
		maxRetries:    3,
		retryBaseWait: 200 * time.Millisecond,
	}
}

// WithAPIKey sets the API key for the embedding provider.
func (p *Embedder) WithAPIKey(apiKey string) *Embedder {
	p.apiKey = apiKey
	return p
}

// WithEndpoint overrides the default endpoint for the embedding provider.
func (p *Embedder) WithEndpoint(endpoint string) *Embedder {
	p.endpoint = endpoint
	return p
}

// WithClient overrides the default HTTP client for the embedding provider.
func (p *Embedder) WithClient(client *http.Client) *Embedder {
	p.client = client
	return p
}

// Name returns the name of the embedding provider.
func (p *Embedder) Name() string {
	return "openai"
}

// Embed creates an embedding vector from the input text.
func (p *Embedder) Embed(ctx context.Context, opts ...embedding.Option) (*embedding.Response, error) {
	config := &embedding.Config{}
	config.Apply(opts)

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Set defaults
	model := config.Model
	if model == "" {
		model = DefaultModel
	}

	// Build Request
	body, err := json.Marshal(Request{
		Input:      config.GetInputAsSlice(),
		Model:      model,
		Dimensions: &config.Dimensions,
		User:       config.User,
	})
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	var result Response
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

	embeddings := make([]embedding.FloatVector, len(result.Data))
	for i, data := range result.Data {
		embeddings[i] = data.Embedding
	}
	response := &embedding.Response{
		Floats: embeddings,
		Model:  result.Model,
		Usage: embedding.Usage{
			PromptTokens: result.Usage.PromptTokens,
			TotalTokens:  result.Usage.TotalTokens,
		},
		Metadata: map[string]any{
			"provider": p.Name(),
		},
	}
	return response, nil
}

// createRequest creates an HTTP request for the OpenAI embeddings API.
func (p *Embedder) createRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
