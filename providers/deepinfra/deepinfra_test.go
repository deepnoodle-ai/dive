package deepinfra

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func TestRequestForwardsCacheKeyAndStructuredOutput(t *testing.T) {
	var request struct {
		PromptCacheKey string `json:"prompt_cache_key"`
		ResponseFormat struct {
			Type       string `json:"type"`
			JSONSchema struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Strict      bool           `json:"strict"`
				Schema      map[string]any `json:"schema"`
			} `json:"json_schema"`
		} `json:"response_format"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"answer\":\"OK\"}"}}]}`))
	}))
	defer server.Close()

	provider := New(WithAPIKey("test-key"), WithEndpoint(server.URL))
	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("Answer in JSON")),
		llm.WithPromptCacheKey("agent-session-123"),
		llm.WithResponseFormat(&llm.ResponseFormat{
			Type:        llm.ResponseFormatTypeJSONSchema,
			Name:        "answer",
			Description: "A short answer",
			Schema: &schema.Schema{
				Type:       "object",
				Properties: map[string]*schema.Property{"answer": {Type: "string"}},
				Required:   []string{"answer"},
			},
		}))
	assert.NoError(t, err)
	assert.Equal(t, response.Message().Text(), `{"answer":"OK"}`)
	assert.Equal(t, request.PromptCacheKey, "agent-session-123")
	assert.Equal(t, request.ResponseFormat.Type, "json_schema")
	assert.Equal(t, request.ResponseFormat.JSONSchema.Name, "answer")
	assert.Equal(t, request.ResponseFormat.JSONSchema.Description, "A short answer")
	assert.True(t, request.ResponseFormat.JSONSchema.Strict)
	assert.Equal(t, request.ResponseFormat.JSONSchema.Schema["additionalProperties"], false)
}

func TestRequestJSONModeAndInvalidSchema(t *testing.T) {
	var format map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ResponseFormat map[string]any `json:"response_format"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		format = request.ResponseFormat
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer server.Close()

	provider := New(WithAPIKey("test-key"), WithEndpoint(server.URL))
	message := llm.WithMessages(llm.NewUserTextMessage("Answer in JSON"))
	_, err := provider.Generate(context.Background(), message,
		llm.WithResponseFormat(&llm.ResponseFormat{Type: llm.ResponseFormatTypeJSON}))
	assert.NoError(t, err)
	assert.Equal(t, format["type"], "json_object")
	_, err = provider.Generate(context.Background(), message,
		llm.WithResponseFormat(&llm.ResponseFormat{Type: llm.ResponseFormatTypeJSONSchema, Name: "answer"}))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "schema is required")
}

func TestBinaryDocumentFailsBeforeRequest(t *testing.T) {
	provider := New(WithAPIKey("test-key"), WithEndpoint("http://localhost:1"))
	_, err := provider.Generate(context.Background(), llm.WithMessages(llm.NewUserMessage(
		&llm.DocumentContent{Source: &llm.ContentSource{
			Type: llm.ContentSourceTypeBase64, MediaType: "application/pdf", Data: "eA==",
		}},
	)))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "supports text documents only")
}

func TestCreateModelSendsNativeIDAndDeepInfraKey(t *testing.T) {
	t.Setenv("DEEP_INFRA_API_KEY", "deepinfra-key")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	requests := make(chan string, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/openai/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer deepinfra-key", r.Header.Get("Authorization"))
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, "system", request.Messages[0].Role)
		assert.Equal(t, "Answer briefly.", request.Messages[0].Content)
		assert.Equal(t, "user", request.Messages[1].Role)
		requests <- request.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`))
	}))
	defer server.Close()

	for _, id := range []string{ModelQwen38Flash, "publisher/new-chat-model"} {
		model := providers.CreateModel("deepinfra/"+id, server.URL+"/v1/openai/chat/completions")
		assert.NotNil(t, model)
		assert.Equal(t, "deepinfra", model.Name())
		response, err := model.Generate(context.Background(),
			llm.WithSystemPrompt("Answer briefly."),
			llm.WithMessages(llm.NewUserTextMessage("Reply with OK")))
		assert.NoError(t, err)
		assert.Equal(t, "OK", response.Message().Text())
		assert.Nil(t, response.Usage.Cost)
		assert.True(t, response.Usage.CostEstimateUnavailable)
		assert.Equal(t, id, <-requests)
	}
}

func TestGenerateUsesDeepInfraEstimatedCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","model":"zai-org/GLM-5.3-Flash","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10,"estimated_cost":0.0007}}`))
	}))
	defer server.Close()

	provider := New(WithAPIKey("test-key"), WithEndpoint(server.URL), WithModel(ModelGLM53Flash))
	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("Reply with OK")))
	assert.NoError(t, err)
	assert.NotNil(t, response.Usage.Cost)
	assert.Equal(t, 0.0007, response.Usage.Cost.Total)
	assert.Equal(t, "USD", response.Usage.Cost.Currency)
	assert.Equal(t, llm.CostSourceProviderEstimate, response.Usage.Cost.Source)
	assert.False(t, response.Usage.CostEstimateUnavailable)
}

func TestStreamUsesChatCompletionsAndEstimatedCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, ModelGLM53Flash, request.Model)
		assert.True(t, request.Stream)
		assert.Equal(t, "system", request.Messages[0].Role)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"zai-org/GLM-5.3-Flash","choices":[{"index":0,"delta":{"role":"assistant","content":"OK"}}]}`,
			``,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"zai-org/GLM-5.3-Flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			``,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"zai-org/GLM-5.3-Flash","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10,"estimated_cost":0.0007}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer server.Close()

	provider := New(WithAPIKey("test-key"), WithEndpoint(server.URL), WithModel(ModelGLM53Flash))
	stream, err := provider.Stream(context.Background(),
		llm.WithSystemPrompt("Answer briefly."),
		llm.WithMessages(llm.NewUserTextMessage("Reply with OK")))
	assert.NoError(t, err)
	defer stream.Close()
	acc := llm.NewResponseAccumulator()
	for stream.Next() {
		assert.NoError(t, acc.AddEvent(stream.Event()))
	}
	assert.NoError(t, stream.Err())
	response := acc.Response()
	assert.Equal(t, "OK", response.Message().Text())
	assert.NotNil(t, response.Usage.Cost)
	assert.Equal(t, 0.0007, response.Usage.Cost.Total)
	assert.Equal(t, llm.CostSourceProviderEstimate, response.Usage.Cost.Source)
	assert.False(t, response.Usage.CostEstimateUnavailable)
}

func TestStreamKeepsCostUnknownWhenEstimateMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"zai-org/GLM-5.3-Flash","choices":[{"index":0,"delta":{"role":"assistant","content":"OK"}}]}`,
			``,
			`data: {"id":"chatcmpl-1","object":"chat.completion.chunk","model":"zai-org/GLM-5.3-Flash","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")))
	}))
	defer server.Close()

	provider := New(WithAPIKey("test-key"), WithEndpoint(server.URL), WithModel(ModelGLM53Flash))
	stream, err := provider.Stream(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("Reply with OK")))
	assert.NoError(t, err)
	defer stream.Close()
	acc := llm.NewResponseAccumulator()
	for stream.Next() {
		assert.NoError(t, acc.AddEvent(stream.Event()))
	}
	assert.NoError(t, stream.Err())
	assert.Nil(t, acc.Response().Usage.Cost)
	assert.True(t, acc.Response().Usage.CostEstimateUnavailable)
}

func TestAPIKeyPrecedence(t *testing.T) {
	t.Setenv("DEEP_INFRA_API_KEY", "")
	t.Setenv("DEEPINFRA_API_KEY", "")
	t.Setenv("DEEPINFRA_TOKEN", "token")
	assert.Equal(t, "token", APIKey())
	t.Setenv("DEEPINFRA_API_KEY", "alias")
	assert.Equal(t, "alias", APIKey())
	t.Setenv("DEEP_INFRA_API_KEY", "preferred")
	assert.Equal(t, "preferred", APIKey())
}
