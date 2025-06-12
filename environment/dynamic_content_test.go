package environment

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diveagents/dive/llm"
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

	// Create a test Risor script
	risorScript := filepath.Join(tempDir, "test.risor")
	err := os.WriteFile(risorScript, []byte(`"Hello from Risor file"`), 0644)
	require.NoError(t, err)

	// Create a test shell script that outputs JSON
	shellScript := filepath.Join(tempDir, "test.sh")
	err = os.WriteFile(shellScript, []byte(`#!/bin/bash
echo '"Hello from shell script"'`), 0755)
	require.NoError(t, err)

	tests := []struct {
		name        string
		scriptPath  string
		expected    int
		expectError bool
	}{
		{
			name:       "risor script",
			scriptPath: risorScript,
			expected:   1,
		},
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

func TestFilterSerializableGlobals(t *testing.T) {
	globals := map[string]any{
		"string":   "value",
		"number":   42,
		"boolean":  true,
		"array":    []string{"a", "b"},
		"map":      map[string]string{"key": "value"},
		"function": func() {},      // This should be filtered out
		"channel":  make(chan int), // This should be filtered out
	}

	filtered := filterSerializableGlobals(globals)

	// Should contain serializable types
	require.Contains(t, filtered, "string")
	require.Contains(t, filtered, "number")
	require.Contains(t, filtered, "boolean")
	require.Contains(t, filtered, "array")
	require.Contains(t, filtered, "map")

	// Should not contain non-serializable types
	require.NotContains(t, filtered, "function")
	require.NotContains(t, filtered, "channel")
}

func TestIsJSONSerializable(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		serial bool
	}{
		{"string", "hello", true},
		{"int", 42, true},
		{"bool", true, true},
		{"slice", []int{1, 2, 3}, true},
		{"map", map[string]int{"a": 1}, true},
		{"function", func() {}, false},
		{"channel", make(chan int), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isJSONSerializable(tt.value)
			require.Equal(t, tt.serial, result)
		})
	}
}
