//go:build integration

package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestIntegration_SimpleTextGeneration(t *testing.T) {
	provider := setupIntegrationProvider(t)

	response, err := provider.Generate(
		context.Background(),
		llm.WithUserTextMessage("Say 'Integration test successful' and nothing else"),
		llm.WithTemperature(0.0),
	)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotEmpty(t, response.ID)
	assert.Equal(t, llm.Assistant, response.Role)

	text := response.Message().Text()
	assert.NotEmpty(t, text)
	assert.Equal(t,
		"integration test successful",
		strings.TrimSpace(strings.ToLower(text)))
}

func TestIntegration_MultipleMessages(t *testing.T) {
	provider := setupIntegrationProvider(t)

	response, err := provider.Generate(
		context.Background(),
		llm.WithMessages(
			llm.NewUserTextMessage("Respond with a totally random word"),
			llm.NewAssistantTextMessage("The word is 'flabbergasted'"),
			llm.NewUserTextMessage("Now respond with that word only"),
		),
		llm.WithTemperature(0.0),
	)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	text := strings.TrimSpace(strings.ToLower(response.Message().Text()))
	assert.NotEmpty(t, text)
	assert.Equal(t, "flabbergasted", text)
}

func TestIntegration_PromptCachingUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	provider := New(
		WithAPIKey(apiKey),
		WithModel(ModelGPT56Luna),
		WithMaxTokens(16),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	uniquePrefix := fmt.Sprintf("Dive cache qualification %d. ", time.Now().UnixNano())
	longPrompt := uniquePrefix + strings.Repeat(
		"This repeated sentence makes the stable prompt prefix long enough for automatic prompt caching. ",
		700,
	)
	request := []llm.Option{
		llm.WithUserTextMessage(longPrompt + "\nReply with the word ready."),
		llm.WithPromptCacheKey(uniqueCacheKey(t)),
	}

	first, err := provider.Generate(ctx, request...)
	assert.NoError(t, err)
	assert.NotNil(t, first)
	assert.True(t, first.Usage.CacheCreationInputTokens > 0, "first GPT-5.6 request should report a cache write")

	var cached *llm.Response
	for attempt := 0; attempt < 4; attempt++ {
		candidate, err := provider.Generate(ctx, request...)
		assert.NoError(t, err)
		assert.NotNil(t, candidate)
		t.Logf("OpenAI cache attempt %d: model=%s input=%d cached=%d total_input=%d",
			attempt+1,
			candidate.Model,
			candidate.Usage.InputTokens,
			candidate.Usage.CacheReadInputTokens,
			candidate.Usage.TotalInputTokens(),
		)
		if candidate.Usage.CacheReadInputTokens > 0 {
			cached = candidate
			break
		}
		time.Sleep(time.Second)
	}
	assert.NotNil(t, cached, "repeated request should observe an OpenAI cache hit")
	assert.Equal(t, first.Usage.TotalInputTokens(), cached.Usage.TotalInputTokens())
	expectedCost := TextModelPricing[ModelGPT56Luna].CostOf(&cached.Usage)
	assert.NotNil(t, cached.Usage.Cost)
	assert.Equal(t, expectedCost, *cached.Usage.Cost)
	t.Logf("OpenAI cache hit: input=%d cached=%d total_input=%d cost=%f",
		cached.Usage.InputTokens,
		cached.Usage.CacheReadInputTokens,
		cached.Usage.TotalInputTokens(),
		cached.Usage.Cost.Total,
	)
}

func uniqueCacheKey(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("dive-%s-%d", t.Name(), time.Now().UnixNano())
}

// TestIntegration_WebSearchTool tests the web search tool functionality
func TestIntegration_WebSearchTool(t *testing.T) {
	provider := setupIntegrationProvider(t)

	webSearchTool := NewWebSearchPreviewTool(
		WebSearchPreviewToolOptions{
			SearchContextSize: "medium",
			UserLocation: &UserLocation{
				Type:    "approximate",
				Country: "US",
			},
		})

	response, err := provider.Generate(
		context.Background(),
		llm.WithUserTextMessage("Search for the latest news on cryptocurrency"),
		llm.WithTools(webSearchTool),
		llm.WithTemperature(0.0),
	)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.NotEmpty(t, response.Message().Text())

	// The response should contain some information about AI news
	text := strings.ToLower(response.Message().Text())
	assert.True(t,
		strings.Contains(text, "bitcoin") || strings.Contains(text, "ethereum"),
		"Response should contain crypto content: %s", response.Message().Text())
}

// TestIntegration_ErrorScenarios tests various error conditions
func TestIntegration_ErrorScenarios(t *testing.T) {
	t.Run("invalid API key", func(t *testing.T) {
		provider := New(WithAPIKey("invalid-key"))

		ctx := context.Background()
		_, err := provider.Generate(ctx, llm.WithUserTextMessage("This should fail"))

		assert.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "401")
	})

	t.Run("empty messages", func(t *testing.T) {
		provider := setupIntegrationProvider(t)

		_, err := provider.Generate(context.Background())

		assert.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "no messages")
	})

	t.Run("context timeout", func(t *testing.T) {
		provider := setupIntegrationProvider(t)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		_, err := provider.Generate(ctx, llm.WithUserTextMessage("This should timeout"))
		assert.Error(t, err)

		errMessage := strings.ToLower(err.Error())
		ok := strings.Contains(errMessage, "timeout") ||
			strings.Contains(errMessage, "context deadline exceeded")
		assert.True(t, ok, "Error should indicate timeout: %s", err.Error())
	})
}

// TestIntegration_AdvancedFeatures tests advanced OpenAI-specific features
func TestIntegration_AdvancedFeatures(t *testing.T) {
	t.Run("reasoning effort with o-series model", func(t *testing.T) {
		provider := New(WithModel("o3"))

		response, err := provider.Generate(
			context.Background(),
			llm.WithUserTextMessage("Solve this simple math problem: 15 + 27"),
			llm.WithReasoningEffort("medium"),
		)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Contains(t, response.Message().Text(), "42")
	})

	t.Run("service tier configuration", func(t *testing.T) {
		provider := setupIntegrationProvider(t)

		response, err := provider.Generate(
			context.Background(),
			llm.WithUserTextMessage("Say 'Service tier test'"),
			llm.WithServiceTier("default"),
			llm.WithTemperature(0.0),
		)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.NotEmpty(t, response.Message().Text())
	})
}

func TestIntegration_FileInput(t *testing.T) {
	ctx := context.Background()
	provider := setupIntegrationProvider(t)

	content, err := os.ReadFile("../../internal/fixtures/hola.pdf")
	assert.NoError(t, err)
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserMessage(
			&llm.TextContent{
				Text: "What does the PDF say?",
			},
			&llm.DocumentContent{
				Title: "file.pdf",
				Source: &llm.ContentSource{
					Type:      llm.ContentSourceTypeBase64,
					MediaType: "application/pdf",
					Data:      base64.StdEncoding.EncodeToString(content),
				},
			},
		),
	))
	assert.NoError(t, err)
	assert.Contains(t, strings.ToLower(response.Message().Text()), "hola")
}

func TestIntegration_Vision(t *testing.T) {
	ctx := context.Background()
	provider := setupIntegrationProvider(t)

	content, err := os.ReadFile("../../internal/fixtures/go.png")
	assert.NoError(t, err)
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserMessage(
			&llm.TextContent{
				Text: "What is this image? Is there a word written on it?",
			},
			&llm.ImageContent{
				Source: &llm.ContentSource{
					Type:      llm.ContentSourceTypeBase64,
					MediaType: "image/png",
					Data:      base64.StdEncoding.EncodeToString(content),
				},
			},
		),
	))
	assert.NoError(t, err)
	assert.Contains(t, strings.ToLower(response.Message().Text()), "go")
}

// setupIntegrationProvider creates a provider for integration testing
func setupIntegrationProvider(t *testing.T) *Provider {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check for API key
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	// Create provider with reasonable defaults for testing
	return New(
		WithAPIKey(apiKey),
		WithModel("gpt-4o"),
		WithMaxTokens(1000),
	)
}

// TestIntegration_PhaseReplayedAcrossStatelessTurns qualifies faithful manual
// continuation: the assistant turn from a live GPT-5.6 call is persisted,
// reloaded, and resent as history without `previous_response_id`. Every phase
// OpenAI assigned must reach the wire unchanged on the follow-up request.
func TestIntegration_PhaseReplayedAcrossStatelessTurns(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}
	provider := New(
		WithAPIKey(apiKey),
		WithModel(string(ModelGPT56Sol)),
		WithMaxTokens(2000),
	)

	first, err := provider.Generate(
		context.Background(),
		llm.WithUserTextMessage("In two sentences, what is the capital of France?"),
		llm.WithReasoningEffort(llm.ReasoningEffortLow),
	)
	assert.NoError(t, err)

	// Persist and reload the assistant turn the way a durable runtime would.
	stored, err := json.Marshal(first.Message())
	assert.NoError(t, err)
	var reloaded llm.Message
	assert.NoError(t, json.Unmarshal(stored, &reloaded))

	var phases []string
	for _, content := range reloaded.Content {
		if text, ok := content.(*llm.TextContent); ok {
			if phase := text.Metadata[openAIPhaseMetadataKey]; phase != "" {
				phases = append(phases, phase)
			}
		}
	}
	if len(phases) == 0 {
		t.Skip("model returned no phased output messages; nothing to qualify")
	}

	// The replayed request must carry every phase the model assigned.
	replayed, err := encodeMessages([]*llm.Message{&reloaded})
	assert.NoError(t, err)
	replayedJSON, err := json.Marshal(replayed)
	assert.NoError(t, err)
	for _, phase := range phases {
		assert.Contains(t, string(replayedJSON), `"phase":"`+phase+`"`)
	}

	// And OpenAI must accept that history on a stateless follow-up turn.
	second, err := provider.Generate(
		context.Background(),
		llm.WithMessages(
			llm.NewUserTextMessage("In two sentences, what is the capital of France?"),
			&reloaded,
			llm.NewUserTextMessage("Now name its largest airport."),
		),
		llm.WithReasoningEffort(llm.ReasoningEffortLow),
	)
	assert.NoError(t, err)
	assert.NotEmpty(t, second.Message().Text())
}
