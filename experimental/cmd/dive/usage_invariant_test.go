package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	googleprovider "github.com/deepnoodle-ai/dive/providers/google"
	"github.com/deepnoodle-ai/dive/providers/grok"
	"github.com/deepnoodle-ai/dive/providers/mistral"
	"github.com/deepnoodle-ai/dive/providers/ollama"
	openairesponses "github.com/deepnoodle-ai/dive/providers/openai"
	"github.com/deepnoodle-ai/dive/providers/openaicompletions"
	"github.com/deepnoodle-ai/dive/providers/openrouter"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
)

func TestProviderUsageBucketsAreDisjoint(t *testing.T) {
	responsesBody := `{
		"id":"resp_1",
		"model":"gpt-5.6-sol",
		"status":"completed",
		"output":[],
		"usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":70}}
	}`
	completionsBody := `{
		"id":"chatcmpl-1",
		"model":"gpt-5.6-sol",
		"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":70}}
	}`
	anthropicBody := `{
		"id":"msg_1",
		"type":"message",
		"role":"assistant",
		"model":"claude-opus-4-8",
		"content":[{"type":"text","text":"ok"}],
		"usage":{"input_tokens":30,"output_tokens":20,"cache_creation_input_tokens":10,"cache_read_input_tokens":60}
	}`

	tests := []struct {
		name         string
		decode       func(*testing.T) llm.Usage
		wantInput    int
		wantRead     int
		wantCreation int
		wantPrompt   int
	}{
		{
			name: "openai responses",
			decode: func(t *testing.T) llm.Usage {
				return generateFixture(t, responsesBody, func(endpoint string) llm.LLM {
					return openairesponses.New(
						openairesponses.WithAPIKey("test-key"),
						openairesponses.WithEndpoint(endpoint),
						openairesponses.WithModel(openairesponses.ModelGPT56Sol),
						openairesponses.WithMaxRetries(0),
					)
				})
			},
			wantInput: 30, wantRead: 70, wantPrompt: 100,
		},
		{
			name: "grok responses inheritance",
			decode: func(t *testing.T) llm.Usage {
				return generateFixture(t, responsesBody, func(endpoint string) llm.LLM {
					return grok.New(
						grok.WithAPIKey("test-key"),
						grok.WithEndpoint(endpoint),
						grok.WithModel(grok.ModelGrok45),
						grok.WithMaxRetries(0),
					)
				})
			},
			wantInput: 30, wantRead: 70, wantPrompt: 100,
		},
		{
			name: "openai completions",
			decode: func(t *testing.T) llm.Usage {
				return generateFixture(t, completionsBody, func(endpoint string) llm.LLM {
					return openaicompletions.New(
						openaicompletions.WithAPIKey("test-key"),
						openaicompletions.WithEndpoint(endpoint),
						openaicompletions.WithModel(openaicompletions.ModelGPT56Sol),
						openaicompletions.WithMaxRetries(0),
					)
				})
			},
			wantInput: 30, wantRead: 70, wantPrompt: 100,
		},
		{
			name: "openrouter completions inheritance",
			decode: func(t *testing.T) llm.Usage {
				return generateFixture(t, completionsBody, func(endpoint string) llm.LLM {
					return openrouter.New(
						openrouter.WithAPIKey("test-key"),
						openrouter.WithEndpoint(endpoint),
						openrouter.WithModel("openai/gpt-5.6-sol"),
						openrouter.WithMaxRetries(0),
					)
				})
			},
			wantInput: 30, wantRead: 70, wantPrompt: 100,
		},
		{
			name: "mistral completions inheritance",
			decode: func(t *testing.T) llm.Usage {
				return generateFixture(t, completionsBody, func(endpoint string) llm.LLM {
					return mistral.New(
						mistral.WithAPIKey("test-key"),
						mistral.WithEndpoint(endpoint),
						mistral.WithModel(mistral.ModelMistralSmall),
						mistral.WithMaxRetries(0),
					)
				})
			},
			wantInput: 30, wantRead: 70, wantPrompt: 100,
		},
		{
			name: "google",
			decode: func(t *testing.T) llm.Usage {
				response := &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{{
						Content:      &genai.Content{Role: "model", Parts: []*genai.Part{{Text: "ok"}}},
						FinishReason: genai.FinishReasonStop,
					}},
					UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
						PromptTokenCount:        100,
						CandidatesTokenCount:    20,
						CachedContentTokenCount: 70,
					},
				}
				iterator := googleprovider.NewStreamIteratorFromSeq(context.Background(), func(yield func(*genai.GenerateContentResponse, error) bool) {
					yield(response, nil)
				}, googleprovider.ModelGemini36Flash)
				defer iterator.Close()
				accumulator := llm.NewResponseAccumulator()
				for iterator.Next() {
					assert.NoError(t, accumulator.AddEvent(iterator.Event()))
				}
				assert.NoError(t, iterator.Err())
				return accumulator.Response().Usage
			},
			wantInput: 30, wantRead: 70, wantPrompt: 100,
		},
		{
			name: "anthropic",
			decode: func(t *testing.T) llm.Usage {
				return generateFixture(t, anthropicBody, func(endpoint string) llm.LLM {
					return anthropic.New(
						anthropic.WithAPIKey("test-key"),
						anthropic.WithEndpoint(endpoint),
						anthropic.WithModel(anthropic.ModelClaudeOpus48),
						anthropic.WithMaxRetries(0),
					)
				})
			},
			wantInput: 30, wantRead: 60, wantCreation: 10, wantPrompt: 100,
		},
		{
			name: "ollama anthropic inheritance",
			decode: func(t *testing.T) llm.Usage {
				return generateFixture(t, anthropicBody, func(endpoint string) llm.LLM {
					return ollama.New(
						ollama.WithEndpoint(endpoint),
						ollama.WithModel("llama-test"),
						ollama.WithMaxRetries(0),
					)
				})
			},
			wantInput: 30, wantRead: 60, wantCreation: 10, wantPrompt: 100,
		},
	}

	pricing := llm.PricingInfo{
		InputPrice:      5,
		OutputPrice:     30,
		CacheReadPrice:  0.5,
		CacheWritePrice: 6.25,
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := tt.decode(t)
			assert.Equal(t, tt.wantInput, usage.InputTokens)
			assert.Equal(t, tt.wantRead, usage.CacheReadInputTokens)
			assert.Equal(t, tt.wantCreation, usage.CacheCreationInputTokens)
			assert.Equal(t, tt.wantPrompt, usage.TotalInputTokens())

			cost := pricing.CostOf(&usage)
			const perMillion = 1_000_000.0
			wantInputCost := float64(tt.wantInput) * pricing.InputPrice / perMillion
			wantReadCost := float64(tt.wantRead) * pricing.CacheReadPrice / perMillion
			wantWriteCost := float64(tt.wantCreation) * pricing.CacheWritePrice / perMillion
			wantOutputCost := float64(20) * pricing.OutputPrice / perMillion
			assert.Equal(t, wantInputCost, cost.Input)
			assert.Equal(t, wantReadCost, cost.CacheRead)
			assert.Equal(t, wantWriteCost, cost.CacheWrite)
			assert.Equal(t, wantInputCost+wantReadCost+wantWriteCost+wantOutputCost, cost.Total)
		})
	}
}

func generateFixture(t *testing.T, body string, newProvider func(string) llm.LLM) llm.Usage {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	response, err := newProvider(server.URL).Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("hello")),
	)
	assert.NoError(t, err)
	return response.Usage
}
