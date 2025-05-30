package openai

import (
	"encoding/json"
	"testing"

	"github.com/diveagents/dive/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		request  Request
		expected string
	}{
		{
			name: "minimal request",
			request: Request{
				Model: "gpt-4o",
				Input: []*InputMessage{
					{
						Role: "user",
						Content: []*InputContent{
							{Type: "input_text", Text: "Hello"},
						},
					},
				},
			},
			expected: `{"model":"gpt-4o","input":[{"role":"user","content":[{"type":"input_text","text":"Hello"}]}]}`,
		},
		{
			name: "full request with all fields",
			request: Request{
				Model: "gpt-4o",
				Input: []*InputMessage{
					{
						Role: "user",
						Content: []*InputContent{
							{Type: "input_text", Text: "Hello"},
						},
					},
				},
				Include:            []string{"usage", "reasoning"},
				Instructions:       "Be helpful",
				MaxOutputTokens:    1000,
				Metadata:           map[string]string{"session": "123"},
				PreviousResponseID: "resp_123",
				ServiceTier:        "default",
				Reasoning:          &ReasoningConfig{Effort: stringPtr("medium")},
				ParallelToolCalls:  boolPtr(true),
				Stream:             boolPtr(false),
				Temperature:        float64Ptr(0.7),
				Text:               &TextConfig{Format: TextFormat{Type: "text"}},
				ToolChoice:         "auto",
				Tools:              []any{map[string]any{"type": "function", "name": "test"}},
			},
			expected: `{"model":"gpt-4o","input":[{"role":"user","content":[{"type":"input_text","text":"Hello"}]}],"include":["usage","reasoning"],"instructions":"Be helpful","max_output_tokens":1000,"metadata":{"session":"123"},"previous_response_id":"resp_123","service_tier":"default","reasoning":{"effort":"medium"},"parallel_tool_calls":true,"stream":false,"temperature":0.7,"text":{"format":{"type":"text"}},"tool_choice":"auto","tools":[{"name":"test","type":"function"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.request)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))

			// Test unmarshaling
			var unmarshaled Request
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err)
			assert.Equal(t, tt.request, unmarshaled)
		})
	}
}

func TestInputContent_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		content  InputContent
		expected string
	}{
		{
			name:     "text content",
			content:  InputContent{Type: "input_text", Text: "Hello world"},
			expected: `{"type":"input_text","text":"Hello world"}`,
		},
		{
			name:     "image content",
			content:  InputContent{Type: "input_image", ImageURL: "https://example.com/image.jpg"},
			expected: `{"type":"input_image","image_url":"https://example.com/image.jpg"}`,
		},
		{
			name:     "file content",
			content:  InputContent{Type: "input_file", Filename: "test.pdf", FileData: "base64data"},
			expected: `{"type":"input_file","filename":"test.pdf","file_data":"base64data"}`,
		},
		{
			name:     "approval content",
			content:  InputContent{Type: "approval", Approve: boolPtr(true), ApprovalRequestID: "req_123"},
			expected: `{"type":"approval","approve":true,"approval_request_id":"req_123"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.content)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))

			// Test unmarshaling
			var unmarshaled InputContent
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err)
			assert.Equal(t, tt.content, unmarshaled)
		})
	}
}

func TestFunctionTool_JSONMarshaling(t *testing.T) {
	tool := FunctionTool{
		Type: "function",
		Name: "get_weather",
		Parameters: schema.Schema{
			Type: "object",
			Properties: map[string]*schema.Property{
				"location": {Type: "string", Description: "The city name"},
			},
			Required: []string{"location"},
		},
		Strict:      true,
		Description: "Get weather information",
	}

	expected := `{
		"type": "function",
		"name": "get_weather",
		"parameters": {
			"type": "object",
			"properties": {
				"location": {
					"type": "string",
					"description": "The city name"
				}
			},
			"required": ["location"]
		},
		"strict": true,
		"description": "Get weather information"
	}`

	// Test marshaling
	data, err := json.Marshal(tool)
	require.NoError(t, err)
	assert.JSONEq(t, expected, string(data))

	// Test unmarshaling
	var unmarshaled FunctionTool
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, tool, unmarshaled)
}

func TestUserLocation_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		location UserLocation
		expected string
	}{
		{
			name:     "approximate location",
			location: UserLocation{Type: "approximate", Country: "US"},
			expected: `{"type":"approximate","country":"US"}`,
		},
		{
			name: "exact location",
			location: UserLocation{
				Type:     "exact",
				City:     "San Francisco",
				Country:  "US",
				Region:   "California",
				Timezone: "America/Los_Angeles",
			},
			expected: `{"type":"exact","city":"San Francisco","country":"US","region":"California","timezone":"America/Los_Angeles"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.location)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))

			// Test unmarshaling
			var unmarshaled UserLocation
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err)
			assert.Equal(t, tt.location, unmarshaled)
		})
	}
}

func TestResponse_JSONMarshaling(t *testing.T) {
	response := Response{
		ID:        "resp_123",
		Object:    "response",
		CreatedAt: 1672531200,
		Status:    "completed",
		Model:     "gpt-4o",
		Output: []OutputItem{
			{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{Type: "text", Text: "Hello! How can I help you?"},
				},
			},
		},
		ParallelToolCalls: false,
		Store:             false,
		Temperature:       0.7,
		Tools:             []any{},
		TopP:              1.0,
		Truncation:        "auto",
		Usage: &Usage{
			InputTokens:  10,
			OutputTokens: 15,
			TotalTokens:  25,
		},
	}

	// Test marshaling
	data, err := json.Marshal(response)
	require.NoError(t, err)

	// Test unmarshaling
	var unmarshaled Response
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, response, unmarshaled)
}

func TestOutputItem_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name     string
		item     OutputItem
		expected string
	}{
		{
			name: "message output",
			item: OutputItem{
				Type: "message",
				Role: "assistant",
				Content: []OutputContent{
					{Type: "text", Text: "Hello!"},
				},
			},
			expected: `{"type":"message","role":"assistant","content":[{"type":"text","text":"Hello!"}]}`,
		},
		{
			name: "tool call output",
			item: OutputItem{
				Type:      "tool_call",
				CallID:    "call_123",
				Name:      "get_weather",
				Arguments: `{"location":"Boston"}`,
			},
			expected: `{"type":"tool_call","call_id":"call_123","name":"get_weather","arguments":"{\"location\":\"Boston\"}"}`,
		},
		{
			name: "web search output",
			item: OutputItem{
				Type: "web_search",
				Results: []WebSearchResult{
					{
						URL:         "https://example.com",
						Title:       "Example",
						Description: "An example page",
					},
				},
			},
			expected: `{"type":"web_search","results":[{"url":"https://example.com","title":"Example","description":"An example page"}]}`,
		},
		{
			name: "image generation output",
			item: OutputItem{
				Type:          "image_generation",
				RevisedPrompt: "A beautiful sunset",
				Result:        "base64imagedata",
			},
			expected: `{"type":"image_generation","revised_prompt":"A beautiful sunset","result":"base64imagedata"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			data, err := json.Marshal(tt.item)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))

			// Test unmarshaling
			var unmarshaled OutputItem
			err = json.Unmarshal(data, &unmarshaled)
			require.NoError(t, err)
			assert.Equal(t, tt.item, unmarshaled)
		})
	}
}

func TestStreamEvent_JSONMarshaling(t *testing.T) {
	event := StreamEvent{
		Type: "response.created",
		Response: &Response{
			ID:     "resp_123",
			Object: "response",
			Status: "in_progress",
		},
	}

	// Test marshaling
	data, err := json.Marshal(event)
	require.NoError(t, err)

	var unmarshaled StreamEvent
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, event, unmarshaled)
}

func TestOmitEmptyFields(t *testing.T) {
	// Test that omitempty works correctly
	request := Request{
		Model: "gpt-4o",
		Input: []*InputMessage{
			{
				Role: "user",
				Content: []*InputContent{
					{Type: "input_text", Text: "Hello"},
				},
			},
		},
		// All optional fields left as zero values
	}

	data, err := json.Marshal(request)
	require.NoError(t, err)

	// Should only contain non-empty fields
	expected := `{"model":"gpt-4o","input":[{"role":"user","content":[{"type":"input_text","text":"Hello"}]}]}`
	assert.JSONEq(t, expected, string(data))
}

func TestNullVsMissingFields(t *testing.T) {
	// Test difference between null and missing fields
	tests := []struct {
		name     string
		input    string
		expected Request
	}{
		{
			name:  "missing optional fields",
			input: `{"model":"gpt-4o","input":[]}`,
			expected: Request{
				Model: "gpt-4o",
				Input: []*InputMessage{},
			},
		},
		{
			name:  "null optional fields",
			input: `{"model":"gpt-4o","input":[],"temperature":null,"stream":null}`,
			expected: Request{
				Model:       "gpt-4o",
				Input:       []*InputMessage{},
				Temperature: nil,
				Stream:      nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var unmarshaled Request
			err := json.Unmarshal([]byte(tt.input), &unmarshaled)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, unmarshaled)
		})
	}
}

// Helper functions for pointer values
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func float64Ptr(f float64) *float64 {
	return &f
}
