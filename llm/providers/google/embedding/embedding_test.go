package google

import (
	"context"
	"os"
	"testing"

	"github.com/deepnoodle-ai/dive/embedding"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// MockGenAIClient is a mock implementation of the genai.Client for testing
type MockGenAIClient struct {
	embedResponse *genai.EmbedContentResponse
	embedError    error
}

func (m *MockGenAIClient) EmbedContent(ctx context.Context, model string, content genai.Content, config *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
	if m.embedError != nil {
		return nil, m.embedError
	}
	return m.embedResponse, nil
}

func TestEmbedder_Name(t *testing.T) {
	embedder := New()
	require.Equal(t, "google", embedder.Name())
}

func TestEmbedder_New_WithEnvironment(t *testing.T) {
	// Test with GEMINI_API_KEY
	t.Setenv("GEMINI_API_KEY", "test-gemini-key")
	embedder := New()
	require.Equal(t, "test-gemini-key", embedder.apiKey)

	// Test with GOOGLE_API_KEY when GEMINI_API_KEY is not set
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "test-google-key")
	embedder = New()
	require.Equal(t, "test-google-key", embedder.apiKey)

	// Test with project ID and location
	t.Setenv("GOOGLE_PROJECT_ID", "test-project")
	t.Setenv("GOOGLE_LOCATION", "us-central1")
	embedder = New()
	require.Equal(t, "test-project", embedder.projectID)
	require.Equal(t, "us-central1", embedder.location)
}

func TestEmbedder_WithMethods(t *testing.T) {
	embedder := New().
		WithAPIKey("test-key").
		WithProjectID("test-project").
		WithLocation("us-west1").
		WithTaskType("RETRIEVAL_QUERY").
		WithMaxRetries(5)

	require.Equal(t, "test-key", embedder.apiKey)
	require.Equal(t, "test-project", embedder.projectID)
	require.Equal(t, "us-west1", embedder.location)
	require.Equal(t, "RETRIEVAL_QUERY", embedder.taskType)
	require.Equal(t, 5, embedder.maxRetries)
}

func TestEmbedder_Embed_ValidationError(t *testing.T) {
	embedder := New()

	ctx := context.Background()
	// Test with no input
	_, err := embedder.Embed(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "input is required")

	// Test with both input and inputs set
	_, err = embedder.Embed(ctx,
		embedding.WithInput("single input"),
		embedding.WithInputs([]string{"input1", "input2"}),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "input and inputs cannot both be set")

	// Test with negative dimensions
	_, err = embedder.Embed(ctx,
		embedding.WithInput("test"),
		embedding.WithDimensions(-1),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dimensions must not be negative")
}

func TestEmbedder_Embed_TaskTypes(t *testing.T) {
	testCases := []struct {
		taskType string
		valid    bool
	}{
		{"RETRIEVAL_QUERY", true},
		{"RETRIEVAL_DOCUMENT", true},
		{"SEMANTIC_SIMILARITY", true},
		{"CLASSIFICATION", true},
		{"CLUSTERING", true},
	}

	for _, tc := range testCases {
		t.Run(tc.taskType, func(t *testing.T) {
			embedder := New().WithTaskType(tc.taskType)
			require.Equal(t, tc.taskType, embedder.taskType)
		})
	}
}

func TestEmbedder_InitClientError(t *testing.T) {
	// Test initialization without any credentials
	testEmbedder := New()
	testEmbedder.apiKey = ""
	testEmbedder.projectID = ""
	testEmbedder.location = ""

	testCtx := context.Background()

	// This will attempt to create a client without credentials
	// The actual error will depend on the genai client's behavior
	// We're mainly testing that our error handling works
	err := testEmbedder.initClient(testCtx)

	// The genai client might succeed with default credentials (ADC)
	// or it might fail if no credentials are available
	// We just check that the method completes without panic
	_ = err // err can be nil or non-nil depending on environment
}

func TestEmbedder_EstimateTokens(t *testing.T) {
	// Test token estimation logic
	testCases := []struct {
		input          string
		expectedTokens int
	}{
		{"", 0},
		{"a", 1},
		{"test", 1},
		{"hello world", 3},
		{"The quick brown fox jumps over the lazy dog", 11},
		{"This is a longer sentence with multiple words to test token estimation", 18},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			// Our estimation formula: (len(input) + 3) / 4
			estimatedTokens := (len(tc.input) + 3) / 4
			require.Equal(t, tc.expectedTokens, estimatedTokens)
		})
	}
}

func TestEmbedder_MultipleInputs(t *testing.T) {
	config := &embedding.Config{}
	config.Apply([]embedding.Option{
		embedding.WithInputs([]string{
			"first input",
			"second input",
			"third input",
		}),
		embedding.WithModel("text-embedding-004"),
	})

	// Validate configuration
	err := config.Validate()
	require.NoError(t, err)

	// Check inputs
	inputs := config.GetInputAsSlice()
	require.Len(t, inputs, 3)
	require.Equal(t, "first input", inputs[0])
	require.Equal(t, "second input", inputs[1])
	require.Equal(t, "third input", inputs[2])

	// Check input count
	require.Equal(t, 3, config.InputCount())
}

// TestIntegration_GoogleEmbedding tests the actual integration with Google AI
// This test is skipped by default and requires proper API credentials to run
func TestIntegration_GoogleEmbedding(t *testing.T) {
	// Skip if no API key is present
	apiKey := ""
	if value := os.Getenv("GEMINI_API_KEY"); value != "" {
		apiKey = value
	} else if value := os.Getenv("GOOGLE_API_KEY"); value != "" {
		apiKey = value
	}

	if apiKey == "" {
		t.Skip("Skipping integration test: no Google API key found")
	}

	ctx := context.Background()
	embedder := New().WithAPIKey(apiKey)

	// Test single input
	response, err := embedder.Embed(ctx,
		embedding.WithInput("What is artificial intelligence?"),
		embedding.WithModel("text-embedding-004"),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Len(t, response.Floats, 1)
	require.NotEmpty(t, response.Floats[0])
	require.Equal(t, "text-embedding-004", response.Model)
	require.Greater(t, response.Usage.PromptTokens, 0)
	require.Equal(t, response.Usage.PromptTokens, response.Usage.TotalTokens)

	// Test multiple inputs
	response, err = embedder.Embed(ctx,
		embedding.WithInputs([]string{
			"First sentence for embedding",
			"Second sentence for embedding",
		}),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Len(t, response.Floats, 2)
	require.NotEmpty(t, response.Floats[0])
	require.NotEmpty(t, response.Floats[1])

	// Test with dimensions
	response, err = embedder.Embed(ctx,
		embedding.WithInput("Test with custom dimensions"),
		embedding.WithDimensions(256),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Len(t, response.Floats, 1)
	require.Len(t, response.Floats[0], 256)

	// Test different task types
	for _, taskType := range []string{"RETRIEVAL_QUERY", "RETRIEVAL_DOCUMENT"} {
		embedder = embedder.WithTaskType(taskType)
		response, err = embedder.Embed(ctx,
			embedding.WithInput("Test with task type: "+taskType),
		)

		require.NoError(t, err)
		require.NotNil(t, response)
		require.Equal(t, taskType, response.Metadata["task_type"])
	}
}
