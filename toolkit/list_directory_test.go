package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestDirectoryListTool(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "directory_list_test")
	assert.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Create a test directory structure
	subDir1 := filepath.Join(tempDir, "subdir1")
	subDir2 := filepath.Join(tempDir, "subdir2")
	hiddenDir := filepath.Join(tempDir, ".hidden")

	assert.NoError(t, os.Mkdir(subDir1, 0755), "Failed to create subdir1")
	assert.NoError(t, os.Mkdir(subDir2, 0755), "Failed to create subdir2")
	assert.NoError(t, os.Mkdir(hiddenDir, 0755), "Failed to create hidden dir")

	// Create some test files
	testFile1 := filepath.Join(tempDir, "test1.txt")
	testFile2 := filepath.Join(subDir1, "test2.txt")
	testFile3 := filepath.Join(subDir2, "test3.log")
	hiddenFile := filepath.Join(tempDir, ".hidden_file")

	assert.NoError(t, os.WriteFile(testFile1, []byte("test content 1"), 0644), "Failed to create test file 1")
	assert.NoError(t, os.WriteFile(testFile2, []byte("test content 2"), 0644), "Failed to create test file 2")
	assert.NoError(t, os.WriteFile(testFile3, []byte("test content 3"), 0644), "Failed to create test file 3")
	assert.NoError(t, os.WriteFile(hiddenFile, []byte("hidden content"), 0644), "Failed to create hidden file")

	t.Run("ListDirectoryWithExplicitPath", func(t *testing.T) {
		tool := NewListDirectoryTool(ListDirectoryToolOptions{
			MaxEntries:   100,
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &ListDirectoryInput{
			Path: subDir1,
		})
		assert.NoError(t, err, "Unexpected error")

		// Check that the result contains only the expected entry
		assert.Len(t, result.Content, 1)
		output := result.Content[0].Text
		assert.Contains(t, output, "test2.txt")
		assert.NotContains(t, output, "test1.txt")
		assert.NotContains(t, output, "test3.log")
	})

	t.Run("ListNonExistentDirectory", func(t *testing.T) {
		tool := NewListDirectoryTool(ListDirectoryToolOptions{
			MaxEntries:   100,
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &ListDirectoryInput{
			Path: filepath.Join(tempDir, "nonexistent"),
		})
		assert.NoError(t, err, "Unexpected error")
		assert.Contains(t, result.Content[0].Text, "Directory not found")
	})

	t.Run("ListDirectoryWithMaxEntries", func(t *testing.T) {
		// Create many files to test MaxEntries
		manyFilesDir := filepath.Join(tempDir, "many_files")
		assert.NoError(t, os.Mkdir(manyFilesDir, 0755), "Failed to create many_files dir")

		for i := 0; i < 10; i++ {
			filename := filepath.Join(manyFilesDir, fmt.Sprintf("file%d.txt", i))
			assert.NoError(t, os.WriteFile(filename, []byte("content"), 0644), "Failed to create file")
		}

		tool := NewListDirectoryTool(ListDirectoryToolOptions{
			MaxEntries:   5,
			WorkspaceDir: tempDir,
		})

		result, err := tool.Call(context.Background(), &ListDirectoryInput{
			Path: manyFilesDir,
		})
		assert.NoError(t, err, "Unexpected error")

		// Check that the result mentions the limit
		assert.Len(t, result.Content, 1)
		output := result.Content[0].Text
		assert.Contains(t, output, "limited to 5 entries")

		// Count the number of entries in the JSON response
		var entries []DirectoryEntry
		jsonStart := strings.Index(output, "[")
		jsonEnd := strings.LastIndex(output, "]") + 1
		assert.NoError(t, json.Unmarshal([]byte(output[jsonStart:jsonEnd]), &entries))
		assert.Len(t, entries, 5, "Expected exactly 5 entries due to MaxEntries limit")
	})

	t.Run("ListDirectoryOutsideWorkspace", func(t *testing.T) {
		// Create another temp directory outside the workspace
		outsideDir, err := os.MkdirTemp("", "outside_workspace")
		assert.NoError(t, err)
		defer os.RemoveAll(outsideDir)

		tool := NewListDirectoryTool(ListDirectoryToolOptions{
			MaxEntries:   100,
			WorkspaceDir: tempDir,
		})
		result, err := tool.Call(context.Background(), &ListDirectoryInput{
			Path: outsideDir,
		})
		assert.NoError(t, err, "Unexpected error")
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "outside workspace")
	})

	t.Run("ToolDefinition", func(t *testing.T) {
		tool := NewListDirectoryTool(ListDirectoryToolOptions{
			DefaultPath: "/default/path",
		})
		assert.Equal(t, "ListDirectory", tool.Name())
		assert.Equal(t, "path", tool.Schema().Required[0])
	})
}

func TestDirectoryEntryFields(t *testing.T) {
	// Create a temporary directory and file
	tempDir, err := os.MkdirTemp("", "directory_entry_test")
	assert.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.txt")
	assert.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644), "Failed to create test file")

	// Create a tool and list the directory
	tool := NewListDirectoryTool(ListDirectoryToolOptions{
		MaxEntries:   100,
		WorkspaceDir: tempDir,
	})

	result, err := tool.Call(context.Background(), &ListDirectoryInput{
		Path: tempDir,
	})
	assert.NoError(t, err, "Unexpected error")
	assert.Len(t, result.Content, 1)
	output := result.Content[0].Text

	// Parse the JSON response
	var entries []DirectoryEntry
	jsonStart := strings.Index(output, "[")
	jsonEnd := strings.LastIndex(output, "]") + 1
	assert.NoError(t, json.Unmarshal([]byte(output[jsonStart:jsonEnd]), &entries))

	// Verify that we have one entry for the test file
	assert.Len(t, entries, 1, "Expected one entry")
	entry := entries[0]

	// Check all fields
	assert.Equal(t, "test.txt", entry.Name)
	assert.Equal(t, filepath.Join(tempDir, "test.txt"), entry.Path)
	assert.Equal(t, int64(12), entry.Size) // "test content" is 12 bytes
	assert.False(t, entry.IsDir)
	assert.Contains(t, entry.Mode, "rw")
	// Check that ModTime is within 5 seconds of now
	timeDiff := time.Since(entry.ModTime)
	assert.True(t, timeDiff >= 0 && timeDiff <= 5*time.Second, "ModTime should be within 5 seconds of now")
	assert.Equal(t, ".txt", entry.Extension)
}

func TestListDirectoryTool_NoPathProvided(t *testing.T) {
	tool := NewListDirectoryTool(ListDirectoryToolOptions{
		MaxEntries: 100,
	})

	result, err := tool.Call(context.Background(), &ListDirectoryInput{
		Path: "",
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "No directory path provided")
}

func TestListDirectoryTool_DefaultPath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "list_default_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test file
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("content"), 0644))

	tool := NewListDirectoryTool(ListDirectoryToolOptions{
		DefaultPath:  tempDir,
		MaxEntries:   100,
		WorkspaceDir: tempDir,
	})

	// Call with empty path - should use default
	result, err := tool.Call(context.Background(), &ListDirectoryInput{
		Path: "",
	})

	// Should succeed and list the default directory
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "test.txt")
}

func TestListDirectoryTool_FileAsPath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "list_file_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.txt")
	assert.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	tool := NewListDirectoryTool(ListDirectoryToolOptions{
		MaxEntries:   100,
		WorkspaceDir: tempDir,
	})

	result, err := tool.Call(context.Background(), &ListDirectoryInput{
		Path: testFile,
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "not a directory")
}

func TestListDirectoryTool_NoValidator(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "list_novalidator_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("content"), 0644))

	// Create tool without workspace dir
	tool := NewListDirectoryTool(ListDirectoryToolOptions{
		MaxEntries: 100,
	})

	result, err := tool.Call(context.Background(), &ListDirectoryInput{
		Path: tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "test.txt")
}

func TestListDirectoryTool_PreviewCall(t *testing.T) {
	tool := NewListDirectoryTool(ListDirectoryToolOptions{
		MaxEntries: 100,
	})
	ctx := context.Background()

	input := &ListDirectoryInput{
		Path: "/some/path",
	}

	preview := tool.PreviewCall(ctx, input)
	assert.Contains(t, preview.Summary, "/some/path")
}

func TestListDirectoryTool_Annotations(t *testing.T) {
	tool := NewListDirectoryTool(ListDirectoryToolOptions{
		MaxEntries: 100,
	})
	annotations := tool.Annotations()

	assert.Equal(t, "ListDirectory", annotations.Title)
	assert.True(t, annotations.ReadOnlyHint)
	assert.False(t, annotations.DestructiveHint)
	assert.True(t, annotations.IdempotentHint)
}

func TestListDirectoryTool_PermissionDenied(t *testing.T) {
	// Skip on Windows where permission handling is different
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tempDir, err := os.MkdirTemp("", "list_perm_test")
	assert.NoError(t, err)
	defer func() {
		os.Chmod(tempDir, 0755) // Restore permissions for cleanup
		os.RemoveAll(tempDir)
	}()

	// Create a directory with no read permissions
	restrictedDir := filepath.Join(tempDir, "restricted")
	assert.NoError(t, os.Mkdir(restrictedDir, 0000))

	tool := NewListDirectoryTool(ListDirectoryToolOptions{
		MaxEntries:   100,
		WorkspaceDir: tempDir,
	})

	result, err := tool.Call(context.Background(), &ListDirectoryInput{
		Path: restrictedDir,
	})

	assert.NoError(t, err)
	// Should return an error about permission
	assert.True(t, result.IsError)
	assert.Contains(t, strings.ToLower(result.Content[0].Text), "permission denied")
}

func TestListDirectoryTool_DirectoryWithSubdirectories(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "list_subdir_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create subdirectories
	subDir := filepath.Join(tempDir, "subdir")
	assert.NoError(t, os.Mkdir(subDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("content"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested"), 0644))

	tool := NewListDirectoryTool(ListDirectoryToolOptions{
		MaxEntries:   100,
		WorkspaceDir: tempDir,
	})

	result, err := tool.Call(context.Background(), &ListDirectoryInput{
		Path: tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "file.txt")
	assert.Contains(t, output, "subdir")

	// Parse and check is_dir field
	var entries []DirectoryEntry
	jsonStart := strings.Index(output, "[")
	jsonEnd := strings.LastIndex(output, "]") + 1
	assert.NoError(t, json.Unmarshal([]byte(output[jsonStart:jsonEnd]), &entries))

	// Find the subdir entry
	for _, entry := range entries {
		if entry.Name == "subdir" {
			assert.True(t, entry.IsDir)
			assert.Equal(t, "", entry.Extension)
		}
	}
}
