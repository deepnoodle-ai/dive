package google

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/deepnoodle-ai/dive/embedding"
	"github.com/deepnoodle-ai/dive/retry"
	"google.golang.org/genai"
)

var (
	DefaultModel      = "text-embedding-004"
	DefaultTaskType   = "RETRIEVAL_DOCUMENT"
	DefaultMaxRetries = 3
	DefaultRetryWait  = 200 * time.Millisecond
)

var _ embedding.Embedder = &Embedder{}

// Embedder implements the embedding.Embedder interface for Google AI embeddings.
type Embedder struct {
	client        *genai.Client
	apiKey        string
	projectID     string
	location      string
	maxRetries    int
	retryBaseWait time.Duration
	taskType      string
}

// New creates a new Google embedding provider.
func New() *Embedder {
	// Try to get API key from environment
	var apiKey string
	if value := os.Getenv("GEMINI_API_KEY"); value != "" {
		apiKey = value
	} else if value := os.Getenv("GOOGLE_API_KEY"); value != "" {
		apiKey = value
	}

	return &Embedder{
		apiKey:        apiKey,
		projectID:     os.Getenv("GOOGLE_PROJECT_ID"),
		location:      os.Getenv("GOOGLE_LOCATION"),
		maxRetries:    DefaultMaxRetries,
		retryBaseWait: DefaultRetryWait,
		taskType:      DefaultTaskType,
	}
}

// WithAPIKey sets the API key for the embedding provider.
func (e *Embedder) WithAPIKey(apiKey string) *Embedder {
	e.apiKey = apiKey
	return e
}

// WithProjectID sets the Google Cloud project ID for Vertex AI.
func (e *Embedder) WithProjectID(projectID string) *Embedder {
	e.projectID = projectID
	return e
}

// WithLocation sets the Google Cloud location for Vertex AI.
func (e *Embedder) WithLocation(location string) *Embedder {
	e.location = location
	return e
}

// WithClient sets a pre-configured genai client.
func (e *Embedder) WithClient(client *genai.Client) *Embedder {
	e.client = client
	return e
}

// WithTaskType sets the task type for embedding generation.
// Valid values are "RETRIEVAL_QUERY", "RETRIEVAL_DOCUMENT", "SEMANTIC_SIMILARITY", "CLASSIFICATION", "CLUSTERING"
func (e *Embedder) WithTaskType(taskType string) *Embedder {
	e.taskType = taskType
	return e
}

// WithMaxRetries sets the maximum number of retries for failed requests.
func (e *Embedder) WithMaxRetries(maxRetries int) *Embedder {
	e.maxRetries = maxRetries
	return e
}

// WithRetryBaseWait sets the base wait time between retries.
func (e *Embedder) WithRetryBaseWait(wait time.Duration) *Embedder {
	e.retryBaseWait = wait
	return e
}

// Name returns the name of the embedding provider.
func (e *Embedder) Name() string {
	return "google"
}

// Embed creates an embedding vector from the input text.
func (e *Embedder) Embed(ctx context.Context, opts ...embedding.Option) (*embedding.Response, error) {
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

	// Initialize client if not already set
	if e.client == nil {
		if err := e.initClient(ctx); err != nil {
			return nil, fmt.Errorf("failed to initialize client: %w", err)
		}
	}

	// Process inputs
	inputs := config.GetInputAsSlice()
	var embeddings []embedding.FloatVector
	var totalPromptTokens int

	// Google's EmbedContent API processes one input at a time
	// We need to make separate calls for each input
	for _, input := range inputs {
		var result *genai.EmbedContentResponse
		err := retry.Do(ctx, func() error {
			embedConfig := &genai.EmbedContentConfig{
				TaskType: e.taskType,
			}

			// Handle dimensions if specified
			if config.Dimensions > 0 {
				dim := int32(config.Dimensions)
				embedConfig.OutputDimensionality = &dim
			}

			var err error
			result, err = e.client.Models.EmbedContent(ctx, model, genai.Text(input), embedConfig)
			if err != nil {
				return fmt.Errorf("embed content failed: %w", err)
			}
			return nil
		}, retry.WithMaxRetries(e.maxRetries), retry.WithBaseWait(e.retryBaseWait))

		if err != nil {
			return nil, err
		}

		if len(result.Embeddings) == 0 {
			return nil, fmt.Errorf("no embeddings returned from API")
		}

		// The embedding is already a []float32, convert to float64
		floatEmbedding := make([]float64, len(result.Embeddings[0].Values))
		for i, val := range result.Embeddings[0].Values {
			floatEmbedding[i] = float64(val)
		}
		embeddings = append(embeddings, floatEmbedding)

		// Estimate token count (Google doesn't provide exact token counts in embedding API)
		// Using rough estimation: ~4 characters per token
		estimatedTokens := (len(input) + 3) / 4
		totalPromptTokens += estimatedTokens
	}

	response := &embedding.Response{
		Floats: embeddings,
		Model:  model,
		Usage: embedding.Usage{
			PromptTokens: totalPromptTokens,
			TotalTokens:  totalPromptTokens, // For embeddings, total = prompt tokens
		},
		Metadata: map[string]any{
			"provider":  e.Name(),
			"task_type": e.taskType,
		},
	}

	return response, nil
}

// initClient initializes the Google AI client.
func (e *Embedder) initClient(ctx context.Context) error {
	if e.client != nil {
		return nil
	}

	// Create client config
	config := &genai.ClientConfig{
		APIKey:   e.apiKey,
		Project:  e.projectID,
		Location: e.location,
	}

	client, err := genai.NewClient(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create genai client: %w", err)
	}

	e.client = client
	return nil
}
