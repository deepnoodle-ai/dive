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

	"github.com/stretchr/testify/require"
)

func TestDirectoryListTool(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "directory_list_test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Create a test directory structure
	subDir1 := filepath.Join(tempDir, "subdir1")
	subDir2 := filepath.Join(tempDir, "subdir2")
	hiddenDir := filepath.Join(tempDir, ".hidden")

	require.NoError(t, os.Mkdir(subDir1, 0755), "Failed to create subdir1")
	require.NoError(t, os.Mkdir(subDir2, 0755), "Failed to create subdir2")
	require.NoError(t, os.Mkdir(hiddenDir, 0755), "Failed to create hidden dir")

	// Create some test files
	testFile1 := filepath.Join(tempDir, "test1.txt")
	testFile2 := filepath.Join(subDir1, "test2.txt")
	testFile3 := filepath.Join(subDir2, "test3.log")
	hiddenFile := filepath.Join(tempDir, ".hidden_file")

	require.NoError(t, os.WriteFile(testFile1, []byte("test content 1"), 0644), "Failed to create test file 1")
	require.NoError(t, os.WriteFile(testFile2, []byte("test content 2"), 0644), "Failed to create test file 2")
	require.NoError(t, os.WriteFile(testFile3, []byte("test content 3"), 0644), "Failed to create test file 3")
	require.NoError(t, os.WriteFile(hiddenFile, []byte("hidden content"), 0644), "Failed to create hidden file")

	t.Run("ListDirectoryWithDefaultPath", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{
			DefaultPath: tempDir,
			MaxEntries:  100,
		})

		input := DirectoryListInput{
			Path: "",
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), string(inputJSON))
		require.NoError(t, err, "Unexpected error")

		// Check that the result contains all expected entries
		require.Contains(t, result, "subdir1")
		require.Contains(t, result, "subdir2")
		require.Contains(t, result, ".hidden")
		require.Contains(t, result, "test1.txt")
		require.Contains(t, result, ".hidden_file")
	})

	t.Run("ListDirectoryWithExplicitPath", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{
			MaxEntries: 100,
		})

		input := DirectoryListInput{
			Path: subDir1,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), string(inputJSON))
		require.NoError(t, err, "Unexpected error")

		// Check that the result contains only the expected entry
		require.Contains(t, result, "test2.txt")
		require.NotContains(t, result, "test1.txt")
		require.NotContains(t, result, "test3.log")
	})

	t.Run("ListNonExistentDirectory", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{
			MaxEntries: 100,
		})

		input := DirectoryListInput{
			Path: filepath.Join(tempDir, "nonexistent"),
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), string(inputJSON))
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result, "Error: Directory not found")
	})

	t.Run("ListDirectoryWithMaxEntries", func(t *testing.T) {
		// Create many files to test MaxEntries
		manyFilesDir := filepath.Join(tempDir, "many_files")
		require.NoError(t, os.Mkdir(manyFilesDir, 0755), "Failed to create many_files dir")

		for i := 0; i < 10; i++ {
			filename := filepath.Join(manyFilesDir, fmt.Sprintf("file%d.txt", i))
			require.NoError(t, os.WriteFile(filename, []byte("content"), 0644), "Failed to create file")
		}

		tool := NewDirectoryListTool(DirectoryListToolOptions{
			MaxEntries: 5,
		})

		input := DirectoryListInput{
			Path: manyFilesDir,
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), string(inputJSON))
		require.NoError(t, err, "Unexpected error")

		// Check that the result mentions the limit
		require.Contains(t, result, "limited to 5 entries")

		// Count the number of entries in the JSON response
		var entries []DirectoryEntry
		jsonStart := strings.Index(result, "[")
		jsonEnd := strings.LastIndex(result, "]") + 1
		require.NoError(t, json.Unmarshal([]byte(result[jsonStart:jsonEnd]), &entries))
		require.Len(t, entries, 5, "Expected exactly 5 entries due to MaxEntries limit")
	})

	t.Run("ListDirectoryWithRootDirectory", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{
			RootDirectory: tempDir,
			MaxEntries:    100,
		})

		input := DirectoryListInput{
			Path: "subdir1",
		}
		inputJSON, _ := json.Marshal(input)

		result, err := tool.Call(context.Background(), string(inputJSON))
		require.NoError(t, err, "Unexpected error")

		// Check that the result contains the expected entry
		require.Contains(t, result, "test2.txt")

		// Check that the path is relative to the root directory
		var entries []DirectoryEntry
		jsonStart := strings.Index(result, "[")
		jsonEnd := strings.LastIndex(result, "]") + 1
		require.NoError(t, json.Unmarshal([]byte(result[jsonStart:jsonEnd]), &entries))

		for _, entry := range entries {
			require.Equal(t, "subdir1/test2.txt", entry.Path)
		}
	})

	t.Run("ListDirectoryWithAllowList", func(t *testing.T) {
		// Get absolute path for the allow list pattern
		absSubDir1, err := filepath.Abs(subDir1)
		require.NoError(t, err, "Failed to get absolute path")

		tool := NewDirectoryListTool(DirectoryListToolOptions{
			MaxEntries: 100,
			AllowList:  []string{absSubDir1},
		})

		// This should be allowed
		input1 := DirectoryListInput{
			Path: subDir1,
		}
		inputJSON1, _ := json.Marshal(input1)

		result1, err := tool.Call(context.Background(), string(inputJSON1))
		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result1, "test2.txt")

		// This should be denied
		input2 := DirectoryListInput{
			Path: subDir2,
		}
		inputJSON2, _ := json.Marshal(input2)

		result2, err := tool.Call(context.Background(), string(inputJSON2))
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result2, "Access denied")
	})

	t.Run("ListDirectoryWithDenyList", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{
			MaxEntries: 100,
			DenyList:   []string{filepath.Join(tempDir, "**/.hidden*")},
		})

		// This should be allowed
		input1 := DirectoryListInput{
			Path: subDir1,
		}
		inputJSON1, _ := json.Marshal(input1)

		result1, err := tool.Call(context.Background(), string(inputJSON1))
		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result1, "test2.txt")

		// This should be denied
		input2 := DirectoryListInput{
			Path: hiddenDir,
		}
		inputJSON2, _ := json.Marshal(input2)

		result2, err := tool.Call(context.Background(), string(inputJSON2))
		require.NoError(t, err, "Expected error to be returned in result, not as an error")
		require.Contains(t, result2, "Access denied")
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{
			MaxEntries: 100,
		})

		_, err := tool.Call(context.Background(), "{invalid json")
		require.Error(t, err, "Expected error for invalid JSON")
	})

	t.Run("ToolDefinition", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{
			DefaultPath:   "/default/path",
			RootDirectory: "/root/dir",
			AllowList:     []string{"/allowed/*"},
			DenyList:      []string{"/denied/*"},
		})

		def := tool.Definition()
		require.Equal(t, "directory_list", def.Name)
		require.Contains(t, def.Description, "/default/path")
		require.Contains(t, def.Description, "/root/dir")
		require.Contains(t, def.Description, "restricted based on configured allowlist and denylist patterns")
		require.Equal(t, "path", def.Parameters.Required[0])
	})

	t.Run("ShouldReturnResult", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{})
		require.True(t, tool.ShouldReturnResult())
	})
}

func TestDirectoryEntryFields(t *testing.T) {
	// Create a temporary directory and file
	tempDir, err := os.MkdirTemp("", "directory_entry_test")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644), "Failed to create test file")

	// Create a tool and list the directory
	tool := NewDirectoryListTool(DirectoryListToolOptions{
		MaxEntries: 100,
	})

	input := DirectoryListInput{
		Path: tempDir,
	}
	inputJSON, _ := json.Marshal(input)

	result, err := tool.Call(context.Background(), string(inputJSON))
	require.NoError(t, err, "Unexpected error")

	// Parse the JSON response
	var entries []DirectoryEntry
	jsonStart := strings.Index(result, "[")
	jsonEnd := strings.LastIndex(result, "]") + 1
	require.NoError(t, json.Unmarshal([]byte(result[jsonStart:jsonEnd]), &entries))

	// Verify that we have one entry for the test file
	require.Len(t, entries, 1, "Expected one entry")
	entry := entries[0]

	// Check all fields
	require.Equal(t, "test.txt", entry.Name)
	require.Equal(t, filepath.Join(tempDir, "test.txt"), entry.Path)
	require.Equal(t, int64(12), entry.Size) // "test content" is 12 bytes
	require.False(t, entry.IsDir)
	require.Contains(t, entry.Mode, "rw")
	require.WithinDuration(t, time.Now(), entry.ModTime, 5*time.Second)
	require.Equal(t, ".txt", entry.Extension)
}
