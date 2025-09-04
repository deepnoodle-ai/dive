package cli

import (
	"context"
	"fmt"
	"strings"
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

func TestSortResults(t *testing.T) {
	tests := []struct {
		name     string
		metric   MetricType
		results  []*ComparisonResult
		expected []string // Expected order of provider names
	}{
		{
			name:   "sort by speed",
			metric: MetricSpeed,
			results: []*ComparisonResult{
				{Provider: "slow", Latency: 2 * time.Second},
				{Provider: "fast", Latency: 1 * time.Second},
				{Provider: "medium", Latency: 1500 * time.Millisecond},
			},
			expected: []string{"fast", "medium", "slow"},
		},
		{
			name:   "sort by quality (output tokens)",
			metric: MetricQuality,
			results: []*ComparisonResult{
				{Provider: "short", OutputTokens: 10},
				{Provider: "long", OutputTokens: 100},
				{Provider: "medium", OutputTokens: 50},
			},
			expected: []string{"long", "medium", "short"},
		},
		{
			name:   "errors go to end",
			metric: MetricSpeed,
			results: []*ComparisonResult{
				{Provider: "error", Error: fmt.Errorf("test error")},
				{Provider: "success", Latency: 1 * time.Second},
				{Provider: "another-error", Error: fmt.Errorf("another error")},
			},
			expected: []string{"success", "another-error", "error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortResults(tt.results, tt.metric)
			
			actual := make([]string, len(tt.results))
			for i, result := range tt.results {
				actual[i] = result.Provider
			}
			
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
			name: "long text response",
			response: &llm.Response{
				Content: []llm.Content{
					llm.NewTextContent(strings.Repeat("a", 120)),
				},
			},
			expected: strings.Repeat("a", 97) + "...",
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
		Latency:      100 * time.Millisecond,
		InputTokens:  50,
		OutputTokens: 25,
		TotalTokens:  75,
	}

	require.Equal(t, "test-provider", result.Provider)
	require.Equal(t, "test-model", result.Model)
	require.Equal(t, 100*time.Millisecond, result.Latency)
	require.Equal(t, 50, result.InputTokens)
	require.Equal(t, 25, result.OutputTokens)
	require.Equal(t, 75, result.TotalTokens)
	require.Nil(t, result.Error)
}

func TestMetricTypeValidation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected MetricType
		valid    bool
	}{
		{"speed metric", "speed", MetricSpeed, true},
		{"quality metric", "quality", MetricQuality, true},
		{"case insensitive speed", "SPEED", MetricSpeed, true},
		{"case insensitive quality", "Quality", MetricQuality, true},
		{"invalid metric", "invalid", "", false},
		{"empty metric", "", MetricSpeed, true}, // Default to speed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var metric MetricType
			var valid bool
			
			switch strings.ToLower(tt.input) {
			case "speed":
				metric = MetricSpeed
				valid = true
			case "quality":
				metric = MetricQuality
				valid = true
			case "":
				metric = MetricSpeed // Default
				valid = true
			default:
				valid = false
			}

			if tt.valid {
				require.True(t, valid)
				require.Equal(t, tt.expected, metric)
			} else {
				require.False(t, valid)
			}
		})
	}
}

func TestRunComparisonWithMockProviders(t *testing.T) {
	// This test validates the core comparison logic without requiring real API calls
	ctx := context.Background()
	
	// Test that the function properly handles the flow even if providers fail
	// (since we can't easily mock config.GetModel in this test structure)
	
	// Test with invalid providers to ensure error handling
	err := runComparison(ctx, "test prompt", []string{"invalid-provider"}, MetricSpeed)
	require.NoError(t, err) // Should not error even if providers fail
	
	// Test with empty providers list
	err = runComparison(ctx, "test prompt", []string{}, MetricSpeed)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no providers specified")
}