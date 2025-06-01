package openai

import (
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newInputMessage(role string, content string) *responses.EasyInputMessageParam {
	return &responses.EasyInputMessageParam{
		Role: responses.EasyInputMessageRole(role),
		Content: responses.EasyInputMessageContentUnionParam{
			OfInputItemContentList: []responses.ResponseInputContentUnionParam{
				{
					OfInputText: &responses.ResponseInputTextParam{
						Text: content,
					},
				},
			},
		},
	}
}

func newInputImage(imageURL string) *responses.EasyInputMessageParam {
	return &responses.EasyInputMessageParam{
		Role: responses.EasyInputMessageRole("user"),
		Content: responses.EasyInputMessageContentUnionParam{
			OfInputItemContentList: []responses.ResponseInputContentUnionParam{
				{OfInputImage: &responses.ResponseInputImageParam{
					ImageURL: openai.String(imageURL),
					Detail:   responses.ResponseInputImageDetailAuto,
				}},
			},
		},
	}
}

func TestConvertRequest(t *testing.T) {
	tests := []struct {
		name       string
		messages   []*llm.Message
		wantLen    int
		wantInputs responses.ResponseInputParam
	}{
		{
			name: "basic text message",
			messages: []*llm.Message{
				{
					Role: llm.User,
					Content: []llm.Content{
						&llm.TextContent{Text: "Hello, world!"},
					},
				},
			},
			wantLen: 1,
			wantInputs: responses.ResponseInputParam{
				responses.ResponseInputItemUnionParam{
					OfMessage: newInputMessage("user", "Hello, world!"),
				},
			},
		},
		{
			name: "multiple messages",
			messages: []*llm.Message{
				{
					Role: llm.User,
					Content: []llm.Content{
						&llm.TextContent{Text: "First message"},
					},
				},
				{
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.TextContent{Text: "Second message"},
					},
				},
			},
			wantLen: 2,
			wantInputs: responses.ResponseInputParam{
				responses.ResponseInputItemUnionParam{
					OfMessage: newInputMessage("user", "First message"),
				},
				responses.ResponseInputItemUnionParam{
					OfMessage: newInputMessage("assistant", "Second message"),
				},
			},
		},
		{
			name: "image content via URL",
			messages: []*llm.Message{
				{
					Role: llm.User,
					Content: []llm.Content{
						&llm.ImageContent{
							Source: &llm.ContentSource{
								Type: llm.ContentSourceTypeURL,
								URL:  "https://example.com/image.jpg",
							},
						},
					},
				},
			},
			wantLen: 1,
			wantInputs: responses.ResponseInputParam{
				responses.ResponseInputItemUnionParam{
					OfMessage: newInputImage("https://example.com/image.jpg"),
				},
			},
		},
		{
			name: "image generation content",
			messages: []*llm.Message{
				{
					Role: llm.User,
					Content: []llm.Content{
						&llm.ImageContent{
							Source: &llm.ContentSource{
								Type:             llm.ContentSourceTypeBase64,
								MediaType:        "image/jpeg",
								Data:             "base64data",
								GenerationID:     "ig_888",
								GenerationStatus: "completed",
							},
						},
					},
				},
			},
			wantLen: 1,
			wantInputs: responses.ResponseInputParam{
				responses.ResponseInputItemParamOfImageGenerationCall(
					"ig_888",
					"",
					"completed",
				),
			},
		},
		{
			name: "tool use content",
			messages: []*llm.Message{
				{
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{
							ID:    "tool_123",
							Name:  "get_weather",
							Input: []byte(`{"city": "NYC"}`),
						},
					},
				},
			},
			wantLen: 1,
			wantInputs: responses.ResponseInputParam{
				responses.ResponseInputItemParamOfFunctionCall(
					`{"city": "NYC"}`, // arguments
					"tool_123",        // callID
					"get_weather",     // name
				),
			},
		},
		{
			name: "tool result content",
			messages: []*llm.Message{
				{
					Role: llm.User,
					Content: []llm.Content{
						&llm.ToolResultContent{
							ToolUseID: "tool_123",
							Content:   "The weather is sunny",
						},
					},
				},
			},
			wantLen: 1,
			wantInputs: responses.ResponseInputParam{
				responses.ResponseInputItemParamOfFunctionCallOutput(
					"tool_123",             // toolUseID
					"The weather is sunny", // output
				),
			},
		},
		{
			name: "empty messages are skipped",
			messages: []*llm.Message{
				{
					Role:    llm.User,
					Content: []llm.Content{},
				},
				{
					Role: llm.User,
					Content: []llm.Content{
						&llm.TextContent{Text: "Non-empty message"},
					},
				},
			},
			wantLen: 1,
			wantInputs: responses.ResponseInputParam{
				responses.ResponseInputItemUnionParam{
					OfMessage: newInputMessage("user", "Non-empty message"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := convertRequest(tt.messages)
			require.NoError(t, err)
			require.Len(t, result, tt.wantLen)
			if tt.wantInputs != nil {
				require.Equal(t, tt.wantInputs, result)
			}
		})
	}
}

func TestConvertRequestErrors(t *testing.T) {
	tests := []struct {
		name     string
		messages []*llm.Message
		wantErr  string
	}{
		{
			name: "image content without source",
			messages: []*llm.Message{
				{
					Role: llm.User,
					Content: []llm.Content{
						&llm.ImageContent{Source: nil},
					},
				},
			},
			wantErr: "image content source is required",
		},
		{
			name: "document content without source",
			messages: []*llm.Message{
				{
					Role: llm.User,
					Content: []llm.Content{
						&llm.DocumentContent{Source: nil},
					},
				},
			},
			wantErr: "document content source is required",
		},
		{
			name: "base64 image without media type",
			messages: []*llm.Message{
				{
					Role: llm.User,
					Content: []llm.Content{
						&llm.ImageContent{
							Source: &llm.ContentSource{
								Type: llm.ContentSourceTypeBase64,
								Data: "somedata",
							},
						},
					},
				},
			},
			wantErr: "media type and data are required for base64 image content",
		},
		{
			name: "document with URL source",
			messages: []*llm.Message{
				{
					Role: llm.User,
					Content: []llm.Content{
						&llm.DocumentContent{
							Source: &llm.ContentSource{
								Type: llm.ContentSourceTypeURL,
								URL:  "https://example.com/doc.pdf",
							},
						},
					},
				},
			},
			wantErr: "URL-based document content is not supported by OpenAI Responses API",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := convertRequest(tt.messages)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestConvertResponse(t *testing.T) {
	// Create a mock response that matches the actual SDK structure
	mockResponse := &responses.Response{
		ID:     "resp_123",
		Model:  openai.ChatModelGPT4,
		Status: "completed",
		Output: []responses.ResponseOutputItemUnion{
			{Type: "message"}, // Simplified - actual response would have more data
		},
		Usage: responses.ResponseUsage{
			InputTokens:  10,
			OutputTokens: 8,
			InputTokensDetails: responses.ResponseUsageInputTokensDetails{
				CachedTokens: 2,
			},
		},
	}

	result, err := convertResponse(mockResponse)

	require.NoError(t, err)
	assert.Equal(t, "resp_123", result.ID)
	assert.Equal(t, "gpt-4", result.Model)
	assert.Equal(t, llm.Assistant, result.Role)
	assert.Equal(t, 10, result.Usage.InputTokens)
	assert.Equal(t, 8, result.Usage.OutputTokens)
	assert.Equal(t, 2, result.Usage.CacheReadInputTokens)
}

func TestConvertResponseWithDifferentModels(t *testing.T) {
	tests := []struct {
		name     string
		model    openai.ChatModel
		expected string
	}{
		{
			name:     "GPT-4o model",
			model:    openai.ChatModelGPT4o,
			expected: "gpt-4o",
		},
		{
			name:     "GPT-4o Mini model",
			model:    openai.ChatModelGPT4oMini,
			expected: "gpt-4o-mini",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockResponse := &responses.Response{
				ID:     "resp_model_test",
				Model:  tt.model,
				Status: "completed",
				Output: []responses.ResponseOutputItemUnion{},
				Usage: responses.ResponseUsage{
					InputTokens:  5,
					OutputTokens: 3,
				},
			}

			result, err := convertResponse(mockResponse)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Model)
		})
	}
}

func TestConvertResponseWithVariedUsage(t *testing.T) {
	tests := []struct {
		name              string
		inputTokens       int64
		outputTokens      int64
		cachedTokens      int64
		expectedInput     int
		expectedOutput    int
		expectedCacheRead int
	}{
		{
			name:              "no cached tokens",
			inputTokens:       100,
			outputTokens:      50,
			cachedTokens:      0,
			expectedInput:     100,
			expectedOutput:    50,
			expectedCacheRead: 0,
		},
		{
			name:              "with cached tokens",
			inputTokens:       200,
			outputTokens:      75,
			cachedTokens:      25,
			expectedInput:     200,
			expectedOutput:    75,
			expectedCacheRead: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockResponse := &responses.Response{
				ID:     "resp_usage_test",
				Model:  openai.ChatModelGPT4,
				Status: "completed",
				Output: []responses.ResponseOutputItemUnion{},
				Usage: responses.ResponseUsage{
					InputTokens:  tt.inputTokens,
					OutputTokens: tt.outputTokens,
					InputTokensDetails: responses.ResponseUsageInputTokensDetails{
						CachedTokens: tt.cachedTokens,
					},
				},
			}

			result, err := convertResponse(mockResponse)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedInput, result.Usage.InputTokens)
			assert.Equal(t, tt.expectedOutput, result.Usage.OutputTokens)
			assert.Equal(t, tt.expectedCacheRead, result.Usage.CacheReadInputTokens)
		})
	}
}

func TestConvertResponseWithCompletedStatus(t *testing.T) {
	mockResponse := &responses.Response{
		ID:     "resp_completed_test",
		Model:  openai.ChatModelGPT4,
		Status: "completed",
		Output: []responses.ResponseOutputItemUnion{
			{Type: "message"},
		},
		Usage: responses.ResponseUsage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}

	result, err := convertResponse(mockResponse)
	require.NoError(t, err)
	assert.Equal(t, "end_turn", result.StopReason)
}

func TestConvertResponseWithToolCalls(t *testing.T) {
	tests := []struct {
		name       string
		outputType string
	}{
		{"function call", "function_call"},
		{"image generation call", "image_generation_call"},
		{"mcp call", "mcp_call"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockResponse := &responses.Response{
				ID:     "resp_tool_test",
				Model:  openai.ChatModelGPT4,
				Status: "completed",
				Output: []responses.ResponseOutputItemUnion{
					{Type: tt.outputType},
				},
				Usage: responses.ResponseUsage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			}

			result, err := convertResponse(mockResponse)
			require.NoError(t, err)
			assert.Equal(t, "tool_use", result.StopReason)
		})
	}
}

func TestConvertResponseWithMultipleOutputItems(t *testing.T) {
	mockResponse := &responses.Response{
		ID:     "resp_multi_test",
		Model:  openai.ChatModelGPT4,
		Status: "completed",
		Output: []responses.ResponseOutputItemUnion{
			{Type: "message"},
			{Type: "function_call"},
			{Type: "image_generation_call"},
		},
		Usage: responses.ResponseUsage{
			InputTokens:  30,
			OutputTokens: 20,
		},
	}

	result, err := convertResponse(mockResponse)
	require.NoError(t, err)
	assert.Equal(t, "resp_multi_test", result.ID)
	// Should be tool_use because there are function and image generation calls
	assert.Equal(t, "tool_use", result.StopReason)
}

func TestConvertResponseWithEmptyOutput(t *testing.T) {
	mockResponse := &responses.Response{
		ID:     "resp_empty_test",
		Model:  openai.ChatModelGPT4,
		Status: "completed",
		Output: []responses.ResponseOutputItemUnion{},
		Usage: responses.ResponseUsage{
			InputTokens:  5,
			OutputTokens: 0,
		},
	}

	result, err := convertResponse(mockResponse)
	require.NoError(t, err)
	assert.Equal(t, "resp_empty_test", result.ID)
	assert.Equal(t, "end_turn", result.StopReason)
	assert.Len(t, result.Content, 0)
}

func TestConvertResponseBasicFields(t *testing.T) {
	mockResponse := &responses.Response{
		ID:     "resp_fields_test",
		Model:  openai.ChatModelGPT4,
		Status: "completed",
		Output: []responses.ResponseOutputItemUnion{
			{Type: "message"},
		},
		Usage: responses.ResponseUsage{
			InputTokens:  15,
			OutputTokens: 12,
			InputTokensDetails: responses.ResponseUsageInputTokensDetails{
				CachedTokens: 3,
			},
		},
	}

	result, err := convertResponse(mockResponse)

	require.NoError(t, err)
	assert.Equal(t, "resp_fields_test", result.ID)
	assert.Equal(t, "gpt-4", result.Model)
	assert.Equal(t, llm.Assistant, result.Role)
	assert.Equal(t, "end_turn", result.StopReason)
	assert.NotNil(t, result.Content)
	assert.Equal(t, 15, result.Usage.InputTokens)
	assert.Equal(t, 12, result.Usage.OutputTokens)
	assert.Equal(t, 3, result.Usage.CacheReadInputTokens)
}
