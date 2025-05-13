package toolkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestFileReadTool(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "file_read_test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Create a test file
	testContent := "This is test content for FileReadTool"
	testFilePath := filepath.Join(tempDir, "test_read.txt")
	err = os.WriteFile(testFilePath, []byte(testContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a large test file
	largeContent := strings.Repeat("Large content line\n", 1000)
	largeFilePath := filepath.Join(tempDir, "large_test.txt")
	err = os.WriteFile(largeFilePath, []byte(largeContent), 0644)
	require.NoError(t, err, "Failed to create large test file")

	t.Run("ReadExistingFile", func(t *testing.T) {
		tool := NewFileReadTool(FileReadToolOptions{
			MaxSize: 10000,
		})
		input := FileReadInput{
			Path: testFilePath,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Unexpected error")
		require.Equal(t, testContent, result.Output, "Content mismatch")
	})

	t.Run("ReadNonExistentFile", func(t *testing.T) {
		tool := NewFileReadTool(FileReadToolOptions{MaxSize: 10000})
		input := FileReadInput{
			Path: filepath.Join(tempDir, "nonexistent.txt"),
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error: File not found", "Expected 'file not found' error")
	})

	t.Run("ReadLargeFileTruncated", func(t *testing.T) {
		maxSize := 100
		tool := NewFileReadTool(FileReadToolOptions{MaxSize: maxSize})
		input := FileReadInput{
			Path: largeFilePath,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error: File", "Expected error about file size")
		require.Contains(t, result.Output, "is too large", "Expected error about file size")
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		tool := NewFileReadTool(FileReadToolOptions{MaxSize: 10000})

		_, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: "{invalid json",
		})

		require.Error(t, err, "Expected error for invalid JSON")
	})

	t.Run("NoPathProvided", func(t *testing.T) {
		tool := NewFileReadTool(FileReadToolOptions{MaxSize: 10000})
		input := FileReadInput{
			Path: "",
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error: No file path provided", "Expected 'no file path provided' error")
	})

	t.Run("ToolDefinition", func(t *testing.T) {
		tool := NewFileReadTool(FileReadToolOptions{MaxSize: 10000})
		require.Equal(t, "file_read", tool.Name(), "Tool name mismatch")
	})

	t.Run("RootDirectoryBehavior", func(t *testing.T) {
		// Create a subdirectory for testing root directory behavior
		rootDir := filepath.Join(tempDir, "root")
		require.NoError(t, os.MkdirAll(rootDir, 0755), "Failed to create root directory")

		// Create a test file inside the root directory
		rootFileContent := "This is a file inside the root directory"
		rootFilePath := "test_in_root.txt"
		fullRootFilePath := filepath.Join(rootDir, rootFilePath)
		require.NoError(t, os.WriteFile(fullRootFilePath, []byte(rootFileContent), 0644), "Failed to create test file in root")

		// Test reading a file inside the root directory
		tool := NewFileReadTool(FileReadToolOptions{RootDirectory: rootDir, MaxSize: 10000})
		input := FileReadInput{
			Path: rootFilePath, // Only the relative path
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})
		require.NoError(t, err, "Unexpected error")
		require.Equal(t, rootFileContent, result.Output, "Content mismatch")

		// Test attempting to read a file outside the root directory
		input = FileReadInput{
			Path: "../test_read.txt", // Try to access parent directory
		}
		inputJSON, _ = json.Marshal(input)

		result, err = tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error:", "Expected error for path outside root")
		require.Contains(t, result.Output, "outside of root directory", "Expected error about path outside root")
	})
}

func TestFileWriteTool(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "file_write_test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Create subdirectories for testing
	allowedDir := filepath.Join(tempDir, "allowed")
	deniedDir := filepath.Join(tempDir, "denied")
	nestedDir := filepath.Join(allowedDir, "nested")

	for _, dir := range []string{allowedDir, deniedDir, nestedDir} {
		require.NoError(t, os.MkdirAll(dir, 0755), "Failed to create directory")
	}

	t.Run("WriteToNewFile", func(t *testing.T) {
		tool := NewFileWriteTool(FileWriteToolOptions{})
		testFilePath := filepath.Join(tempDir, "test_write.txt")
		testContent := "This is test content for FileWriteTool"

		input := FileWriteInput{
			Path:    testFilePath,
			Content: testContent,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Output, "Successfully wrote", "Expected success message")

		// Verify file was created with correct content
		content, err := os.ReadFile(testFilePath)
		require.NoError(t, err, "Failed to read written file")
		require.Equal(t, testContent, string(content), "Content mismatch")
	})

	t.Run("WriteToNonExistentDirectory", func(t *testing.T) {
		tool := NewFileWriteTool(FileWriteToolOptions{})
		testFilePath := filepath.Join(tempDir, "new_dir", "test_write.txt")
		testContent := "This should create the directory"

		input := FileWriteInput{
			Path:    testFilePath,
			Content: testContent,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Output, "Successfully wrote", "Expected success message")

		// Verify file was created with correct content
		content, err := os.ReadFile(testFilePath)
		require.NoError(t, err, "Failed to read written file")
		require.Equal(t, testContent, string(content), "Content mismatch")
	})

	t.Run("NoPathProvided", func(t *testing.T) {
		tool := NewFileWriteTool(FileWriteToolOptions{})
		input := FileWriteInput{
			Path:    "",
			Content: "Some content",
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error: No file path provided", "Expected 'no file path provided' error")
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		tool := NewFileWriteTool(FileWriteToolOptions{})

		_, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: "{invalid json",
		})

		require.Error(t, err, "Expected error for invalid JSON")
	})

	t.Run("AllowlistExactMatch", func(t *testing.T) {
		allowedPath := filepath.Join(allowedDir, "allowed.txt")
		deniedPath := filepath.Join(deniedDir, "denied.txt")

		tool := NewFileWriteTool(FileWriteToolOptions{
			AllowList: []string{allowedPath},
		})
		testContent := "This should be allowed"

		// Test allowed path
		input := FileWriteInput{
			Path:    allowedPath,
			Content: testContent,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Output, "Successfully wrote", "Expected success message")

		// Test denied path
		input = FileWriteInput{
			Path:    deniedPath,
			Content: testContent,
		}
		inputJSON, _ = json.Marshal(input)

		result, err = tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error: Access denied", "Expected access denied error")
	})

	t.Run("DenylistExactMatch", func(t *testing.T) {
		allowedPath := filepath.Join(allowedDir, "allowed.txt")
		deniedPath := filepath.Join(deniedDir, "denied.txt")

		tool := NewFileWriteTool(FileWriteToolOptions{
			DenyList: []string{deniedPath},
		})
		testContent := "This should be denied"

		// Test allowed path
		input := FileWriteInput{
			Path:    allowedPath,
			Content: testContent,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Output, "Successfully wrote", "Expected success message")

		// Test denied path
		input = FileWriteInput{
			Path:    deniedPath,
			Content: testContent,
		}
		inputJSON, _ = json.Marshal(input)

		result, err = tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error: Access denied", "Expected access denied error")
	})

	t.Run("AllowlistWildcard", func(t *testing.T) {
		// Test with * wildcard
		tool := NewFileWriteTool(FileWriteToolOptions{
			AllowList: []string{filepath.Join(allowedDir, "*.txt")},
		})
		testContent := "Testing wildcard"

		// Should be allowed
		allowedPath := filepath.Join(allowedDir, "wildcard.txt")
		input := FileWriteInput{
			Path:    allowedPath,
			Content: testContent,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Output, "Successfully wrote", "Expected success message")

		// Should be denied (wrong extension)
		deniedPath := filepath.Join(allowedDir, "wildcard.json")
		input = FileWriteInput{
			Path:    deniedPath,
			Content: testContent,
		}
		inputJSON, _ = json.Marshal(input)

		result, err = tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error: Access denied", "Expected access denied error")
	})

	t.Run("AllowlistDoubleWildcard", func(t *testing.T) {
		// Test with ** wildcard
		tool := NewFileWriteTool(FileWriteToolOptions{
			AllowList: []string{filepath.Join(allowedDir, "**")},
		})
		testContent := "Testing double wildcard"

		// Should be allowed (in allowed dir)
		allowedPath := filepath.Join(allowedDir, "double_wildcard.txt")
		input := FileWriteInput{
			Path:    allowedPath,
			Content: testContent,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Output, "Successfully wrote", "Expected success message")

		// Should be allowed (in nested dir)
		nestedPath := filepath.Join(nestedDir, "nested_wildcard.txt")
		input = FileWriteInput{
			Path:    nestedPath,
			Content: testContent,
		}
		inputJSON, _ = json.Marshal(input)

		result, err = tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Output, "Successfully wrote", "Expected success message")

		// Should be denied (outside allowed dir)
		deniedPath := filepath.Join(deniedDir, "outside_wildcard.txt")
		input = FileWriteInput{
			Path:    deniedPath,
			Content: testContent,
		}
		inputJSON, _ = json.Marshal(input)

		result, err = tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error: Access denied", "Expected access denied error")
	})

	t.Run("DenylistOverridesAllowlist", func(t *testing.T) {
		// Allow all files in allowed dir, but deny specific file
		specificPath := filepath.Join(allowedDir, "specific.txt")
		tool := NewFileWriteTool(FileWriteToolOptions{
			AllowList: []string{filepath.Join(allowedDir, "**")},
			DenyList:  []string{specificPath},
		})
		testContent := "Testing deny override"

		// Should be allowed (in allowed dir)
		allowedPath := filepath.Join(allowedDir, "not_denied.txt")
		input := FileWriteInput{
			Path:    allowedPath,
			Content: testContent,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Output, "Successfully wrote", "Expected success message")

		// Should be denied (specifically denied)
		input = FileWriteInput{
			Path:    specificPath,
			Content: testContent,
		}
		inputJSON, _ = json.Marshal(input)

		result, err = tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error: Access denied", "Expected access denied error")
	})

	t.Run("ToolDefinition", func(t *testing.T) {
		tool := NewFileWriteTool(FileWriteToolOptions{
			AllowList: []string{"/allowed/**"},
			DenyList:  []string{"/denied/**"},
		})
		require.Equal(t, "file_write", tool.Name(), "Tool name mismatch")
	})

	t.Run("RootDirectoryBehavior", func(t *testing.T) {
		// Create a subdirectory for testing root directory behavior
		rootDir := filepath.Join(tempDir, "write_root")
		require.NoError(t, os.MkdirAll(rootDir, 0755), "Failed to create root directory")

		// Test writing a file inside the root directory
		tool := NewFileWriteTool(FileWriteToolOptions{
			RootDirectory: rootDir,
		})

		// Write to a file using a relative path
		relativeFilePath := "test_in_root.txt"
		testContent := "This is a file inside the root directory"

		input := FileWriteInput{
			Path:    relativeFilePath,
			Content: testContent,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})
		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Output, "Successfully wrote", "Expected success message")

		// Verify the file was created in the root directory
		fullPath := filepath.Join(rootDir, relativeFilePath)
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err, "Failed to read written file")
		require.Equal(t, testContent, string(content), "Content mismatch")

		// Test attempting to write a file outside the root directory
		input = FileWriteInput{
			Path:    "../outside_root.txt", // Try to access parent directory
			Content: "This should be denied",
		}
		inputJSON, _ = json.Marshal(input)

		result, err = tool.Call(context.Background(), &llm.ToolCallInput{
			Input: string(inputJSON),
		})
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error:", "Expected error for path outside root")
		require.Contains(t, result.Output, "outside of root directory", "Expected error about path outside root")

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result, "Error:", "Expected error for path outside root")
		require.Contains(t, result, "outside of root directory", "Expected error about path outside root")

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Output, "Error:", "Expected error for path outside root")
		require.Contains(t, result.Output, "outside of root directory", "Expected error about path outside root")
	})
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pattern  string
		expected bool
	}{
		{"ExactMatch", "/path/to/file.txt", "/path/to/file.txt", true},
		{"ExactMismatch", "/path/to/file.txt", "/path/to/other.txt", false},
		{"SingleWildcard", "/path/to/file.txt", "/path/to/*.txt", true},
		{"SingleWildcardMismatch", "/path/to/file.txt", "/path/to/*.json", false},
		{"DoubleWildcardPrefix", "/path/to/subdir/file.txt", "/path/to/**", true},
		{"DoubleWildcardPrefixMismatch", "/other/path/file.txt", "/path/to/**", false},
		{"DoubleWildcardSuffix", "/path/to/subdir/file.txt", "**/file.txt", true},
		{"DoubleWildcardSuffixMismatch", "/path/to/subdir/other.txt", "**/file.txt", false},
		{"DoubleWildcardMiddle", "/path/to/subdir/file.txt", "/path/**/*.txt", true},
		{"DoubleWildcardMiddleMismatch", "/path/to/subdir/file.json", "/path/**/*.txt", false},
		{"DoubleDoubleWildcard", "/path/to/file.txt", "/path/**/to/some/**/file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := matchesPattern(tt.path, tt.pattern)
			require.NoError(t, err, "Unexpected error")
			require.Equal(t, tt.expected, result, "Pattern matching result mismatch")
		})
	}
}
