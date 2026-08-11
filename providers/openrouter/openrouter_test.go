package openrouter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestProvider_ImplementsInterfaces(t *testing.T) {
	provider := New()

	// Test that it implements LLM interface
	var _ llm.LLM = provider

	// Test that it implements StreamingLLM interface
	var _ llm.StreamingLLM = provider
}

func TestNew(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		provider := New()
		assert.NotNil(t, provider)
		assert.Equal(t, DefaultModel, provider.model)
		assert.Equal(t, DefaultEndpoint, provider.endpoint)
		assert.Equal(t, DefaultMaxTokens, provider.maxTokens)
		assert.NotNil(t, provider.Provider)
	})

	t.Run("with options", func(t *testing.T) {
		provider := New(
			WithAPIKey("test-key"),
			WithModel("openai/gpt-3.5-turbo"),
			WithEndpoint("https://custom.endpoint.com"),
			WithMaxTokens(2048),
			WithSiteURL("https://myapp.com"),
			WithSiteName("My App"),
		)
		assert.NotNil(t, provider)
		assert.Equal(t, "test-key", provider.apiKey)
		assert.Equal(t, "openai/gpt-3.5-turbo", provider.model)
		assert.Equal(t, "https://custom.endpoint.com", provider.endpoint)
		assert.Equal(t, 2048, provider.maxTokens)
		assert.Equal(t, "https://myapp.com", provider.siteURL)
		assert.Equal(t, "My App", provider.siteName)
	})
}

func TestName(t *testing.T) {
	provider := New(WithModel("openai/gpt-4"))
	assert.Equal(t, "openrouter", provider.Name())
}

func TestGenerateInheritsDisjointUsageConversion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"chatcmpl-1",
			"model":"openai/gpt-5",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":200,"completion_tokens":10,"total_tokens":210,"prompt_tokens_details":{"cached_tokens":150}}
		}`)
	}))
	defer server.Close()

	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
		WithModel("openai/gpt-5"),
		WithMaxRetries(0),
	)
	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	assert.Equal(t, 50, response.Usage.InputTokens)
	assert.Equal(t, 150, response.Usage.CacheReadInputTokens)
	assert.Equal(t, 200, response.Usage.TotalInputTokens())
	assert.NotNil(t, response.Usage.Cost)
	assert.Equal(t, llm.CostSourceListPriceEstimate, response.Usage.Cost.Source)
}

func TestGenerateMarksCostUnknownWhenReportAndFallbackAreUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"chatcmpl-missing-cost",
			"model":"unlisted/model",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110}
		}`)
	}))
	t.Cleanup(server.Close)

	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
		WithModel("unlisted/model"),
		WithMaxRetries(0),
	)
	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	assert.Nil(t, response.Usage.Cost)
	assert.True(t, response.Usage.CostEstimateUnavailable)
}

func TestGenerateMarksCostUnknownWhenUsageIsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"chatcmpl-missing-usage",
			"model":"openai/gpt-5",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`)
	}))
	t.Cleanup(server.Close)

	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
		WithModel("openai/gpt-5"),
		WithMaxRetries(0),
	)
	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	assert.Nil(t, response.Usage.Cost)
	assert.True(t, response.Usage.CostEstimateUnavailable)
}

func TestGenerateUsesProviderReportedCostForArbitraryModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"chatcmpl-cost",
			"model":"deepseek/deepseek-v4-flash-0731",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"cost":0.004321}
		}`)
	}))
	t.Cleanup(server.Close)

	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
		WithModel("openrouter/auto"),
		WithMaxRetries(0),
	)
	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	assert.Equal(t, "deepseek/deepseek-v4-flash-0731", response.Model)
	assert.NotNil(t, response.Usage.Cost)
	assert.Equal(t, 0.004321, response.Usage.Cost.Total)
	assert.Equal(t, "USD", response.Usage.Cost.Currency)
	assert.Equal(t, llm.CostSourceProviderReported, response.Usage.Cost.Source)
	assert.True(t, response.Usage.Cost.BreakdownUnavailable)
}

func TestStreamUsesProviderReportedCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-cost\",\"model\":\"deepseek/deepseek-v4-flash-0731\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-cost\",\"model\":\"deepseek/deepseek-v4-flash-0731\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-cost\",\"model\":\"deepseek/deepseek-v4-flash-0731\",\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":10,\"total_tokens\":110,\"cost\":0.004321}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)

	provider := New(
		WithAPIKey("test-key"),
		WithEndpoint(server.URL),
		WithModel("openrouter/auto"),
		WithMaxRetries(0),
	)
	iterator, err := provider.Stream(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, iterator.Close()) })
	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		assert.NoError(t, accumulator.AddEvent(iterator.Event()))
	}
	assert.NoError(t, iterator.Err())
	response := accumulator.Response()
	assert.Equal(t, "deepseek/deepseek-v4-flash-0731", response.Model)
	assert.NotNil(t, response.Usage.Cost)
	assert.Equal(t, 0.004321, response.Usage.Cost.Total)
	assert.Equal(t, llm.CostSourceProviderReported, response.Usage.Cost.Source)
}

func TestGetAPIKey(t *testing.T) {
	t.Run("OPENROUTER_API_KEY", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "openrouter-key")
		t.Setenv("OPENAI_API_KEY", "openai-key")

		key := getAPIKey()
		assert.Equal(t, "openrouter-key", key)
	})

	t.Run("fallback to OPENAI_API_KEY", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "openai-key") // ignored

		key := getAPIKey()
		assert.Equal(t, "", key)
	})

	t.Run("no API key", func(t *testing.T) {
		t.Setenv("OPENROUTER_API_KEY", "")
		t.Setenv("OPENAI_API_KEY", "")

		key := getAPIKey()
		assert.Equal(t, "", key)
	})
}
