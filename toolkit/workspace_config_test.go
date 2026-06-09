package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

// invalidWorkspaceDir returns a workspace path that causes NewPathValidator
// to fail: the NUL byte makes symlink resolution return an error that is not
// os.IsNotExist.
func invalidWorkspaceDir() string {
	return "/tmp/bad\x00workspace"
}

func TestReadFileTool_InvalidWorkspaceFailsClosed(t *testing.T) {
	// Create a real file that the tool must NOT read
	tempDir := t.TempDir()
	secretPath := filepath.Join(tempDir, "secret.txt")
	assert.NoError(t, os.WriteFile(secretPath, []byte("secret content"), 0644))

	tool := NewReadFileTool(ReadFileToolOptions{WorkspaceDir: invalidWorkspaceDir()})

	result, err := tool.Call(context.Background(), &ReadFileInput{FilePath: secretPath})
	assert.NoError(t, err)
	assert.True(t, result.IsError, "expected config error, got success")
	assert.Contains(t, result.Content[0].Text, "invalid workspace configuration")
	assert.NotContains(t, result.Content[0].Text, "secret content")
}

func TestWriteFileTool_InvalidWorkspaceFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "out.txt")

	tool := NewWriteFileTool(WriteFileToolOptions{WorkspaceDir: invalidWorkspaceDir()})

	result, err := tool.Call(context.Background(), &WriteFileInput{
		FilePath: targetPath,
		Content:  "data",
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError, "expected config error, got success")
	assert.Contains(t, result.Content[0].Text, "invalid workspace configuration")

	// Verify the file was not written
	_, statErr := os.Stat(targetPath)
	assert.True(t, os.IsNotExist(statErr), "expected file to not be written")
}

func TestEditTool_InvalidWorkspaceFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "file.txt")
	assert.NoError(t, os.WriteFile(targetPath, []byte("hello world"), 0644))

	tool := NewEditTool(EditToolOptions{WorkspaceDir: invalidWorkspaceDir()})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  targetPath,
		OldString: "hello",
		NewString: "goodbye",
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError, "expected config error, got success")
	assert.Contains(t, result.Content[0].Text, "invalid workspace configuration")

	// Verify the file was not modified
	content, readErr := os.ReadFile(targetPath)
	assert.NoError(t, readErr)
	assert.Equal(t, "hello world", string(content))
}

func TestGrepTool_InvalidWorkspaceFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("needle"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: invalidWorkspaceDir()})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "needle",
		Path:    tempDir,
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError, "expected config error, got success")
	assert.Contains(t, result.Content[0].Text, "invalid workspace configuration")
}

func TestListDirectoryTool_InvalidWorkspaceFailsClosed(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("x"), 0644))

	tool := NewListDirectoryTool(ListDirectoryToolOptions{WorkspaceDir: invalidWorkspaceDir()})

	result, err := tool.Call(context.Background(), &ListDirectoryInput{Path: tempDir})
	assert.NoError(t, err)
	assert.True(t, result.IsError, "expected config error, got success")
	assert.Contains(t, result.Content[0].Text, "invalid workspace configuration")
}

func TestTextEditorTool_InvalidWorkspaceFailsClosed(t *testing.T) {
	fs := newMockFileSystem()
	fs.AddFile("/test/file.txt", "secret content")

	tool := NewTextEditorTool(TextEditorToolOptions{
		FileSystem:   fs,
		WorkspaceDir: invalidWorkspaceDir(),
	})

	result, err := tool.Call(context.Background(), &TextEditorToolInput{
		Command: CommandView,
		Path:    "/test/file.txt",
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError, "expected config error, got success")
	assert.Contains(t, result.Content[0].Text, "invalid workspace configuration")
	assert.NotContains(t, result.Content[0].Text, "secret content")
}
