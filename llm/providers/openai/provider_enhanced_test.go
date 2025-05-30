package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Enhanced mock HTTP client for comprehensive testing
type enhancedMockClient struct {
	responses      []enhancedMockResponse
	requests       []*http.Request
	requestBodies  []string
	index          int
	simulateDelays bool
	delayDuration  time.Duration
	failAfterCount int
	failureCount   int
}

type enhancedMockResponse struct {
	statusCode  int
	body        string
	headers     map[string]string
	isStreaming bool
}

func newEnhancedMockClient(responses ...enhancedMockResponse) (*http.Client, *enhancedMockClient) {
	mock := &enhancedMockClient{
		responses: responses,
	}
	client := &http.Client{}
	client.Transport = &enhancedMockTransport{mock: mock}
	return client, mock
}

type enhancedMockTransport struct {
	mock *enhancedMockClient
}

func (t *enhancedMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Store request for inspection
	t.mock.requests = append(t.mock.requests, req)

	// Read and store request body
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		t.mock.requestBodies = append(t.mock.requestBodies, string(body))
		req.Body = io.NopCloser(strings.NewReader(string(body)))
	}

	// Simulate failures
	if t.mock.failAfterCount > 0 && t.mock.failureCount < t.mock.failAfterCount {
		t.mock.failureCount++
		return nil, fmt.Errorf("simulated network error")
	}

	// Simulate delays with context awareness
	if t.mock.simulateDelays {
		select {
		case <-time.After(t.mock.delayDuration):
			// Delay completed normally
		case <-req.Context().Done():
			// Context was cancelled during delay
			return nil, req.Context().Err()
		}
	}

	if t.mock.index >= len(t.mock.responses) {
		return nil, fmt.Errorf("no more mock responses available")
	}

	resp := t.mock.responses[t.mock.index]
	t.mock.index++

	httpResp := &http.Response{
		StatusCode: resp.statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(resp.body)),
	}

	for k, v := range resp.headers {
		httpResp.Header.Set(k, v)
	}

	return httpResp, nil
}

func TestProvider_CompleteEndToEndRequest(t *testing.T) {
	tests := []struct {
		name         string
		config       *llm.Config
		mockResponse string
		validate     func(t *testing.T, mock *enhancedMockClient, response *llm.Response)
	}{
		{
			name: "basic text generation",
			config: &llm.Config{
				Messages: []*llm.Message{
					llm.NewMessage(llm.System, []llm.Content{&llm.TextContent{Text: "You are a helpful assistant."}}),
					llm.NewUserTextMessage("Hello, how are you?"),
				},
				Temperature: &[]float64{0.7}[0],
				MaxTokens:   &[]int{1000}[0],
			},
			mockResponse: `{
				"id": "resp_123",
				"object": "response",
				"created_at": 1234567890,
				"status": "completed",
				"model": "gpt-4o",
				"output": [{
					"type": "message",
					"role": "assistant",
					"content": [{
						"type": "output_text",
						"text": "Hello! I'm doing well, thank you for asking. How can I help you today?"
					}]
				}],
				"usage": {
					"input_tokens": 25,
					"output_tokens": 18,
					"total_tokens": 43
				}
			}`,
			validate: func(t *testing.T, mock *enhancedMockClient, response *llm.Response) {
				// Validate request was properly formed
				require.Len(t, mock.requestBodies, 1)
				var req Request
				err := json.Unmarshal([]byte(mock.requestBodies[0]), &req)
				require.NoError(t, err)

				assert.Equal(t, "gpt-4o", req.Model)
				assert.Equal(t, 0.7, *req.Temperature)
				assert.Equal(t, 1000, req.MaxOutputTokens)
				assert.Len(t, req.Input, 2)
				assert.Equal(t, "system", req.Input[0].Role)
				assert.Equal(t, "user", req.Input[1].Role)

				// Validate response conversion
				require.Len(t, response.Content, 1)
				textContent, ok := response.Content[0].(*llm.TextContent)
				require.True(t, ok)
				assert.Contains(t, textContent.Text, "Hello! I'm doing well")
			},
		},
		{
			name: "tool use request",
			config: &llm.Config{
				Messages: []*llm.Message{
					llm.NewUserTextMessage("What's the weather like?"),
				},
				Tools: []llm.Tool{
					&mockTool{
						name:        "get_weather",
						description: "Get current weather information",
						schema: schema.Schema{
							Type: "object",
							Properties: map[string]*schema.Property{
								"location": {Type: "string", Description: "The city name"},
							},
							Required: []string{"location"},
						},
					},
				},
				ToolChoice: "auto",
			},
			mockResponse: `{
				"id": "resp_456",
				"object": "response",
				"created_at": 1234567890,
				"status": "completed",
				"model": "gpt-4o",
				"output": [{
					"type": "function_call",
					"call_id": "call_123",
					"name": "get_weather",
					"arguments": "{\"location\": \"San Francisco\"}"
				}],
				"usage": {
					"input_tokens": 30,
					"output_tokens": 15,
					"total_tokens": 45
				}
			}`,
			validate: func(t *testing.T, mock *enhancedMockClient, response *llm.Response) {
				// Validate tool configuration in request
				var req Request
				err := json.Unmarshal([]byte(mock.requestBodies[0]), &req)
				require.NoError(t, err)

				assert.Len(t, req.Tools, 1)
				tool := req.Tools[0].(map[string]any)
				assert.Equal(t, "function", tool["type"])
				assert.Equal(t, "get_weather", tool["name"])
				assert.Equal(t, "auto", req.ToolChoice)

				// Validate tool use response
				require.Len(t, response.Content, 1)
				toolContent, ok := response.Content[0].(*llm.ToolUseContent)
				require.True(t, ok)
				assert.Equal(t, "call_123", toolContent.ID)
				assert.Equal(t, "get_weather", toolContent.Name)
				assert.Contains(t, string(toolContent.Input), "San Francisco")
			},
		},
		{
			name: "multimodal request with document",
			config: &llm.Config{
				Messages: []*llm.Message{
					{
						Role: llm.User,
						Content: []llm.Content{
							&llm.TextContent{Text: "Analyze this document"},
							&llm.DocumentContent{
								Source: &llm.ContentSource{
									Type:      llm.ContentSourceTypeBase64,
									MediaType: "application/pdf",
									Data:      "fake pdf content",
								},
							},
						},
					},
				},
			},
			mockResponse: `{
				"id": "resp_789",
				"object": "response",
				"created_at": 1234567890,
				"status": "completed",
				"model": "gpt-4o",
				"output": [{
					"type": "message",
					"role": "assistant",
					"content": [{
						"type": "output_text",
						"text": "I can see this is a PDF document. Based on the content, I can provide the following analysis..."
					}]
				}],
				"usage": {
					"input_tokens": 100,
					"output_tokens": 50,
					"total_tokens": 150
				}
			}`,
			validate: func(t *testing.T, mock *enhancedMockClient, response *llm.Response) {
				// Validate multimodal content in request
				var req Request
				err := json.Unmarshal([]byte(mock.requestBodies[0]), &req)
				require.NoError(t, err)

				assert.Len(t, req.Input, 1)
				assert.Len(t, req.Input[0].Content, 2)
				assert.Equal(t, "input_text", req.Input[0].Content[0].Type)
				assert.Equal(t, "input_file", req.Input[0].Content[1].Type)
			},
		},
		{
			name: "reasoning effort for o-series model",
			config: &llm.Config{
				Messages: []*llm.Message{
					llm.NewUserTextMessage("Solve this complex problem step by step"),
				},
				ReasoningEffort: "high",
			},
			mockResponse: `{
				"id": "resp_o1",
				"object": "response",
				"created_at": 1234567890,
				"status": "completed",
				"model": "o-4o",
				"output": [{
					"type": "message",
					"role": "assistant",
					"content": [{
						"type": "output_text",
						"text": "Let me break this down step by step..."
					}]
				}],
				"reasoning": {
					"effort": "high",
					"summary": "The model carefully analyzed the problem"
				},
				"usage": {
					"input_tokens": 20,
					"output_tokens": 100,
					"total_tokens": 120,
					"output_tokens_details": {
						"reasoning_tokens": 500
					}
				}
			}`,
			validate: func(t *testing.T, mock *enhancedMockClient, response *llm.Response) {
				// Note: This test needs the provider to be configured with an o-series model
				// For now, just validate the response parsing
				require.Len(t, response.Content, 1)
				textContent, ok := response.Content[0].(*llm.TextContent)
				require.True(t, ok)
				assert.Contains(t, textContent.Text, "step by step")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, mock := newEnhancedMockClient(enhancedMockResponse{
				statusCode: 200,
				body:       tt.mockResponse,
			})

			provider := New(
				WithAPIKey("test-key"),
				WithClient(client),
				WithModel("gpt-4o"),
			)

			// Build options dynamically based on what's set in the config
			options := []llm.Option{llm.WithMessages(tt.config.Messages...)}

			if tt.config.Temperature != nil {
				options = append(options, llm.WithTemperature(*tt.config.Temperature))
			}
			if tt.config.MaxTokens != nil {
				options = append(options, llm.WithMaxTokens(*tt.config.MaxTokens))
			}
			if len(tt.config.Tools) > 0 {
				options = append(options, llm.WithTools(tt.config.Tools...))
			}
			if tt.config.ToolChoice != "" {
				options = append(options, llm.WithToolChoice(tt.config.ToolChoice))
			}
			if tt.config.ReasoningEffort != "" {
				options = append(options, llm.WithReasoningEffort(tt.config.ReasoningEffort))
			}

			response, err := provider.Generate(context.Background(), options...)
			require.NoError(t, err)
			require.NotNil(t, response)

			tt.validate(t, mock, response)
		})
	}
}

func TestProvider_StreamingEndToEnd(t *testing.T) {
	streamingResponse := `data: {"type": "response", "response": {"id": "resp_stream_123", "model": "gpt-4o", "output": []}}

data: {"type": "response", "response": {"id": "resp_stream_123", "model": "gpt-4o", "output": [{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "Hello"}]}]}}

data: {"type": "response", "response": {"id": "resp_stream_123", "model": "gpt-4o", "output": [{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "Hello there!"}]}]}}

data: {"type": "response", "response": {"id": "resp_stream_123", "model": "gpt-4o", "output": [{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "Hello there!"}]}], "usage": {"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}}}

data: [DONE]
`

	client, mock := newEnhancedMockClient(enhancedMockResponse{
		statusCode:  200,
		body:        streamingResponse,
		isStreaming: true,
	})

	provider := New(
		WithAPIKey("test-key"),
		WithClient(client),
	)

	stream, err := provider.Stream(context.Background(), llm.WithUserTextMessage("Hello"))
	require.NoError(t, err)
	require.NotNil(t, stream)

	// Validate request had stream: true
	var req Request
	err = json.Unmarshal([]byte(mock.requestBodies[0]), &req)
	require.NoError(t, err)
	assert.True(t, *req.Stream)

	// Collect streaming events
	var events []*llm.Event
	for stream.Next() {
		event := stream.Event()
		events = append(events, event)
	}
	require.NoError(t, stream.Err())

	// Validate we received the expected events
	assert.NotEmpty(t, events)

	// Should have response created, content events, and response done
	hasMessageStart := false
	hasContentBlockStart := false
	hasContentDelta := false
	hasMessageStop := false

	for _, event := range events {
		switch event.Type {
		case llm.EventTypeMessageStart:
			hasMessageStart = true
		case llm.EventTypeContentBlockStart:
			hasContentBlockStart = true
		case llm.EventTypeContentBlockDelta:
			hasContentDelta = true
		case llm.EventTypeMessageStop:
			hasMessageStop = true
		}
	}

	assert.True(t, hasMessageStart, "Should have message_start event")
	assert.True(t, hasContentBlockStart, "Should have content_block_start event")
	assert.True(t, hasContentDelta, "Should have content delta events")
	assert.True(t, hasMessageStop, "Should have message_stop event")
}

func TestProvider_ErrorHandling(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		responseBody string
		headers      map[string]string
		expectError  string
	}{
		{
			name:       "rate limit error",
			statusCode: 429,
			responseBody: `{
				"error": {
					"message": "Rate limit exceeded",
					"type": "rate_limit_exceeded"
				}
			}`,
			headers: map[string]string{
				"Retry-After": "5",
			},
			expectError: "Rate limit exceeded",
		},
		{
			name:       "authentication error",
			statusCode: 401,
			responseBody: `{
				"error": {
					"message": "Invalid API key",
					"type": "invalid_authentication"
				}
			}`,
			expectError: "Invalid API key",
		},
		{
			name:       "model not found",
			statusCode: 404,
			responseBody: `{
				"error": {
					"message": "Model not found",
					"type": "model_not_found"
				}
			}`,
			expectError: "Model not found",
		},
		{
			name:       "server error",
			statusCode: 500,
			responseBody: `{
				"error": {
					"message": "Internal server error",
					"type": "server_error"
				}
			}`,
			expectError: "Internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newEnhancedMockClient(enhancedMockResponse{
				statusCode: tt.statusCode,
				body:       tt.responseBody,
				headers:    tt.headers,
			})

			provider := New(
				WithAPIKey("test-key"),
				WithClient(client),
				WithMaxRetries(0), // No retries for error tests to avoid exhausting responses
			)

			_, err := provider.Generate(context.Background(), llm.WithUserTextMessage("Test message"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}

func TestProvider_RetryLogic(t *testing.T) {
	// First request fails with 429, second succeeds
	successResponse := `{
		"id": "resp_retry",
		"object": "response",
		"created_at": 1234567890,
		"status": "completed",
		"model": "gpt-4o",
		"output": [{
			"type": "message",
			"role": "assistant",
			"content": [{
				"type": "output_text",
				"text": "Success after retry"
			}]
		}]
	}`

	client, mock := newEnhancedMockClient(
		enhancedMockResponse{
			statusCode: 429,
			body:       `{"error": {"message": "Rate limit exceeded", "type": "rate_limit_exceeded"}}`,
			headers:    map[string]string{"Retry-After": "1"},
		},
		enhancedMockResponse{
			statusCode: 200,
			body:       successResponse,
		},
	)

	provider := New(
		WithAPIKey("test-key"),
		WithClient(client),
		WithMaxRetries(3),
		WithBaseWait(10*time.Millisecond), // Fast retry for testing
	)

	response, err := provider.Generate(context.Background(), llm.WithUserTextMessage("Test retry"))
	require.NoError(t, err)
	require.NotNil(t, response)

	// Should have made 2 requests (1 failure + 1 success)
	assert.Len(t, mock.requests, 2)

	textContent, ok := response.Content[0].(*llm.TextContent)
	require.True(t, ok)
	assert.Equal(t, "Success after retry", textContent.Text)
}

func TestProvider_ContextCancellation(t *testing.T) {
	// Create a mock that delays longer than the context timeout
	client, mock := newEnhancedMockClient(enhancedMockResponse{
		statusCode: 200,
		body:       `{"id": "test"}`,
	})
	mock.simulateDelays = true
	mock.delayDuration = 100 * time.Millisecond // Delay longer than timeout

	provider := New(
		WithAPIKey("test-key"),
		WithClient(client),
	)

	// Use a very short timeout to ensure it triggers
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := provider.Generate(ctx, llm.WithUserTextMessage("This should timeout"))
	require.Error(t, err)
	// The error could be either "context deadline exceeded" or "context canceled"
	assert.True(t,
		strings.Contains(err.Error(), "context deadline exceeded") ||
			strings.Contains(err.Error(), "context canceled"),
		"Expected context timeout error, got: %v", err)
}
