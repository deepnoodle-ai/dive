package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadFileTool(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "read_file_test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Create a test file
	testContent := "This is test content for the read_file tool"
	testFilePath := filepath.Join(tempDir, "test_read.txt")
	err = os.WriteFile(testFilePath, []byte(testContent), 0644)
	require.NoError(t, err, "Failed to create test file")

	// Create a large test file
	largeContent := strings.Repeat("Large content line\n", 1000)
	largeFilePath := filepath.Join(tempDir, "large_test.txt")
	err = os.WriteFile(largeFilePath, []byte(largeContent), 0644)
	require.NoError(t, err, "Failed to create large test file")

	t.Run("ReadExistingFile", func(t *testing.T) {
		tool := NewReadFileTool(ReadFileToolOptions{
			MaxSize: 10000,
		})
		result, err := tool.Call(context.Background(), &ReadFileInput{
			FilePath: testFilePath,
		})
		require.NoError(t, err, "Unexpected error")
		require.Len(t, result.Content, 1)
		output := result.Content[0].Text
		require.Equal(t, testContent, output, "Content mismatch")
	})

	t.Run("ReadNonExistentFile", func(t *testing.T) {
		tool := NewReadFileTool(ReadFileToolOptions{MaxSize: 10000})
		result, err := tool.Call(context.Background(), &ReadFileInput{
			FilePath: filepath.Join(tempDir, "nonexistent.txt"),
		})
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Len(t, result.Content, 1)
		output := result.Content[0].Text
		require.Contains(t, output, "Error: File not found", "Expected 'file not found' error")
	})

	t.Run("ReadLargeFileTruncated", func(t *testing.T) {
		maxSize := 100
		tool := NewReadFileTool(ReadFileToolOptions{MaxSize: maxSize})

		result, err := tool.Call(context.Background(), &ReadFileInput{
			FilePath: largeFilePath,
		})
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Len(t, result.Content, 1)
		output := result.Content[0].Text
		require.Contains(t, output, "Error: File", "Expected error about file size")
		require.Contains(t, output, "is too large", "Expected error about file size")
	})

	t.Run("NoPathProvided", func(t *testing.T) {
		tool := NewReadFileTool(ReadFileToolOptions{MaxSize: 10000})
		result, err := tool.Call(context.Background(), &ReadFileInput{})
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.True(t, result.IsError)
		require.Len(t, result.Content, 1)
		output := result.Content[0].Text
		require.Contains(t, output, "Error: No file path provided", "Expected 'no file path provided' error")
	})

	t.Run("ToolDefinition", func(t *testing.T) {
		tool := NewReadFileTool(ReadFileToolOptions{MaxSize: 10000})
		require.Equal(t, "read_file", tool.Name(), "Tool name mismatch")
	})
}

func TestWriteFileTool(t *testing.T) {
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
		tool := NewWriteFileTool(WriteFileToolOptions{})
		testFilePath := filepath.Join(tempDir, "test_write.txt")
		testContent := "This is test content for write_file tool"

		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: testFilePath,
			Content:  testContent,
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Content[0].Text, "Successfully wrote", "Expected success message")

		// Verify file was created with correct content
		content, err := os.ReadFile(testFilePath)
		require.NoError(t, err, "Failed to read written file")
		require.Equal(t, testContent, string(content), "Content mismatch")
	})

	t.Run("WriteToNonExistentDirectory", func(t *testing.T) {
		tool := NewWriteFileTool(WriteFileToolOptions{})
		testFilePath := filepath.Join(tempDir, "new_dir", "test_write.txt")
		testContent := "This should create the directory"

		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: testFilePath,
			Content:  testContent,
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Content[0].Text, "Successfully wrote", "Expected success message")

		// Verify file was created with correct content
		content, err := os.ReadFile(testFilePath)
		require.NoError(t, err, "Failed to read written file")
		require.Equal(t, testContent, string(content), "Content mismatch")
	})

	t.Run("NoPathProvided", func(t *testing.T) {
		tool := NewWriteFileTool(WriteFileToolOptions{})
		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: "",
			Content:  "Some content",
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Content[0].Text, "Error: No file path provided", "Expected 'no file path provided' error")
	})

	t.Run("AllowlistExactMatch", func(t *testing.T) {
		allowedPath := filepath.Join(allowedDir, "allowed.txt")
		deniedPath := filepath.Join(deniedDir, "denied.txt")

		tool := NewWriteFileTool(WriteFileToolOptions{
			AllowList: []string{allowedPath},
		})
		testContent := "This should be allowed"

		// Test allowed path
		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: allowedPath,
			Content:  testContent,
		})
		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Content[0].Text, "Successfully wrote", "Expected success message")

		// Test denied path
		result, err = tool.Call(context.Background(), &WriteFileInput{
			FilePath: deniedPath,
			Content:  testContent,
		})
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Content[0].Text, "Error: Access denied", "Expected access denied error")
	})

	t.Run("DenylistExactMatch", func(t *testing.T) {
		allowedPath := filepath.Join(allowedDir, "allowed.txt")
		deniedPath := filepath.Join(deniedDir, "denied.txt")

		tool := NewWriteFileTool(WriteFileToolOptions{
			DenyList: []string{deniedPath},
		})
		testContent := "This should be denied"

		// Test allowed path
		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: allowedPath,
			Content:  testContent,
		})
		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Content[0].Text, "Successfully wrote", "Expected success message")

		// Test denied path
		result, err = tool.Call(context.Background(), &WriteFileInput{
			FilePath: deniedPath,
			Content:  testContent,
		})
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Content[0].Text, "Error: Access denied", "Expected access denied error")
	})

	t.Run("AllowlistWildcard", func(t *testing.T) {
		// Test with * wildcard
		tool := NewWriteFileTool(WriteFileToolOptions{
			AllowList: []string{filepath.Join(allowedDir, "*.txt")},
		})
		testContent := "Testing wildcard"

		// Should be allowed
		allowedPath := filepath.Join(allowedDir, "wildcard.txt")

		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: allowedPath,
			Content:  testContent,
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Content[0].Text, "Successfully wrote", "Expected success message")

		// Should be denied (wrong extension)
		deniedPath := filepath.Join(allowedDir, "wildcard.json")
		result, err = tool.Call(context.Background(), &WriteFileInput{
			FilePath: deniedPath,
			Content:  testContent,
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Content[0].Text, "Error: Access denied", "Expected access denied error")
	})

	t.Run("AllowlistDoubleWildcard", func(t *testing.T) {
		// Test with ** wildcard
		tool := NewWriteFileTool(WriteFileToolOptions{
			AllowList: []string{filepath.Join(allowedDir, "**")},
		})
		testContent := "Testing double wildcard"

		// Should be allowed (in allowed dir)
		allowedPath := filepath.Join(allowedDir, "double_wildcard.txt")
		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: allowedPath,
			Content:  testContent,
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Content[0].Text, "Successfully wrote", "Expected success message")

		// Should be allowed (in nested dir)
		nestedPath := filepath.Join(nestedDir, "nested_wildcard.txt")
		result, err = tool.Call(context.Background(), &WriteFileInput{
			FilePath: nestedPath,
			Content:  testContent,
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Content[0].Text, "Successfully wrote", "Expected success message")

		// Should be denied (outside allowed dir)
		deniedPath := filepath.Join(deniedDir, "outside_wildcard.txt")
		result, err = tool.Call(context.Background(), &WriteFileInput{
			FilePath: deniedPath,
			Content:  testContent,
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Content[0].Text, "Error: Access denied", "Expected access denied error")
	})

	t.Run("DenylistOverridesAllowlist", func(t *testing.T) {
		// Allow all files in allowed dir, but deny specific file
		specificPath := filepath.Join(allowedDir, "specific.txt")
		tool := NewWriteFileTool(WriteFileToolOptions{
			AllowList: []string{filepath.Join(allowedDir, "**")},
			DenyList:  []string{specificPath},
		})
		testContent := "Testing deny override"

		// Should be allowed (in allowed dir)
		allowedPath := filepath.Join(allowedDir, "not_denied.txt")
		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: allowedPath,
			Content:  testContent,
		})

		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Content[0].Text, "Successfully wrote", "Expected success message")

		// Should be denied (specifically denied)
		result, err = tool.Call(context.Background(), &WriteFileInput{
			FilePath: specificPath,
			Content:  testContent,
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Content[0].Text, "Error: Access denied", "Expected access denied error")
	})

	t.Run("ToolDefinition", func(t *testing.T) {
		tool := NewWriteFileTool(WriteFileToolOptions{
			AllowList: []string{"/allowed/**"},
			DenyList:  []string{"/denied/**"},
		})
		require.Equal(t, "write_file", tool.Name(), "Tool name mismatch")
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
