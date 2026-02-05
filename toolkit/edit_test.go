package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestEditTool_Name(t *testing.T) {
	tool := NewEditTool()
	assert.Equal(t, "Edit", tool.Name())
}

func TestEditTool_Schema(t *testing.T) {
	tool := NewEditTool()
	schema := tool.Schema()

	assert.Equal(t, "object", string(schema.Type))
	assert.Contains(t, schema.Required, "file_path")
	assert.Contains(t, schema.Required, "old_string")
	assert.Contains(t, schema.Required, "new_string")
	assert.Contains(t, schema.Properties, "replace_all")
}

func TestEditTool_Annotations(t *testing.T) {
	tool := NewEditTool()
	annotations := tool.Annotations()

	assert.Equal(t, "Edit", annotations.Title)
	assert.False(t, annotations.ReadOnlyHint)
	assert.True(t, annotations.DestructiveHint)
	assert.True(t, annotations.IdempotentHint)
	assert.True(t, annotations.EditHint)
}

func TestEditTool_BasicReplacement(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("Hello, World!"), 0644)
	assert.NoError(t, err)

	tool := NewEditTool(EditToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  testFile,
		OldString: "World",
		NewString: "Universe",
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	content, err := os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, "Hello, Universe!", string(content))
}

func TestEditTool_ReplaceAll(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("foo bar foo baz foo"), 0644)
	assert.NoError(t, err)

	tool := NewEditTool(EditToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:   testFile,
		OldString:  "foo",
		NewString:  "qux",
		ReplaceAll: true,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	content, err := os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, "qux bar qux baz qux", string(content))
}

func TestEditTool_MultipleOccurrencesError(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("foo bar foo"), 0644)
	assert.NoError(t, err)

	tool := NewEditTool(EditToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  testFile,
		OldString: "foo",
		NewString: "qux",
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "appears 2 times")
}

func TestEditTool_OldStringNotFound(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("Hello, World!"), 0644)
	assert.NoError(t, err)

	tool := NewEditTool(EditToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  testFile,
		OldString: "NotFound",
		NewString: "Replacement",
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "not found")
}

func TestEditTool_SameOldAndNew(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("Hello, World!"), 0644)
	assert.NoError(t, err)

	tool := NewEditTool(EditToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  testFile,
		OldString: "World",
		NewString: "World",
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "must be different")
}

func TestEditTool_RelativePath(t *testing.T) {
	tempDir := t.TempDir()
	tool := NewEditTool(EditToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  "relative/path.txt",
		OldString: "old",
		NewString: "new",
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "must be absolute")
}

func TestEditTool_NonExistentFile(t *testing.T) {
	tempDir := t.TempDir()
	tool := NewEditTool(EditToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  filepath.Join(tempDir, "nonexistent.txt"),
		OldString: "old",
		NewString: "new",
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "does not exist")
}

func TestEditTool_Directory(t *testing.T) {
	tempDir := t.TempDir()
	tool := NewEditTool(EditToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  tempDir,
		OldString: "old",
		NewString: "new",
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "directory")
}

func TestEditTool_OutsideWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "outside.txt")
	err := os.WriteFile(outsideFile, []byte("content"), 0644)
	assert.NoError(t, err)

	tool := NewEditTool(EditToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  outsideFile,
		OldString: "content",
		NewString: "new",
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "outside workspace")
}

func TestEditTool_NoValidator(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("Hello, World!"), 0644)
	assert.NoError(t, err)

	// Create tool without workspace dir - should allow all operations
	tool := NewEditTool()

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  testFile,
		OldString: "World",
		NewString: "Universe",
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	content, err := os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, "Hello, Universe!", string(content))
}

func TestEditTool_MultilineReplacement(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	original := "line 1\nline 2\nline 3"
	err := os.WriteFile(testFile, []byte(original), 0644)
	assert.NoError(t, err)

	tool := NewEditTool(EditToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  testFile,
		OldString: "line 2",
		NewString: "new line 2\nnew line 2b",
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	content, err := os.ReadFile(testFile)
	assert.NoError(t, err)
	assert.Equal(t, "line 1\nnew line 2\nnew line 2b\nline 3", string(content))
}

func TestEditTool_PreviewCall(t *testing.T) {
	tool := NewEditTool()
	ctx := context.Background()

	input := &EditInput{
		FilePath:  "/path/to/file.txt",
		OldString: "old text",
		NewString: "new text",
	}

	preview := tool.PreviewCall(ctx, input)
	assert.Contains(t, preview.Summary, "file.txt")
	assert.Contains(t, preview.Summary, "old text")
}

func TestEditTool_PreviewCallReplaceAll(t *testing.T) {
	tool := NewEditTool()
	ctx := context.Background()

	input := &EditInput{
		FilePath:   "/path/to/file.txt",
		OldString:  "old",
		NewString:  "new",
		ReplaceAll: true,
	}

	preview := tool.PreviewCall(ctx, input)
	assert.Contains(t, preview.Summary, "Replace all")
}

func TestEditTool_LargeFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "large.txt")

	// Create a file larger than max size
	tool := NewEditTool(EditToolOptions{
		WorkspaceDir: tempDir,
		MaxFileSize:  100,
	})

	largeContent := make([]byte, 200)
	for i := range largeContent {
		largeContent[i] = 'a'
	}
	err := os.WriteFile(testFile, largeContent, 0644)
	assert.NoError(t, err)

	result, err := tool.Call(context.Background(), &EditInput{
		FilePath:  testFile,
		OldString: "aaa",
		NewString: "bbb",
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "too large")
}
