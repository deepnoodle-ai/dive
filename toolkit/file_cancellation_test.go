package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestCancelledFileToolsDoNotMutateFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	assert.NoError(t, os.WriteFile(path, []byte("before"), 0644))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	write := NewWriteFileTool(WriteFileToolOptions{WorkspaceDir: dir})
	result, err := write.Call(ctx, &WriteFileInput{FilePath: path, Content: "after"})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)

	edit := NewEditTool(EditToolOptions{WorkspaceDir: dir})
	result, err = edit.Call(ctx, &EditInput{FilePath: path, OldString: "before", NewString: "after"})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)

	editor := NewTextEditorTool(TextEditorToolOptions{WorkspaceDir: dir})
	newPath := filepath.Join(dir, "new.txt")
	content := "created"
	result, err = editor.Call(ctx, &TextEditorToolInput{Command: CommandCreate, Path: newPath, FileText: &content})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)

	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "before", string(data))
	_, err = os.Stat(newPath)
	assert.True(t, os.IsNotExist(err))
}

func TestCancelledReadAndListReturnContextError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	assert.NoError(t, os.WriteFile(path, []byte("content"), 0644))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	read := NewReadFileTool(ReadFileToolOptions{WorkspaceDir: dir})
	result, err := read.Call(ctx, &ReadFileInput{FilePath: path})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)

	list := NewListDirectoryTool(ListDirectoryToolOptions{WorkspaceDir: dir})
	result, err = list.Call(ctx, &ListDirectoryInput{Path: dir})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}
