package openrouter

import (
	"context"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestOpenRouterIntegration(t *testing.T) {
	// Skip if no API key is available
	if getAPIKey() == "" {
		t.Skip("Skipping integration test: no OPENROUTER_API_KEY set")
	}

	provider := New(WithModel(ModelGPT4o))

	// This would require an actual API key to work
	ctx := context.Background()
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Say 'hello' and nothing else."),
	))
	assert.NoError(t, err)
	assert.NotNil(t, response)

	assert.Equal(t, llm.Assistant, response.Role)
	assert.True(t, response.Usage.TotalInputTokens()+response.Usage.OutputTokens > 0)
	assert.NotNil(t, response.Usage.Cost)
	assert.Equal(t, llm.CostSourceProviderReported, response.Usage.Cost.Source)
	assert.Equal(t, "USD", response.Usage.Cost.Currency)
	assert.True(t, response.Usage.Cost.Total >= 0)

	ok := strings.Contains(strings.ToLower(response.Message().Text()), "hello")
	assert.True(t, ok)
}
