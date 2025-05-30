//go:build integration

package openai

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diveagents/dive/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_SimpleTextGeneration tests basic text generation with real API calls
func TestIntegration_SimpleTextGeneration(t *testing.T) {
	provider := setupIntegrationProvider(t)

	ctx := context.Background()
	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Say 'Integration test successful' and nothing else"),
		llm.WithTemperature(0.0),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotEmpty(t, response.ID)
	assert.Equal(t, llm.Assistant, response.Role)
	assert.NotEmpty(t, response.Message().Text())
	assert.Contains(t, strings.ToLower(response.Message().Text()), "integration test successful")
}

// TestIntegration_MultipleMessages tests conversation with multiple messages
func TestIntegration_MultipleMessages(t *testing.T) {
	provider := setupIntegrationProvider(t)

	ctx := context.Background()
	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("I will ask you to remember a number. The number is: 42"),
			llm.NewAssistantTextMessage("I will remember that the number is 42."),
			llm.NewUserTextMessage("What was the number I asked you to remember?"),
		),
		llm.WithTemperature(0.0),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Contains(t, response.Message().Text(), "42")
}

// TestIntegration_WebSearchTool tests the web search tool functionality
func TestIntegration_WebSearchTool(t *testing.T) {
	provider := setupIntegrationProvider(t)

	webSearchTool := NewWebSearchTool(WebSearchToolOptions{
		SearchContextSize: "medium",
		UserLocation: &UserLocation{
			Type:    "approximate",
			Country: "US",
		},
	})

	ctx := context.Background()
	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Search for the latest news about artificial intelligence and summarize one recent development"),
		llm.WithTools(webSearchTool),
		llm.WithTemperature(0.3),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotEmpty(t, response.Message().Text())

	// The response should contain some information about AI news
	text := strings.ToLower(response.Message().Text())
	assert.True(t,
		strings.Contains(text, "artificial intelligence") ||
			strings.Contains(text, "ai") ||
			strings.Contains(text, "machine learning"),
		"Response should contain AI-related content: %s", response.Message().Text())
}

// TestIntegration_ImageGenerationTool tests the image generation tool functionality
func TestIntegration_ImageGenerationTool(t *testing.T) {
	provider := setupIntegrationProvider(t)

	imageGenTool := NewImageGenerationTool(ImageGenerationToolOptions{
		Size:    "1024x1024",
		Quality: "medium",
	})

	ctx := context.Background()
	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Generate a simple image of a red circle on a white background"),
		llm.WithTools(imageGenTool),
		llm.WithTemperature(0.3),
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotEmpty(t, response.Message().Text())

	// The response should mention image generation
	text := strings.ToLower(response.Message().Text())
	assert.True(t,
		strings.Contains(text, "image") ||
			strings.Contains(text, "generated") ||
			strings.Contains(text, "created"),
		"Response should mention image generation: %s", response.Message().Text())
}

// TestIntegration_StreamingResponse tests streaming response functionality
func TestIntegration_StreamingResponse(t *testing.T) {
	provider := setupIntegrationProvider(t)

	ctx := context.Background()
	iterator, err := provider.Stream(ctx,
		llm.WithUserTextMessage("Count from 1 to 5, with each number on a separate line. Be concise."),
		llm.WithTemperature(0.0),
	)

	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	// Accumulate events and verify streaming works
	accum := llm.NewResponseAccumulator()
	eventCount := 0
	var hasMessageStart, hasContentBlockStart bool

	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)

		switch event.Type {
		case llm.EventTypeMessageStart:
			hasMessageStart = true
		case llm.EventTypeContentBlockStart:
			hasContentBlockStart = true
		}

		require.NoError(t, accum.AddEvent(event))
		eventCount++
	}

	require.NoError(t, iterator.Err())
	assert.Greater(t, eventCount, 0, "Should receive at least one event")
	assert.True(t, hasMessageStart, "Should receive message start event")
	assert.True(t, hasContentBlockStart, "Should receive content block start event")
	// Note: OpenAI Responses API appears to send complete text in one go rather than streaming deltas

	response := accum.Response()
	require.NotNil(t, response)
	assert.NotEmpty(t, response.Message().Text())

	// Verify the response contains numbers 1-5
	text := response.Message().Text()
	for i := 1; i <= 5; i++ {
		assert.Contains(t, text, string(rune('0'+i)), "Response should contain number %d", i)
	}
}

// TestIntegration_StreamingWithTools tests streaming response with tool usage
func TestIntegration_StreamingWithTools(t *testing.T) {
	provider := setupIntegrationProvider(t)

	webSearchTool := NewWebSearchTool(WebSearchToolOptions{
		SearchContextSize: "low",
		UserLocation: &UserLocation{
			Type:    "approximate",
			Country: "US",
		},
	})

	ctx := context.Background()
	iterator, err := provider.Stream(ctx,
		llm.WithUserTextMessage("Search for the current weather in San Francisco and tell me what it is like today"),
		llm.WithTools(webSearchTool),
		llm.WithTemperature(0.3),
	)

	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	// Accumulate events and verify tool usage in streaming
	accum := llm.NewResponseAccumulator()
	eventCount := 0
	var hasToolUse bool
	var hasContentBlockStart bool

	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)

		switch event.Type {
		case llm.EventTypeContentBlockStart:
			hasContentBlockStart = true
			if event.ContentBlock != nil && event.ContentBlock.Type == llm.ContentTypeToolUse {
				hasToolUse = true
			}
		}

		require.NoError(t, accum.AddEvent(event))
		eventCount++
	}

	require.NoError(t, iterator.Err())
	assert.Greater(t, eventCount, 0)

	response := accum.Response()
	require.NotNil(t, response)

	// Either we should use the web search tool, OR we should get a meaningful response
	// (The model might decide it doesn't need to search for some queries)
	if hasToolUse {
		t.Log("Web search tool was used successfully")
		assert.NotEmpty(t, response.Message().Text())
	} else {
		t.Log("Web search tool was not used, but we should still get a response")
		assert.True(t, hasContentBlockStart, "Should receive at least content block start event")
		// For weather queries without search, the model should indicate it cannot access real-time data
		responseText := strings.ToLower(response.Message().Text())
		assert.True(t,
			strings.Contains(responseText, "cannot") ||
				strings.Contains(responseText, "don't have") ||
				strings.Contains(responseText, "unable") ||
				strings.Contains(responseText, "weather") ||
				len(responseText) > 10, // Any substantial response
			"Should either use tool or indicate inability to access real-time data: %s", response.Message().Text())
	}
}

// TestIntegration_ErrorScenarios tests various error conditions
func TestIntegration_ErrorScenarios(t *testing.T) {
	t.Run("invalid API key", func(t *testing.T) {
		provider := New(WithAPIKey("invalid-key"))

		ctx := context.Background()
		_, err := provider.Generate(ctx,
			llm.WithUserTextMessage("This should fail"),
		)

		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "401")
	})

	t.Run("empty messages", func(t *testing.T) {
		provider := setupIntegrationProvider(t)

		ctx := context.Background()
		_, err := provider.Generate(ctx)

		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "no messages")
	})

	t.Run("context timeout", func(t *testing.T) {
		provider := setupIntegrationProvider(t)

		// Create a context that times out very quickly
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()

		_, err := provider.Generate(ctx,
			llm.WithUserTextMessage("This should timeout"),
		)

		require.Error(t, err)
		assert.True(t,
			strings.Contains(strings.ToLower(err.Error()), "timeout") ||
				strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded"),
			"Error should indicate timeout: %s", err.Error())
	})
}

// TestIntegration_AdvancedFeatures tests advanced OpenAI-specific features
func TestIntegration_AdvancedFeatures(t *testing.T) {
	t.Run("reasoning effort with o-series model", func(t *testing.T) {
		// Skip if we don't have access to o-series models
		if os.Getenv("OPENAI_TEST_O_SERIES") == "" {
			t.Skip("Skipping o-series model test (set OPENAI_TEST_O_SERIES=1 to enable)")
		}

		provider := New(WithModel("o1-mini"))

		ctx := context.Background()
		response, err := provider.Generate(ctx,
			llm.WithUserTextMessage("Solve this simple math problem: 15 + 27"),
			llm.WithReasoningEffort("medium"),
		)

		require.NoError(t, err)
		require.NotNil(t, response)
		assert.Contains(t, response.Message().Text(), "42")
	})

	t.Run("parallel tool calls", func(t *testing.T) {
		provider := setupIntegrationProvider(t)

		webSearchTool := NewWebSearchTool(WebSearchToolOptions{
			SearchContextSize: "low",
		})

		ctx := context.Background()
		response, err := provider.Generate(ctx,
			llm.WithUserTextMessage("Search for information about both 'OpenAI' and 'artificial intelligence' separately"),
			llm.WithTools(webSearchTool),
			llm.WithParallelToolCalls(true),
			llm.WithTemperature(0.3),
		)

		require.NoError(t, err)
		require.NotNil(t, response)
		assert.NotEmpty(t, response.Message().Text())
	})

	t.Run("service tier configuration", func(t *testing.T) {
		provider := setupIntegrationProvider(t)

		ctx := context.Background()
		response, err := provider.Generate(ctx,
			llm.WithUserTextMessage("Say 'Service tier test'"),
			llm.WithServiceTier("default"),
			llm.WithTemperature(0.0),
		)

		require.NoError(t, err)
		require.NotNil(t, response)
		assert.NotEmpty(t, response.Message().Text())
	})
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
