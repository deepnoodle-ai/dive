package mistral

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestGenerateInheritsDisjointUsageConversion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"chatcmpl-1",
			"model":"mistral-small-latest",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":200,"completion_tokens":10,"total_tokens":210,"prompt_tokens_details":{"cached_tokens":150}}
		}`)
	}))
	defer server.Close()

	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
		WithModel(ModelMistralSmall),
		WithMaxRetries(0),
	)
	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	assert.Equal(t, 50, response.Usage.InputTokens)
	assert.Equal(t, 150, response.Usage.CacheReadInputTokens)
	assert.Equal(t, 200, response.Usage.TotalInputTokens())
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
