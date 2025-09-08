package cli

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/stretchr/testify/require"
)

// MockLLM implements the llm.LLM interface for testing
type MockLLM struct {
	name     string
	model    string
	latency  time.Duration
	usage    llm.Usage
	response string
	err      error
}

func (m *MockLLM) Name() string {
	return m.name
}

func (m *MockLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	if m.latency > 0 {
		time.Sleep(m.latency)
	}

	if m.err != nil {
		return nil, m.err
	}

	return &llm.Response{
		ID:    "test-response-id",
		Model: m.model,
		Role:  llm.Assistant,
		Content: []llm.Content{
			llm.NewTextContent(m.response),
		},
		StopReason: "end_turn",
		Type:       "message",
		Usage:      m.usage,
	}, nil
}

func TestAverageDuration(t *testing.T) {
	tests := []struct {
		name      string
		durations []time.Duration
		expected  time.Duration
	}{
		{
			name:      "single duration",
			durations: []time.Duration{1 * time.Second},
			expected:  1 * time.Second,
		},
		{
			name:      "multiple durations",
			durations: []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second},
			expected:  2 * time.Second,
		},
		{
			name:      "empty slice",
			durations: []time.Duration{},
			expected:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := averageDuration(tt.durations)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestExtractResponsePreview(t *testing.T) {
	tests := []struct {
		name     string
		response *llm.Response
		expected string
	}{
		{
			name: "short text response",
			response: &llm.Response{
				Content: []llm.Content{
					llm.NewTextContent("Hello world"),
				},
			},
			expected: "Hello world",
		},
		{
			name: "no text content",
			response: &llm.Response{
				Content: []llm.Content{},
			},
			expected: "No text content",
		},
		{
			name: "tool use content",
			response: &llm.Response{
				Content: []llm.Content{
					&llm.ToolUseContent{
						Name:  "test_tool",
						Input: []byte(`{"test": "data"}`),
					},
				},
			},
			expected: "No text content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := extractResponsePreview(tt.response)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestComparisonResultStructure(t *testing.T) {
	result := &ComparisonResult{
		Provider:     "test-provider",
		Model:        "test-model",
		AvgLatency:   100 * time.Millisecond,
		InputTokens:  50,
		OutputTokens: 25,
		TotalTokens:  75,
		EstCost:      0.003,
		Runs:         3,
	}

	require.Equal(t, "test-provider", result.Provider)
	require.Equal(t, "test-model", result.Model)
	require.Equal(t, 100*time.Millisecond, result.AvgLatency)
	require.Equal(t, 50, result.InputTokens)
	require.Equal(t, 25, result.OutputTokens)
	require.Equal(t, 75, result.TotalTokens)
	require.Equal(t, 0.003, result.EstCost)
	require.Equal(t, 3, result.Runs)
	require.Nil(t, result.Error)
}

func TestFormatError(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		err      error
		expected string
	}{
		{
			name:     "api key error",
			provider: "anthropic",
			err:      fmt.Errorf("api key missing"),
			expected: "anthropic: API key missing/invalid",
		},
		{
			name:     "timeout error",
			provider: "openai",
			err:      fmt.Errorf("request timeout"),
			expected: "openai: Request timed out",
		},
		{
			name:     "generic error",
			provider: "groq",
			err:      fmt.Errorf("connection failed"),
			expected: "groq: connection failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := formatError(tt.provider, tt.err)
			require.Equal(t, tt.expected, actual)
		})
	}
}
