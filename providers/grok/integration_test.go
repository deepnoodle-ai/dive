package grok

import (
	"context"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func skipIfNoAPIKey(t *testing.T) {
	t.Helper()
	if getAPIKey() == "" {
		t.Skip("Skipping integration test: no XAI_API_KEY or GROK_API_KEY set")
	}
}

func TestIntegration_Generate(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New()
	ctx := context.Background()

	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Say 'hello' and nothing else."),
	))
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, llm.Assistant, response.Role)

	ok := strings.Contains(strings.ToLower(response.Message().Text()), "hello")
	assert.True(t, ok)
}

func TestIntegration_Stream(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New()
	ctx := context.Background()

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Say 'hello' and nothing else."),
	))
	assert.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		err := accumulator.AddEvent(event)
		assert.NoError(t, err)
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	assert.NotNil(t, response)
	ok := strings.Contains(strings.ToLower(response.Message().Text()), "hello")
	assert.True(t, ok)
}

func TestIntegration_WebSearch(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New()
	ctx := context.Background()

	webSearch := NewWebSearchTool(WebSearchToolOptions{})

	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("What is the current weather in San Francisco? Be brief."),
		),
		llm.WithTools(webSearch),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, llm.Assistant, response.Role)

	text := response.Message().Text()
	t.Logf("Web search response: %s", text)
	assert.True(t, len(text) > 0)
}

func TestIntegration_WebSearchWithDomains(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New()
	ctx := context.Background()

	webSearch := NewWebSearchTool(WebSearchToolOptions{
		AllowedDomains: []string{"wikipedia.org"},
	})

	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("What is Claude Shannon known for? Be brief."),
		),
		llm.WithTools(webSearch),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response)

	text := response.Message().Text()
	t.Logf("Web search with domains response: %s", text)
	assert.True(t, len(text) > 0)
}

func TestIntegration_XSearch(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New()
	ctx := context.Background()

	xSearch := NewXSearchTool(XSearchToolOptions{})

	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("What are people saying about AI on X today? Be brief."),
		),
		llm.WithTools(xSearch),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, llm.Assistant, response.Role)

	text := response.Message().Text()
	t.Logf("X search response: %s", text)
	assert.True(t, len(text) > 0)
}

func TestIntegration_WebSearchAndXSearch(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New()
	ctx := context.Background()

	webSearch := NewWebSearchTool(WebSearchToolOptions{})
	xSearch := NewXSearchTool(XSearchToolOptions{})

	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("What is xAI and what are people saying about it on X? Be brief."),
		),
		llm.WithTools(webSearch, xSearch),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response)

	text := response.Message().Text()
	t.Logf("Web + X search response: %s", text)
	assert.True(t, len(text) > 0)
}

func TestIntegration_WebSearchStream(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New()
	ctx := context.Background()

	webSearch := NewWebSearchTool(WebSearchToolOptions{})

	iterator, err := provider.Stream(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("What is the latest news about SpaceX? One sentence only."),
		),
		llm.WithTools(webSearch),
	)
	assert.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		err := accumulator.AddEvent(event)
		assert.NoError(t, err)
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	assert.NotNil(t, response)
	text := response.Message().Text()
	t.Logf("Streaming web search response: %s", text)
	assert.True(t, len(text) > 0)
}

func TestIntegration_PromptCaching(t *testing.T) {
	skipIfNoAPIKey(t)

	cacheKey := "dive-test-cache-key-abc123"
	provider := New(WithPromptCacheKey(cacheKey))
	ctx := context.Background()

	systemPrompt := "You are a helpful assistant that answers questions concisely. " +
		"Always respond in exactly one sentence."

	// First request establishes the cache
	response1, err := provider.Generate(ctx,
		llm.WithSystemPrompt(systemPrompt),
		llm.WithMessages(
			llm.NewUserTextMessage("What is 2+2?"),
		),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response1)
	t.Logf("Turn 1 - input: %d, output: %d, cached: %d",
		response1.Usage.InputTokens, response1.Usage.OutputTokens,
		response1.Usage.CacheReadInputTokens)

	// Second request with the same cache key should get cache hits
	response2, err := provider.Generate(ctx,
		llm.WithSystemPrompt(systemPrompt),
		llm.WithMessages(
			llm.NewUserTextMessage("What is 2+2?"),
			llm.NewAssistantTextMessage(response1.Message().Text()),
			llm.NewUserTextMessage("What is 3+3?"),
		),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response2)
	t.Logf("Turn 2 - input: %d, output: %d, cached: %d",
		response2.Usage.InputTokens, response2.Usage.OutputTokens,
		response2.Usage.CacheReadInputTokens)

	// Verify we got a response (cache hits are best-effort, so we don't
	// assert on CacheReadInputTokens > 0)
	assert.True(t, len(response2.Message().Text()) > 0)
}

func TestIntegration_PromptCachingStream(t *testing.T) {
	skipIfNoAPIKey(t)

	cacheKey := "dive-test-stream-cache-key-xyz789"
	provider := New(WithPromptCacheKey(cacheKey))
	ctx := context.Background()

	iterator, err := provider.Stream(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("Say 'cached' and nothing else."),
		),
	)
	assert.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		err := accumulator.AddEvent(iterator.Event())
		assert.NoError(t, err)
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	assert.NotNil(t, response)
	ok := strings.Contains(strings.ToLower(response.Message().Text()), "cached")
	assert.True(t, ok)
}

func TestIntegration_MultiAgent(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New(WithModel(ModelGrok420MultiAgent))
	ctx := context.Background()

	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("What are the key differences between TCP and UDP? Be brief, 2-3 sentences."),
		),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, llm.Assistant, response.Role)

	text := response.Message().Text()
	t.Logf("Multi-agent response: %s", text)
	assert.True(t, len(text) > 0)
}

func TestIntegration_MultiAgentWithTools(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New(WithModel(ModelGrok420MultiAgent))
	ctx := context.Background()

	webSearch := NewWebSearchTool(WebSearchToolOptions{})
	xSearch := NewXSearchTool(XSearchToolOptions{})

	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("What is the latest news about xAI? One paragraph."),
		),
		llm.WithTools(webSearch, xSearch),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response)

	text := response.Message().Text()
	t.Logf("Multi-agent with tools response: %s", text)
	assert.True(t, len(text) > 0)
}

func TestIntegration_MultiAgent4Agents(t *testing.T) {
	skipIfNoAPIKey(t)

	// 4 agents = reasoning effort "low" or "medium"
	provider := New(WithModel(ModelGrok420MultiAgent))
	ctx := context.Background()

	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("What is Go known for? One sentence."),
		),
		llm.WithReasoningEffort("low"),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response)

	text := response.Message().Text()
	t.Logf("4-agent response: %s", text)
	assert.True(t, len(text) > 0)
}

func TestIntegration_MultiAgent16Agents(t *testing.T) {
	skipIfNoAPIKey(t)

	// 16 agents = reasoning effort "high"
	provider := New(WithModel(ModelGrok420MultiAgent))
	ctx := context.Background()

	response, err := provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("Compare Rust and Go in one paragraph."),
		),
		llm.WithReasoningEffort("high"),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response)

	text := response.Message().Text()
	t.Logf("16-agent response: %s", text)
	assert.True(t, len(text) > 0)
}

func TestIntegration_MultiAgentStream(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := New(WithModel(ModelGrok420MultiAgent))
	ctx := context.Background()

	iterator, err := provider.Stream(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("What is Kubernetes? One sentence."),
		),
	)
	assert.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		err := accumulator.AddEvent(iterator.Event())
		assert.NoError(t, err)
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	assert.NotNil(t, response)
	text := response.Message().Text()
	t.Logf("Multi-agent streaming response: %s", text)
	assert.True(t, len(text) > 0)
}

func TestIntegration_Grok420Models(t *testing.T) {
	skipIfNoAPIKey(t)

	models := []struct {
		name  string
		model string
	}{
		{"grok-4.20-reasoning", ModelGrok420Reasoning},
		{"grok-4.20-non-reasoning", ModelGrok420NonReasoning},
	}

	for _, m := range models {
		t.Run(m.name, func(t *testing.T) {
			provider := New(WithModel(m.model))
			ctx := context.Background()

			response, err := provider.Generate(ctx, llm.WithMessages(
				llm.NewUserTextMessage("Say 'hello' and nothing else."),
			))
			assert.NoError(t, err)
			assert.NotNil(t, response)

			ok := strings.Contains(strings.ToLower(response.Message().Text()), "hello")
			assert.True(t, ok)
		})
	}
}
