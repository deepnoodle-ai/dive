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

	t.Run("ListDirectoryWithExplicitPath", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{MaxEntries: 100})
		result, err := tool.Call(context.Background(), &DirectoryListInput{
			Path: subDir1,
		})
		require.NoError(t, err, "Unexpected error")

		// Check that the result contains only the expected entry
		require.Len(t, result.Content, 1)
		output := result.Content[0].Text
		require.Contains(t, output, "test2.txt")
		require.NotContains(t, output, "test1.txt")
		require.NotContains(t, output, "test3.log")
	})

	t.Run("ListNonExistentDirectory", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{MaxEntries: 100})
		result, err := tool.Call(context.Background(), &DirectoryListInput{
			Path: filepath.Join(tempDir, "nonexistent"),
		})
		require.NoError(t, err, "Unexpected error")
		require.Contains(t, result.Content[0].Text, "Error: Directory not found")
	})

	t.Run("ListDirectoryWithMaxEntries", func(t *testing.T) {
		// Create many files to test MaxEntries
		manyFilesDir := filepath.Join(tempDir, "many_files")
		require.NoError(t, os.Mkdir(manyFilesDir, 0755), "Failed to create many_files dir")

		for i := 0; i < 10; i++ {
			filename := filepath.Join(manyFilesDir, fmt.Sprintf("file%d.txt", i))
			require.NoError(t, os.WriteFile(filename, []byte("content"), 0644), "Failed to create file")
		}

		tool := NewDirectoryListTool(DirectoryListToolOptions{MaxEntries: 5})

		result, err := tool.Call(context.Background(), &DirectoryListInput{
			Path: manyFilesDir,
		})
		require.NoError(t, err, "Unexpected error")

		// Check that the result mentions the limit
		require.Len(t, result.Content, 1)
		output := result.Content[0].Text
		require.Contains(t, output, "limited to 5 entries")

		// Count the number of entries in the JSON response
		var entries []DirectoryEntry
		jsonStart := strings.Index(output, "[")
		jsonEnd := strings.LastIndex(output, "]") + 1
		require.NoError(t, json.Unmarshal([]byte(output[jsonStart:jsonEnd]), &entries))
		require.Len(t, entries, 5, "Expected exactly 5 entries due to MaxEntries limit")
	})

	t.Run("ListDirectoryWithRootDirectory", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{
			RootDirectory: tempDir,
			MaxEntries:    100,
		})
		result, err := tool.Call(context.Background(), &DirectoryListInput{
			Path: "subdir1",
		})
		require.NoError(t, err, "Unexpected error")

		// Check that the result contains the expected entry
		require.Len(t, result.Content, 1)
		output := result.Content[0].Text
		require.Contains(t, output, "test2.txt")

		// Check that the path is relative to the root directory
		var entries []DirectoryEntry
		jsonStart := strings.Index(output, "[")
		jsonEnd := strings.LastIndex(output, "]") + 1
		require.NoError(t, json.Unmarshal([]byte(output[jsonStart:jsonEnd]), &entries))

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
		result1, err := tool.Call(context.Background(), &DirectoryListInput{
			Path: subDir1,
		})
		require.NoError(t, err, "Unexpected error")
		require.Len(t, result1.Content, 1)
		output := result1.Content[0].Text
		require.Contains(t, output, "test2.txt")

		// This should be denied
		result2, err := tool.Call(context.Background(), &DirectoryListInput{
			Path: subDir2,
		})
		require.NoError(t, err, "Unexpected error")
		require.Len(t, result2.Content, 1)
		output = result2.Content[0].Text
		require.Contains(t, output, "Access denied")
	})

	t.Run("ListDirectoryWithDenyList", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{
			MaxEntries: 100,
			DenyList:   []string{filepath.Join(tempDir, "**/.hidden*")},
		})
		// This should be allowed
		result1, err := tool.Call(context.Background(), &DirectoryListInput{
			Path: subDir1,
		})
		require.NoError(t, err, "Unexpected error")
		require.Len(t, result1.Content, 1)
		output := result1.Content[0].Text
		require.Contains(t, output, "test2.txt")

		// This should be denied
		result2, err := tool.Call(context.Background(), &DirectoryListInput{
			Path: hiddenDir,
		})
		require.NoError(t, err, "Unexpected error")
		require.Len(t, result2.Content, 1)
		output = result2.Content[0].Text
		require.Contains(t, output, "Access denied")
	})

	t.Run("ToolDefinition", func(t *testing.T) {
		tool := NewDirectoryListTool(DirectoryListToolOptions{
			DefaultPath:   "/default/path",
			RootDirectory: "/root/dir",
			AllowList:     []string{"/allowed/*"},
			DenyList:      []string{"/denied/*"},
		})
		require.Equal(t, "directory_list", tool.Name())
		require.Equal(t, "path", tool.Schema().Required[0])
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

	result, err := tool.Call(context.Background(), &DirectoryListInput{
		Path: tempDir,
	})
	require.NoError(t, err, "Unexpected error")
	require.Len(t, result.Content, 1)
	output := result.Content[0].Text

	// Parse the JSON response
	var entries []DirectoryEntry
	jsonStart := strings.Index(output, "[")
	jsonEnd := strings.LastIndex(output, "]") + 1
	require.NoError(t, json.Unmarshal([]byte(output[jsonStart:jsonEnd]), &entries))

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
