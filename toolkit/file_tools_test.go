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
			MaxSize:      10000,
			WorkspaceDir: tempDir,
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
		tool := NewReadFileTool(ReadFileToolOptions{
			MaxSize:      10000,
			WorkspaceDir: tempDir,
		})
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
		tool := NewReadFileTool(ReadFileToolOptions{
			MaxSize:      maxSize,
			WorkspaceDir: tempDir,
		})

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
		tool := NewReadFileTool(ReadFileToolOptions{
			MaxSize:      10000,
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &ReadFileInput{})
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.True(t, result.IsError)
		require.Len(t, result.Content, 1)
		output := result.Content[0].Text
		require.Contains(t, output, "Error: No file path provided", "Expected 'no file path provided' error")
	})

	t.Run("ReadOutsideWorkspace", func(t *testing.T) {
		// Create another temp directory outside the workspace
		outsideDir, err := os.MkdirTemp("", "outside_workspace")
		require.NoError(t, err)
		defer os.RemoveAll(outsideDir)

		outsideFile := filepath.Join(outsideDir, "outside.txt")
		err = os.WriteFile(outsideFile, []byte("outside content"), 0644)
		require.NoError(t, err)

		tool := NewReadFileTool(ReadFileToolOptions{
			MaxSize:      10000,
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &ReadFileInput{
			FilePath: outsideFile,
		})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "outside workspace")
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

	t.Run("WriteToNewFile", func(t *testing.T) {
		tool := NewWriteFileTool(WriteFileToolOptions{
			WorkspaceDir: tempDir,
		})
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
		tool := NewWriteFileTool(WriteFileToolOptions{
			WorkspaceDir: tempDir,
		})
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
		tool := NewWriteFileTool(WriteFileToolOptions{
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: "",
			Content:  "Some content",
		})

		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result.Content[0].Text, "Error: No file path provided", "Expected 'no file path provided' error")
	})

	t.Run("WriteOutsideWorkspace", func(t *testing.T) {
		// Create another temp directory outside the workspace
		outsideDir, err := os.MkdirTemp("", "outside_workspace")
		require.NoError(t, err)
		defer os.RemoveAll(outsideDir)

		outsideFile := filepath.Join(outsideDir, "outside.txt")

		tool := NewWriteFileTool(WriteFileToolOptions{
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: outsideFile,
			Content:  "should not be written",
		})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "outside workspace")

		// Verify file was not created
		_, err = os.Stat(outsideFile)
		require.True(t, os.IsNotExist(err), "File should not have been created")
	})

	t.Run("ToolDefinition", func(t *testing.T) {
		tool := NewWriteFileTool(WriteFileToolOptions{})
		require.Equal(t, "write_file", tool.Name(), "Tool name mismatch")
	})
}

func TestPathValidator(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "path_validator_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a nested directory structure
	nestedDir := filepath.Join(tempDir, "nested", "deep")
	require.NoError(t, os.MkdirAll(nestedDir, 0755))

	// Create a file in the nested directory
	testFile := filepath.Join(nestedDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	t.Run("PathInWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		require.NoError(t, err)

		inWorkspace, err := validator.IsInWorkspace(testFile)
		require.NoError(t, err)
		require.True(t, inWorkspace)
	})

	t.Run("PathOutsideWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		require.NoError(t, err)

		inWorkspace, err := validator.IsInWorkspace("/etc/passwd")
		require.NoError(t, err)
		require.False(t, inWorkspace)
	})

	t.Run("TraversalAttempt", func(t *testing.T) {
		validator, err := NewPathValidator(nestedDir)
		require.NoError(t, err)

		// Try to access parent directory
		parentPath := filepath.Join(nestedDir, "..", "..", "outside.txt")
		inWorkspace, err := validator.IsInWorkspace(parentPath)
		require.NoError(t, err)
		require.False(t, inWorkspace)
	})

	t.Run("ValidateReadInWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		require.NoError(t, err)

		err = validator.ValidateRead(testFile)
		require.NoError(t, err)
	})

	t.Run("ValidateReadOutsideWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		require.NoError(t, err)

		err = validator.ValidateRead("/etc/passwd")
		require.Error(t, err)
		require.True(t, IsPathAccessError(err))
	})

	t.Run("ValidateWriteInWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		require.NoError(t, err)

		newFile := filepath.Join(tempDir, "new.txt")
		err = validator.ValidateWrite(newFile)
		require.NoError(t, err)
	})

	t.Run("ValidateWriteOutsideWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		require.NoError(t, err)

		err = validator.ValidateWrite("/tmp/outside.txt")
		require.Error(t, err)
		require.True(t, IsPathAccessError(err))
	})
}
