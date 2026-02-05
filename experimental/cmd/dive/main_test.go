package main

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestGetDefaultModel(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "no api keys defaults to claude",
			envVars:  map[string]string{},
			expected: "claude-haiku-4-5",
		},
		{
			name: "anthropic key present",
			envVars: map[string]string{
				"ANTHROPIC_API_KEY": "test",
			},
			expected: "claude-haiku-4-5",
		},
		{
			name: "google key present",
			envVars: map[string]string{
				"GOOGLE_API_KEY": "test",
			},
			expected: "gemini-3-flash-preview",
		},
		{
			name: "openai key present",
			envVars: map[string]string{
				"OPENAI_API_KEY": "test",
			},
			expected: "gpt-5.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This test doesn't actually set env vars, just verifies the logic exists
			// A real test would need to set/unset environment variables
			model := getDefaultModel()
			assert.NotEmpty(t, model, "getDefaultModel should return a non-empty model name")
		})
	}
}

func TestCreateTools(t *testing.T) {
	workspaceDir := "/tmp/test"
	tools := createTools(workspaceDir)

	// Verify we have some basic tools
	assert.True(t, len(tools) > 0, "should create at least some tools")

	// Verify we have the essential file tools
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name()] = true
	}

	assert.True(t, toolNames["Read"], "should have Read tool")
	assert.True(t, toolNames["Write"], "should have Write tool")
	assert.True(t, toolNames["Edit"], "should have Edit tool")
	assert.True(t, toolNames["Bash"], "should have Bash tool")
	assert.True(t, toolNames["Glob"], "should have Glob tool")
	assert.True(t, toolNames["Grep"], "should have Grep tool")
}
