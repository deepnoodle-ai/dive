package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestReadFileTool(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "read_file_test")
	assert.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Create a test file
	testContent := "This is test content for the read_file tool"
	testFilePath := filepath.Join(tempDir, "test_read.txt")
	err = os.WriteFile(testFilePath, []byte(testContent), 0644)
	assert.NoError(t, err, "Failed to create test file")

	// Create a large test file
	largeContent := strings.Repeat("Large content line\n", 1000)
	largeFilePath := filepath.Join(tempDir, "large_test.txt")
	err = os.WriteFile(largeFilePath, []byte(largeContent), 0644)
	assert.NoError(t, err, "Failed to create large test file")

	t.Run("ReadExistingFile", func(t *testing.T) {
		tool := NewReadFileTool(ReadFileToolOptions{
			MaxSize:      10000,
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &ReadFileInput{
			FilePath: testFilePath,
		})
		assert.NoError(t, err, "Unexpected error")
		assert.Len(t, result.Content, 1)
		output := result.Content[0].Text
		assert.Equal(t, testContent, output, "Content mismatch")
	})

	t.Run("ReadNonExistentFile", func(t *testing.T) {
		tool := NewReadFileTool(ReadFileToolOptions{
			MaxSize:      10000,
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &ReadFileInput{
			FilePath: filepath.Join(tempDir, "nonexistent.txt"),
		})
		assert.NoError(t, err, "Expected error to be returned in result, not as an error")
		assert.Len(t, result.Content, 1)
		output := result.Content[0].Text
		assert.Contains(t, output, "Error: File not found", "Expected 'file not found' error")
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
		assert.NoError(t, err, "Expected error to be returned in result, not as an error")
		assert.Len(t, result.Content, 1)
		output := result.Content[0].Text
		assert.Contains(t, output, "Error: File", "Expected error about file size")
		assert.Contains(t, output, "is too large", "Expected error about file size")
	})

	t.Run("NoPathProvided", func(t *testing.T) {
		tool := NewReadFileTool(ReadFileToolOptions{
			MaxSize:      10000,
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &ReadFileInput{})
		assert.NoError(t, err, "Expected error to be returned in result, not as an error")
		assert.True(t, result.IsError)
		assert.Len(t, result.Content, 1)
		output := result.Content[0].Text
		assert.Contains(t, output, "Error: No file path provided", "Expected 'no file path provided' error")
	})

	t.Run("ReadOutsideWorkspace", func(t *testing.T) {
		// Create another temp directory outside the workspace
		outsideDir, err := os.MkdirTemp("", "outside_workspace")
		assert.NoError(t, err)
		defer os.RemoveAll(outsideDir)

		outsideFile := filepath.Join(outsideDir, "outside.txt")
		err = os.WriteFile(outsideFile, []byte("outside content"), 0644)
		assert.NoError(t, err)

		tool := NewReadFileTool(ReadFileToolOptions{
			MaxSize:      10000,
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &ReadFileInput{
			FilePath: outsideFile,
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "outside workspace")
	})

	t.Run("ToolDefinition", func(t *testing.T) {
		tool := NewReadFileTool(ReadFileToolOptions{MaxSize: 10000})
		assert.Equal(t, "Read", tool.Name(), "Tool name mismatch")
	})
}

func TestWriteFileTool(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "file_write_test")
	assert.NoError(t, err, "Failed to create temp directory")
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

		assert.NoError(t, err, "Unexpected error")
		assert.Contains(t, result.Content[0].Text, "Successfully wrote", "Expected success message")

		// Verify file was created with correct content
		content, err := os.ReadFile(testFilePath)
		assert.NoError(t, err, "Failed to read written file")
		assert.Equal(t, testContent, string(content), "Content mismatch")
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

		assert.NoError(t, err, "Unexpected error")
		assert.Contains(t, result.Content[0].Text, "Successfully wrote", "Expected success message")

		// Verify file was created with correct content
		content, err := os.ReadFile(testFilePath)
		assert.NoError(t, err, "Failed to read written file")
		assert.Equal(t, testContent, string(content), "Content mismatch")
	})

	t.Run("NoPathProvided", func(t *testing.T) {
		tool := NewWriteFileTool(WriteFileToolOptions{
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: "",
			Content:  "Some content",
		})

		assert.NoError(t, err, "Expected error to be returned in result, not as an error")
		assert.Contains(t, result.Content[0].Text, "Error: No file path provided", "Expected 'no file path provided' error")
	})

	t.Run("WriteOutsideWorkspace", func(t *testing.T) {
		// Create another temp directory outside the workspace
		outsideDir, err := os.MkdirTemp("", "outside_workspace")
		assert.NoError(t, err)
		defer os.RemoveAll(outsideDir)

		outsideFile := filepath.Join(outsideDir, "outside.txt")

		tool := NewWriteFileTool(WriteFileToolOptions{
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &WriteFileInput{
			FilePath: outsideFile,
			Content:  "should not be written",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "outside workspace")

		// Verify file was not created
		_, err = os.Stat(outsideFile)
		assert.True(t, os.IsNotExist(err), "File should not have been created")
	})

	t.Run("ToolDefinition", func(t *testing.T) {
		tool := NewWriteFileTool(WriteFileToolOptions{})
		assert.Equal(t, "Write", tool.Name(), "Tool name mismatch")
	})
}

func TestRealFileSystem_ListDir(t *testing.T) {
	// Create a temporary directory structure for testing
	tempDir, err := os.MkdirTemp("", "listdir_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a directory structure:
	// tempDir/
	//   file1.txt
	//   .hidden_file
	//   subdir1/
	//     file2.txt
	//     .hidden_dir/
	//       file3.txt
	//   subdir2/
	//     subsubdir/
	//       deep_file.txt (depth 3 - should not be listed)

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("file1"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, ".hidden_file"), []byte("hidden"), 0644))

	subdir1 := filepath.Join(tempDir, "subdir1")
	assert.NoError(t, os.MkdirAll(subdir1, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(subdir1, "file2.txt"), []byte("file2"), 0644))

	hiddenDir := filepath.Join(subdir1, ".hidden_dir")
	assert.NoError(t, os.MkdirAll(hiddenDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(hiddenDir, "file3.txt"), []byte("file3"), 0644))

	subsubdir := filepath.Join(tempDir, "subdir2", "subsubdir")
	assert.NoError(t, os.MkdirAll(subsubdir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(subsubdir, "deep_file.txt"), []byte("deep"), 0644))

	fs := &RealFileSystem{}

	t.Run("ListsFilesWithinDepthLimit", func(t *testing.T) {
		output, err := fs.ListDir(tempDir)
		assert.NoError(t, err)

		// Should contain the root directory
		assert.Contains(t, output, tempDir)

		// Should contain files at depth 1
		assert.Contains(t, output, "file1.txt")

		// Should contain directories at depth 1
		assert.Contains(t, output, "subdir1")
		assert.Contains(t, output, "subdir2")

		// Should contain files at depth 2
		assert.Contains(t, output, "file2.txt")
	})

	t.Run("ExcludesHiddenFiles", func(t *testing.T) {
		output, err := fs.ListDir(tempDir)
		assert.NoError(t, err)

		// Should NOT contain hidden files
		assert.NotContains(t, output, ".hidden_file")

		// Should NOT contain hidden directories or their contents
		assert.NotContains(t, output, ".hidden_dir")
		assert.NotContains(t, output, "file3.txt")
	})

	t.Run("RespectsDepthLimit", func(t *testing.T) {
		output, err := fs.ListDir(tempDir)
		assert.NoError(t, err)

		// Should NOT contain files deeper than 2 levels
		assert.NotContains(t, output, "deep_file.txt")
	})

	t.Run("HandlesNonExistentDirectory", func(t *testing.T) {
		output, err := fs.ListDir("/nonexistent/path")
		// Should return an error or empty output
		if err == nil {
			assert.Empty(t, output)
		}
	})

	t.Run("HandlesDangerousPathCharacters", func(t *testing.T) {
		// Test that paths starting with - don't cause issues
		// (This was the security issue with the old find-based implementation)
		dangerousDir := filepath.Join(tempDir, "-rf")
		assert.NoError(t, os.MkdirAll(dangerousDir, 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(dangerousDir, "test.txt"), []byte("test"), 0644))

		// This should work without any shell injection issues
		output, err := fs.ListDir(tempDir)
		assert.NoError(t, err)
		assert.Contains(t, output, "-rf")
	})
}

func TestPathValidator(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "path_validator_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a nested directory structure
	nestedDir := filepath.Join(tempDir, "nested", "deep")
	assert.NoError(t, os.MkdirAll(nestedDir, 0755))

	// Create a file in the nested directory
	testFile := filepath.Join(nestedDir, "test.txt")
	assert.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	t.Run("PathInWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		assert.NoError(t, err)

		inWorkspace, err := validator.IsInWorkspace(testFile)
		assert.NoError(t, err)
		assert.True(t, inWorkspace)
	})

	t.Run("PathOutsideWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		assert.NoError(t, err)

		inWorkspace, err := validator.IsInWorkspace("/etc/passwd")
		assert.NoError(t, err)
		assert.False(t, inWorkspace)
	})

	t.Run("TraversalAttempt", func(t *testing.T) {
		validator, err := NewPathValidator(nestedDir)
		assert.NoError(t, err)

		// Try to access parent directory
		parentPath := filepath.Join(nestedDir, "..", "..", "outside.txt")
		inWorkspace, err := validator.IsInWorkspace(parentPath)
		assert.NoError(t, err)
		assert.False(t, inWorkspace)
	})

	t.Run("ValidateReadInWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		assert.NoError(t, err)

		err = validator.ValidateRead(testFile)
		assert.NoError(t, err)
	})

	t.Run("ValidateReadOutsideWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		assert.NoError(t, err)

		err = validator.ValidateRead("/etc/passwd")
		assert.Error(t, err)
		assert.True(t, IsPathAccessError(err))
	})

	t.Run("ValidateWriteInWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		assert.NoError(t, err)

		newFile := filepath.Join(tempDir, "new.txt")
		err = validator.ValidateWrite(newFile)
		assert.NoError(t, err)
	})

	t.Run("ValidateWriteOutsideWorkspace", func(t *testing.T) {
		validator, err := NewPathValidator(tempDir)
		assert.NoError(t, err)

		err = validator.ValidateWrite("/tmp/outside.txt")
		assert.Error(t, err)
		assert.True(t, IsPathAccessError(err))
	})
}

func TestReadFileTool_WithOffsetAndLimit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "read_offset_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a file with multiple lines
	content := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10"
	testFile := filepath.Join(tempDir, "multiline.txt")
	err = os.WriteFile(testFile, []byte(content), 0644)
	assert.NoError(t, err)

	tool := NewReadFileTool(ReadFileToolOptions{
		MaxSize:      10000,
		WorkspaceDir: tempDir,
	})

	t.Run("ReadWithOffset", func(t *testing.T) {
		result, err := tool.Call(context.Background(), &ReadFileInput{
			FilePath: testFile,
			Offset:   3,
			Limit:    3,
		})
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		output := result.Content[0].Text
		assert.Contains(t, output, "line 3")
		assert.Contains(t, output, "line 4")
		assert.Contains(t, output, "line 5")
		assert.NotContains(t, output, "line 1")
		assert.NotContains(t, output, "line 6")
	})

	t.Run("ReadFromBeginningWithLimit", func(t *testing.T) {
		result, err := tool.Call(context.Background(), &ReadFileInput{
			FilePath: testFile,
			Limit:    2,
		})
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		output := result.Content[0].Text
		assert.Contains(t, output, "line 1")
		assert.Contains(t, output, "line 2")
		assert.NotContains(t, output, "line 3")
	})
}

func TestReadFileTool_BinaryFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "read_binary_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a binary file with null bytes
	binaryContent := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	binaryFile := filepath.Join(tempDir, "binary.bin")
	err = os.WriteFile(binaryFile, binaryContent, 0644)
	assert.NoError(t, err)

	tool := NewReadFileTool(ReadFileToolOptions{
		MaxSize:      10000,
		WorkspaceDir: tempDir,
	})

	result, err := tool.Call(context.Background(), &ReadFileInput{
		FilePath: binaryFile,
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "binary file")
}

func TestReadFileTool_Directory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "read_dir_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	tool := NewReadFileTool(ReadFileToolOptions{
		MaxSize:      10000,
		WorkspaceDir: tempDir,
	})

	result, err := tool.Call(context.Background(), &ReadFileInput{
		FilePath: tempDir,
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "directory")
}

func TestReadFileTool_NoValidator(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "read_novalidator_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("content"), 0644)
	assert.NoError(t, err)

	// Create tool without workspace dir
	tool := NewReadFileTool(ReadFileToolOptions{MaxSize: 10000})

	result, err := tool.Call(context.Background(), &ReadFileInput{
		FilePath: testFile,
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Equal(t, "content", result.Content[0].Text)
}

func TestReadFileTool_PreviewCall(t *testing.T) {
	tool := NewReadFileTool(ReadFileToolOptions{MaxSize: 10000})
	ctx := context.Background()

	input := &ReadFileInput{
		FilePath: "/path/to/file.txt",
	}

	preview := tool.PreviewCall(ctx, input)
	assert.Contains(t, preview.Summary, "/path/to/file.txt")
}

func TestReadFileTool_Annotations(t *testing.T) {
	tool := NewReadFileTool(ReadFileToolOptions{MaxSize: 10000})
	annotations := tool.Annotations()

	assert.Equal(t, "Read", annotations.Title)
	assert.True(t, annotations.ReadOnlyHint)
	assert.False(t, annotations.DestructiveHint)
	assert.True(t, annotations.IdempotentHint)
}

func TestWriteFileTool_OverwriteExisting(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "write_overwrite_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "existing.txt")
	err = os.WriteFile(testFile, []byte("original content"), 0644)
	assert.NoError(t, err)

	tool := NewWriteFileTool(WriteFileToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &WriteFileInput{
		FilePath: testFile,
		Content:  "new content",
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	content, err := os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, "new content", string(content))
}

func TestWriteFileTool_NoValidator(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "write_novalidator_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.txt")

	// Create tool without workspace dir
	tool := NewWriteFileTool(WriteFileToolOptions{})

	result, err := tool.Call(context.Background(), &WriteFileInput{
		FilePath: testFile,
		Content:  "content",
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	content, err := os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestWriteFileTool_PreviewCall(t *testing.T) {
	tool := NewWriteFileTool(WriteFileToolOptions{})
	ctx := context.Background()

	input := &WriteFileInput{
		FilePath: "/path/to/file.txt",
		Content:  "some content here",
	}

	preview := tool.PreviewCall(ctx, input)
	assert.Contains(t, preview.Summary, "/path/to/file.txt")
	assert.Contains(t, preview.Details, "17 bytes")
}

func TestWriteFileTool_Annotations(t *testing.T) {
	tool := NewWriteFileTool(WriteFileToolOptions{})
	annotations := tool.Annotations()

	assert.Equal(t, "Write", annotations.Title)
	assert.False(t, annotations.ReadOnlyHint)
	assert.True(t, annotations.DestructiveHint)
	assert.True(t, annotations.IdempotentHint)
	assert.True(t, annotations.EditHint)
}

func TestWriteFileTool_EmptyContent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "write_empty_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "empty.txt")

	tool := NewWriteFileTool(WriteFileToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &WriteFileInput{
		FilePath: testFile,
		Content:  "",
	})
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	content, err := os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, "", string(content))
}
