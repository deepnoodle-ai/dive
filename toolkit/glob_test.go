package toolkit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestGlobTool_Name(t *testing.T) {
	tool := NewGlobTool()
	assert.Equal(t, "Glob", tool.Name())
}

func TestGlobTool_Schema(t *testing.T) {
	tool := NewGlobTool()
	schema := tool.Schema()

	assert.Equal(t, "object", string(schema.Type))
	assert.Contains(t, schema.Required, "pattern")
	assert.Contains(t, schema.Properties, "path")
}

func TestGlobTool_Annotations(t *testing.T) {
	tool := NewGlobTool()
	annotations := tool.Annotations()

	assert.Equal(t, "Glob", annotations.Title)
	assert.True(t, annotations.ReadOnlyHint)
	assert.False(t, annotations.DestructiveHint)
	assert.True(t, annotations.IdempotentHint)
}

func TestGlobTool_BasicPattern(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file1.go"), []byte("package main"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file2.go"), []byte("package main"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("text"), 0644))

	tool := NewGlobTool(GlobToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.go",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "file1.go")
	assert.Contains(t, output, "file2.go")
	assert.NotContains(t, output, "file.txt")
}

func TestGlobTool_RecursivePattern(t *testing.T) {
	tempDir := t.TempDir()

	// Create nested directory structure
	subDir := filepath.Join(tempDir, "sub")
	assert.NoError(t, os.MkdirAll(subDir, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "root.go"), []byte("package main"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(subDir, "nested.go"), []byte("package sub"), 0644))

	tool := NewGlobTool(GlobToolOptions{WorkspaceDir: tempDir})

	// Test pattern that matches nested files
	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "sub/*.go",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "nested.go")

	// Test pattern that matches root files
	result2, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.go",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result2.IsError)
	assert.Contains(t, result2.Content[0].Text, "root.go")
}

func TestGlobTool_NoMatches(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("text"), 0644))

	tool := NewGlobTool(GlobToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.go",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "No matching files")
}

func TestGlobTool_DefaultExcludes(t *testing.T) {
	tempDir := t.TempDir()

	// Create files in excluded directories
	nodeModules := filepath.Join(tempDir, "node_modules")
	gitDir := filepath.Join(tempDir, ".git")
	assert.NoError(t, os.MkdirAll(nodeModules, 0755))
	assert.NoError(t, os.MkdirAll(gitDir, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.js"), []byte("code"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(nodeModules, "dep.js"), []byte("dep"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte("git"), 0644))

	tool := NewGlobTool(GlobToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.js",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "main.js")
	// node_modules files should not appear with *.js pattern since they're in a subdirectory
}

func TestGlobTool_MaxResults(t *testing.T) {
	tempDir := t.TempDir()

	// Create more files than max results
	for i := 0; i < 10; i++ {
		filename := filepath.Join(tempDir, strings.Repeat("a", i+1)+".txt")
		assert.NoError(t, os.WriteFile(filename, []byte("content"), 0644))
	}

	tool := NewGlobTool(GlobToolOptions{
		WorkspaceDir: tempDir,
		MaxResults:   5,
	})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.txt",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	// Count results
	output := result.Content[0].Text
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Equal(t, 5, len(lines))
}

func TestGlobTool_InvalidPattern(t *testing.T) {
	tempDir := t.TempDir()
	tool := NewGlobTool(GlobToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "[invalid",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "Invalid glob pattern")
}

func TestGlobTool_NonExistentPath(t *testing.T) {
	tempDir := t.TempDir()
	tool := NewGlobTool(GlobToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.txt",
		Path:    filepath.Join(tempDir, "nonexistent"),
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "does not exist")
}

func TestGlobTool_FileAsPath(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	assert.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	tool := NewGlobTool(GlobToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.txt",
		Path:    testFile,
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "not a directory")
}

func TestGlobTool_OutsideWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	outsideDir := t.TempDir()

	tool := NewGlobTool(GlobToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.txt",
		Path:    outsideDir,
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "outside workspace")
}

func TestGlobTool_NoValidator(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("content"), 0644))

	// Create tool without workspace dir
	tool := NewGlobTool()

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.txt",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "test.txt")
}

func TestGlobTool_DefaultPath(t *testing.T) {
	// When no path provided, should use current working directory
	tool := NewGlobTool()

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.go",
	})

	// Should not error - will search cwd
	assert.NoError(t, err)
	assert.False(t, result.IsError)
}

func TestGlobTool_BraceExpansion(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file.js"), []byte("js"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file.ts"), []byte("ts"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("txt"), 0644))

	tool := NewGlobTool(GlobToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.{js,ts}",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "file.js")
	assert.Contains(t, output, "file.ts")
	assert.NotContains(t, output, "file.txt")
}

func TestGlobTool_PreviewCall(t *testing.T) {
	tool := NewGlobTool()
	ctx := context.Background()

	input := &GlobInput{
		Pattern: "**/*.go",
		Path:    "/some/path",
	}

	preview := tool.PreviewCall(ctx, input)
	assert.Contains(t, preview.Summary, "**/*.go")
	assert.Contains(t, preview.Summary, "/some/path")
}

func TestGlobTool_CustomExcludes(t *testing.T) {
	tempDir := t.TempDir()

	// Create files
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("code"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "main_test.go"), []byte("test"), 0644))

	tool := NewGlobTool(GlobToolOptions{
		WorkspaceDir:    tempDir,
		DefaultExcludes: []string{"*_test.go"},
	})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "*.go",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "main.go")
	assert.NotContains(t, output, "main_test.go")
}

func TestGlobTool_EmptyDefaultExcludesDisablesBuiltInExcludes(t *testing.T) {
	tempDir := t.TempDir()
	nodeModules := filepath.Join(tempDir, "node_modules")
	assert.NoError(t, os.MkdirAll(nodeModules, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.js"), []byte("code"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(nodeModules, "dep.js"), []byte("dep"), 0644))

	tool := NewGlobTool(GlobToolOptions{
		WorkspaceDir:    tempDir,
		DefaultExcludes: []string{},
	})

	result, err := tool.Call(context.Background(), &GlobInput{
		Pattern: "**/*.js",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "node_modules/dep.js")
}

func TestGlobTool_Call_ReturnsConfigError(t *testing.T) {
	tool := &GlobTool{
		configErr: errors.New("validator init failed"),
	}

	result, err := tool.Call(context.Background(), &GlobInput{Pattern: "*.go"})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "validator init failed")
}

func TestGlobTool_Call_ReturnsWorkspaceConfigErrorWhenValidatorMissing(t *testing.T) {
	tool := &GlobTool{workspaceDir: "/bad/workspace"}

	result, err := tool.Call(context.Background(), &GlobInput{Pattern: "*.go"})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "WorkspaceDir \"/bad/workspace\"")
	assert.Contains(t, result.Content[0].Text, "path validator is not initialized")
}
