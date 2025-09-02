package eval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestRisorContent(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		globals     map[string]any
		expected    int
		expectError bool
	}{
		{
			name:     "simple text output",
			script:   `"Hello, World!"`,
			globals:  nil,
			expected: 1,
		},
		{
			name:     "text content block",
			script:   `{"type": "text", "text": "Hello from script"}`,
			globals:  nil,
			expected: 1,
		},
		{
			name: "multiple content blocks",
			script: `[
				{"type": "text", "text": "First block"},
				{"type": "text", "text": "Second block"}
			]`,
			globals:  nil,
			expected: 2,
		},
		{
			name:     "with globals",
			script:   `"Hello, " + name + "!"`,
			globals:  map[string]any{"name": "Alice"},
			expected: 1,
		},
		{
			name:        "invalid script",
			script:      `invalid syntax`,
			globals:     nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risorContent := &RisorContent{
				Dynamic: tt.script,
			}

			content, err := risorContent.Content(context.Background(), tt.globals)
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Len(t, content, tt.expected)
		})
	}
}

func TestScriptPathContent(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test shell script that outputs JSON
	shellScript := filepath.Join(tempDir, "test.sh")
	err := os.WriteFile(shellScript, []byte(`#!/bin/bash
echo '[{"type": "text", "text": "Hello from shell script"}]'`), 0755)
	require.NoError(t, err)

	tests := []struct {
		name        string
		scriptPath  string
		expected    int
		expectError bool
	}{
		{
			name:       "shell script",
			scriptPath: shellScript,
			expected:   1,
		},
		{
			name:        "non-existent script",
			scriptPath:  "/non/existent/script.sh",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scriptContent := &ScriptPathContent{
				DynamicFrom: tt.scriptPath,
				BasePath:    tempDir,
			}

			content, err := scriptContent.Content(context.Background(), nil)
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Len(t, content, tt.expected)
		})
	}
}

func TestRisorContentType(t *testing.T) {
	risorContent := &RisorContent{Dynamic: `"test"`}
	require.Equal(t, llm.ContentTypeDynamic, risorContent.Type())
}

func TestScriptPathContentType(t *testing.T) {
	scriptContent := &ScriptPathContent{DynamicFrom: "test.sh"}
	require.Equal(t, llm.ContentTypeDynamic, scriptContent.Type())
}
