package openairesponses

import (
	"bufio"
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

// Mock HTTP client for testing
type mockHTTPClient struct {
	*http.Client
	responses []mockResponse
	requests  []*http.Request
	index     int
}

type mockResponse struct {
	statusCode int
	body       string
	headers    map[string]string
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Store the request for inspection
	m.requests = append(m.requests, req)

	if m.index >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses available")
	}

	resp := m.responses[m.index]
	m.index++

	// Create response
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

func newMockClient(responses ...mockResponse) (*http.Client, *mockHTTPClient) {
	mock := &mockHTTPClient{
		Client:    &http.Client{},
		responses: responses,
	}
	// Return the embedded http.Client but with our custom Do method
	client := &http.Client{}
	client.Transport = &mockTransport{mock: mock}
	return client, mock
}

type mockTransport struct {
	mock *mockHTTPClient
}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.mock.Do(req)
}

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		opts     []Option
		expected func(*testing.T, *Provider)
	}{
		{
			name: "default configuration",
			opts: []Option{},
			expected: func(t *testing.T, p *Provider) {
				assert.Equal(t, DefaultModel, p.model)
				assert.Equal(t, DefaultEndpoint, p.endpoint)
				assert.NotNil(t, p.client)
				assert.Equal(t, 6, p.maxRetries)           // Default retry count
				assert.Equal(t, 2*time.Second, p.baseWait) // Default base wait
			},
		},
		{
			name: "with custom options",
			opts: []Option{
				WithAPIKey("test-key"),
				WithModel("gpt-4o"),
				WithEndpoint("https://custom.endpoint.com"),
			},
			expected: func(t *testing.T, p *Provider) {
				assert.Equal(t, "test-key", p.apiKey)
				assert.Equal(t, "gpt-4o", p.model)
				assert.Equal(t, "https://custom.endpoint.com", p.endpoint)
			},
		},
		{
			name: "with custom client",
			opts: []Option{
				WithClient(&http.Client{Timeout: 30 * time.Second}),
			},
			expected: func(t *testing.T, p *Provider) {
				assert.Equal(t, 30*time.Second, p.client.Timeout)
			},
		},
		{
			name: "with custom retry configuration",
			opts: []Option{
				WithMaxRetries(3),
				WithBaseWait(500 * time.Millisecond),
			},
			expected: func(t *testing.T, p *Provider) {
				assert.Equal(t, 3, p.maxRetries)
				assert.Equal(t, 500*time.Millisecond, p.baseWait)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := New(tt.opts...)
			tt.expected(t, provider)
		})
	}
}

func TestProvider_Name(t *testing.T) {
	provider := New(WithModel("gpt-4o"))
	assert.Equal(t, "openai-responses-gpt-4o", provider.Name())
}

func TestProvider_SupportsStreaming(t *testing.T) {
	provider := New()
	assert.True(t, provider.SupportsStreaming())
}

func TestProvider_buildRequest(t *testing.T) {
	tests := []struct {
		name     string
		provider *Provider
		config   *llm.Config
		expected func(*testing.T, *Request)
		wantErr  bool
	}{
		{
			name:     "basic request",
			provider: New(WithModel("gpt-4o")),
			config: &llm.Config{
				Messages: []*llm.Message{
					llm.NewUserTextMessage("Hello, world!"),
				},
				Temperature: &[]float64{0.7}[0],
			},
			expected: func(t *testing.T, req *Request) {
				assert.Equal(t, "gpt-4o", req.Model)
				assert.Equal(t, 0.7, *req.Temperature)
				assert.Equal(t, "Hello, world!", req.Input)
			},
		},
		{
			name:     "with tools",
			provider: New(),
			config: &llm.Config{
				Messages: []*llm.Message{
					llm.NewUserTextMessage("Use a tool"),
				},
				Tools: []llm.Tool{
					&mockTool{
						name:        "test_tool",
						description: "A test tool",
						schema: schema.Schema{
							Type: "object",
							Properties: map[string]*schema.Property{
								"param": {Type: "string"},
							},
						},
					},
				},
			},
			expected: func(t *testing.T, req *Request) {
				assert.Len(t, req.Tools, 1)
				assert.Equal(t, "function", req.Tools[0].Type)
				assert.Equal(t, "test_tool", req.Tools[0].Function.Name)
				assert.Equal(t, "A test tool", req.Tools[0].Function.Description)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := tt.provider.buildRequest(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.expected(t, request)
		})
	}
}

func TestProvider_convertMessagesToInput(t *testing.T) {
	provider := New()

	tests := []struct {
		name     string
		messages []*llm.Message
		expected interface{}
	}{
		{
			name: "single user text message",
			messages: []*llm.Message{
				llm.NewUserTextMessage("Hello"),
			},
			expected: "Hello",
		},
		{
			name: "multiple messages",
			messages: []*llm.Message{
				llm.NewUserTextMessage("Hello"),
				llm.NewAssistantTextMessage("Hi there"),
			},
			expected: []InputMessage{
				{
					Role: "user",
					Content: []InputContent{
						{Type: "input_text", Text: "Hello"},
					},
				},
				{
					Role: "assistant",
					Content: []InputContent{
						{Type: "input_text", Text: "Hi there"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := provider.convertMessagesToInput(tt.messages, &llm.Config{})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProvider_Generate(t *testing.T) {
	mockResp := Response{
		ID:     "resp-123",
		Object: "response",
		Model:  "gpt-4o",
		Status: "completed",
		Output: []OutputItem{
			{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{Type: "output_text", Text: "Hello, world!"},
				},
			},
		},
		Usage: &Usage{
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
		},
	}

	mockClient, mockData := newMockClient(mockResponse{
		statusCode: 200,
		body:       mustMarshal(mockResp),
	})

	provider := New(
		WithAPIKey("test-key"),
		WithClient(mockClient),
		WithMaxRetries(1),                // Faster retries for tests
		WithBaseWait(5*time.Millisecond), // Much shorter wait time for tests
	)

	ctx := context.Background()
	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Hello"),
	)

	require.NoError(t, err)
	assert.Equal(t, "resp-123", response.ID)
	assert.Equal(t, llm.Assistant, response.Role)
	assert.Equal(t, "Hello, world!", response.Message().Text())
	assert.Equal(t, 10, response.Usage.InputTokens)
	assert.Equal(t, 5, response.Usage.OutputTokens)

	// Verify request was made correctly
	require.Len(t, mockData.requests, 1)
	req := mockData.requests[0]
	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestProvider_Generate_Error(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantErr    string
	}{
		{
			name:       "rate limit error",
			statusCode: 429,
			body:       `{"error": {"message": "Rate limit exceeded"}}`,
			wantErr:    "Rate limit exceeded",
		},
		{
			name:       "unauthorized error",
			statusCode: 401,
			body:       `{"error": {"message": "Invalid API key"}}`,
			wantErr:    "Invalid API key",
		},
		{
			name:       "server error",
			statusCode: 500,
			body:       `{"error": {"message": "Internal server error"}}`,
			wantErr:    "Internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For non-recoverable errors (like 401), we only need 1 response
			// For recoverable errors (like 429, 500), we need enough for retries
			numResponses := 1
			if tt.statusCode == 429 || tt.statusCode == 500 {
				numResponses = 3 // 1 initial + 2 retries
			}

			responses := make([]mockResponse, numResponses)
			for i := range responses {
				responses[i] = mockResponse{
					statusCode: tt.statusCode,
					body:       tt.body,
				}
			}
			mockClient, _ := newMockClient(responses...)

			provider := New(
				WithAPIKey("test-key"),
				WithClient(mockClient),
				WithMaxRetries(2),                 // Faster retries for tests
				WithBaseWait(10*time.Millisecond), // Much shorter wait time for tests
			)

			// Use a context with timeout to prevent hanging in case of issues
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			_, err := provider.Generate(ctx,
				llm.WithUserTextMessage("Hello"),
			)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestProvider_Stream(t *testing.T) {
	// Mock streaming response
	streamData := []string{
		`data: {"type": "response", "response": {"id": "resp-123", "object": "response", "model": "gpt-4o", "status": "in_progress", "output": []}}`,
		`data: {"type": "response", "response": {"id": "resp-123", "output": [{"type": "message", "role": "assistant", "content": [{"type": "text", "text": "Hello"}]}]}}`,
		`data: {"type": "response", "response": {"id": "resp-123", "output": [{"type": "message", "role": "assistant", "content": [{"type": "text", "text": "Hello, world!"}]}], "usage": {"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}}}`,
		`data: [DONE]`,
	}

	mockClient, mockData := newMockClient(mockResponse{
		statusCode: 200,
		body:       strings.Join(streamData, "\n") + "\n",
		headers:    map[string]string{"Content-Type": "text/event-stream"},
	})

	provider := New(
		WithAPIKey("test-key"),
		WithClient(mockClient),
		WithMaxRetries(1),                // Faster retries for tests
		WithBaseWait(5*time.Millisecond), // Much shorter wait time for tests
	)

	ctx := context.Background()
	iterator, err := provider.Stream(ctx,
		llm.WithUserTextMessage("Hello"),
	)

	require.NoError(t, err)
	defer iterator.Close()

	// Collect events
	var events []*llm.Event
	for iterator.Next() {
		events = append(events, iterator.Event())
	}

	require.NoError(t, iterator.Err())
	assert.NotEmpty(t, events)

	// Verify we got at least a message start event
	assert.Equal(t, llm.EventTypeMessageStart, events[0].Type)

	// Verify request was made correctly
	require.Len(t, mockData.requests, 1)
	req := mockData.requests[0]
	assert.Equal(t, "text/event-stream", req.Header.Get("Accept"))
}

func TestProvider_convertResponse(t *testing.T) {
	provider := New()

	tests := []struct {
		name     string
		response *Response
		expected func(*testing.T, *llm.Response)
		wantErr  bool
	}{
		{
			name: "basic text response",
			response: &Response{
				ID:     "resp-123",
				Object: "response",
				Model:  "gpt-4o",
				Status: "completed",
				Output: []OutputItem{
					{
						Type: "message",
						Role: "assistant",
						Content: []OutputContent{
							{Type: "output_text", Text: "Hello, world!"},
						},
					},
				},
				Usage: &Usage{
					InputTokens:  10,
					OutputTokens: 5,
					TotalTokens:  15,
				},
			},
			expected: func(t *testing.T, resp *llm.Response) {
				assert.Equal(t, "resp-123", resp.ID)
				assert.Equal(t, llm.Assistant, resp.Role)
				assert.Equal(t, "Hello, world!", resp.Message().Text())
				assert.Equal(t, 10, resp.Usage.InputTokens)
				assert.Equal(t, 5, resp.Usage.OutputTokens)
			},
		},
		{
			name: "response with tool calls",
			response: &Response{
				ID:     "resp-456",
				Object: "response",
				Model:  "gpt-4o",
				Status: "completed",
				Output: []OutputItem{
					{
						Type:      "function_call",
						CallID:    "call-123",
						Name:      "test_tool",
						Arguments: `{"param": "value"}`,
					},
				},
			},
			expected: func(t *testing.T, resp *llm.Response) {
				toolCalls := resp.ToolCalls()
				require.Len(t, toolCalls, 1)
				assert.Equal(t, "call-123", toolCalls[0].ID)
				assert.Equal(t, "test_tool", toolCalls[0].Name)
				assert.Equal(t, `{"param": "value"}`, string(toolCalls[0].Input))
			},
		},
		{
			name: "error response",
			response: &Response{
				ID:     "resp-error",
				Object: "response",
				Status: "failed",
				Error: &ResponseError{
					Type:    "invalid_request",
					Message: "Invalid input",
				},
			},
			expected: func(t *testing.T, resp *llm.Response) {
				assert.Equal(t, "resp-error", resp.ID)
				assert.Equal(t, llm.Assistant, resp.Role)
				// Error responses have no content
				assert.Empty(t, resp.Content)
			},
			wantErr: false, // The convertResponse function doesn't check for errors yet
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := provider.convertResponse(tt.response)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.expected(t, result)
		})
	}
}

func TestProvider_buildTools(t *testing.T) {
	tests := []struct {
		name     string
		provider *Provider
		config   *llm.Config
		expected func(*testing.T, []Tool)
		wantErr  bool
	}{
		{
			name:     "no tools enabled",
			provider: New(),
			config: &llm.Config{
				Features: []string{},
			},
			expected: func(t *testing.T, tools []Tool) {
				assert.Empty(t, tools)
			},
		},
		{
			name:     "per-request tool enabling",
			provider: New(), // Provider has no tools enabled by default
			config: &llm.Config{
				Features: []string{
					FeatureWebSearch,
					FeatureImageGeneration,
				},
			},
			expected: func(t *testing.T, tools []Tool) {
				require.Len(t, tools, 2)

				// Find web search tool
				var webSearchTool *Tool
				var imageGenTool *Tool
				for i := range tools {
					if tools[i].Type == "web_search_preview" {
						webSearchTool = &tools[i]
					}
					if tools[i].Type == "image_generation" {
						imageGenTool = &tools[i]
					}
				}

				require.NotNil(t, webSearchTool, "Expected web search tool to be enabled")
				require.NotNil(t, imageGenTool, "Expected image generation tool to be enabled")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools, err := tt.provider.buildTools(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			tt.expected(t, tools)
		})
	}
}

func TestStreamIterator(t *testing.T) {
	// Test stream iterator with mock data
	streamData := `data: {"type": "response", "response": {"id": "resp-123", "output": [{"type": "message", "role": "assistant", "content": [{"type": "text", "text": "Hello"}]}]}}
data: [DONE]
`

	reader := strings.NewReader(streamData)
	iterator := &StreamIterator{
		reader: bufio.NewReader(reader),
		body:   io.NopCloser(reader),
	}

	// Test Next() and Event()
	hasNext := iterator.Next()
	assert.True(t, hasNext)

	event := iterator.Event()
	assert.NotNil(t, event)
	assert.Equal(t, llm.EventTypeMessageStart, event.Type)

	// Test Close()
	err := iterator.Close()
	assert.NoError(t, err)

	// Test Err()
	assert.NoError(t, iterator.Err())
}

func TestIntegration_BasicGeneration(t *testing.T) {
	// Skip if no API key is provided
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	provider := New()
	if provider.apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	ctx := context.Background()
	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Say 'hello' and nothing else"),
		llm.WithTemperature(0.0),
	)

	require.NoError(t, err)
	assert.NotEmpty(t, response.Message().Text())
	assert.Contains(t, strings.ToLower(response.Message().Text()), "hello")
}

func TestIntegration_StreamingGeneration(t *testing.T) {
	// Skip if no API key is provided
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	provider := New()
	if provider.apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping integration test")
	}

	ctx := context.Background()
	iterator, err := provider.Stream(ctx,
		llm.WithUserTextMessage("Count from 1 to 3, one number per line"),
		llm.WithTemperature(0.0),
	)

	require.NoError(t, err)
	defer iterator.Close()

	accum := llm.NewResponseAccumulator()
	eventCount := 0
	for iterator.Next() {
		event := iterator.Event()
		require.NoError(t, accum.AddEvent(event))
		eventCount++
	}

	require.NoError(t, iterator.Err())
	assert.Greater(t, eventCount, 0)

	response := accum.Response()
	require.NotNil(t, response)
	assert.NotEmpty(t, response.Message().Text())
}

// Helper types and functions

type mockTool struct {
	name        string
	description string
	schema      schema.Schema
}

func (m *mockTool) Name() string          { return m.name }
func (m *mockTool) Description() string   { return m.description }
func (m *mockTool) Schema() schema.Schema { return m.schema }
func (m *mockTool) Call(ctx context.Context, input string) (string, error) {
	return "mock result", nil
}

func mustMarshal(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestConvertDocumentContentToInput(t *testing.T) {
	provider := New()

	// Test with base64 data
	messages := []*llm.Message{
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.TextContent{Text: "What is this document about?"},
				&llm.DocumentContent{
					Title: "test.pdf",
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeBase64,
						MediaType: "application/pdf",
						Data:      "JVBERi0xLjQK...",
					},
				},
			},
		},
	}

	input, err := provider.convertMessagesToInput(messages, &llm.Config{})
	require.NoError(t, err)

	inputMessages, ok := input.([]InputMessage)
	require.True(t, ok)
	require.Len(t, inputMessages, 1)

	msg := inputMessages[0]
	require.Equal(t, "user", msg.Role)
	require.Len(t, msg.Content, 2)

	// Check text content
	require.Equal(t, "input_text", msg.Content[0].Type)
	require.Equal(t, "What is this document about?", msg.Content[0].Text)

	// Check file content
	require.Equal(t, "input_file", msg.Content[1].Type)
	require.Equal(t, "test.pdf", msg.Content[1].Filename)
	require.Equal(t, "data:application/pdf;base64,JVBERi0xLjQK...", msg.Content[1].FileData)
	require.Empty(t, msg.Content[1].FileID)
}

func TestConvertDocumentContentWithFileIDToInput(t *testing.T) {
	provider := New()

	// Test with file ID
	messages := []*llm.Message{
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.TextContent{Text: "Analyze this document"},
				&llm.DocumentContent{
					Title: "document.pdf",
					Source: &llm.ContentSource{
						Type:   llm.ContentSourceTypeFile,
						FileID: "file-abc123",
					},
				},
			},
		},
	}

	input, err := provider.convertMessagesToInput(messages, &llm.Config{})
	require.NoError(t, err)

	inputMessages, ok := input.([]InputMessage)
	require.True(t, ok)
	require.Len(t, inputMessages, 1)

	msg := inputMessages[0]
	require.Equal(t, "user", msg.Role)
	require.Len(t, msg.Content, 2)

	// Check text content
	require.Equal(t, "input_text", msg.Content[0].Type)
	require.Equal(t, "Analyze this document", msg.Content[0].Text)

	// Check file content
	require.Equal(t, "input_file", msg.Content[1].Type)
	require.Equal(t, "file-abc123", msg.Content[1].FileID)
	require.Equal(t, "document.pdf", msg.Content[1].Filename)
	require.Empty(t, msg.Content[1].FileData)
}
