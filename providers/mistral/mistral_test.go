package mistral

import (
	"context"
	"os"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestProvider_Name(t *testing.T) {
	p := New()
	expected := "mistral-" + DefaultModel
	assert.Equal(t, expected, p.Name())
}

func TestProvider_WithModel(t *testing.T) {
	model := ModelMistralSmall
	p := New(WithModel(model))
	expected := "mistral-" + model
	assert.Equal(t, expected, p.Name())
}

func TestProvider_WithAPIKey(t *testing.T) {
	apiKey := "test-api-key"
	p := New(WithAPIKey(apiKey))
	assert.Equal(t, apiKey, p.apiKey)
}

func TestProvider_WithEndpoint(t *testing.T) {
	endpoint := "https://custom-endpoint.com/v1/chat/completions"
	p := New(WithEndpoint(endpoint))
	assert.Equal(t, endpoint, p.endpoint)
}

func TestProvider_WithMaxTokens(t *testing.T) {
	maxTokens := 2048
	p := New(WithMaxTokens(maxTokens))
	assert.Equal(t, maxTokens, p.maxTokens)
}

func TestProvider_Generate(t *testing.T) {
	if os.Getenv("MISTRAL_API_KEY") == "" {
		t.Skip("MISTRAL_API_KEY not set, skipping integration test")
	}

	p := New(WithModel(ModelMistralSmall))
	message := llm.NewUserTextMessage("Hello, world!")
	resp, err := p.Generate(context.Background(), llm.WithMessages(message))
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotEmpty(t, resp.Message().Text())
}
