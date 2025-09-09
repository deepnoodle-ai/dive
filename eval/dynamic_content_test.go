package eval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/stretchr/testify/require"
)

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

func TestScriptPathContentType(t *testing.T) {
	scriptContent := &ScriptPathContent{DynamicFrom: "test.sh"}
	require.Equal(t, llm.ContentTypeDynamic, scriptContent.Type())
}
