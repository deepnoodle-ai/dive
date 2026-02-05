package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestGrepTool_Name(t *testing.T) {
	tool := NewGrepTool()
	assert.Equal(t, "Grep", tool.Name())
}

func TestGrepTool_Schema(t *testing.T) {
	tool := NewGrepTool()
	schema := tool.Schema()

	assert.Equal(t, "object", string(schema.Type))
	assert.Contains(t, schema.Required, "pattern")
	assert.Contains(t, schema.Properties, "path")
	assert.Contains(t, schema.Properties, "glob")
	assert.Contains(t, schema.Properties, "type")
	assert.Contains(t, schema.Properties, "output_mode")
}

func TestGrepTool_Annotations(t *testing.T) {
	tool := NewGrepTool()
	annotations := tool.Annotations()

	assert.Equal(t, "Grep", annotations.Title)
	assert.True(t, annotations.ReadOnlyHint)
	assert.False(t, annotations.DestructiveHint)
	assert.True(t, annotations.IdempotentHint)
}

func TestGrepTool_BasicSearch(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("Hello World\nGoodbye World"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "file2.txt"), []byte("No match here"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "World",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "file1.txt")
	assert.NotContains(t, result.Content[0].Text, "file2.txt")
}

func TestGrepTool_FilesWithMatchesMode(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "match1.txt"), []byte("pattern here"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "match2.txt"), []byte("pattern there"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "nomatch.txt"), []byte("nothing"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern:    "pattern",
		Path:       tempDir,
		OutputMode: GrepOutputFilesWithMatches,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "match1.txt")
	assert.Contains(t, output, "match2.txt")
	assert.NotContains(t, output, "nomatch.txt")
}

func TestGrepTool_ContentMode(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("line 1\npattern line\nline 3"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern:    "pattern",
		Path:       tempDir,
		OutputMode: GrepOutputContent,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "pattern line")
	assert.Contains(t, output, "2:") // Line number
}

func TestGrepTool_CountMode(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("foo\nfoo\nbar\nfoo"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern:    "foo",
		Path:       tempDir,
		OutputMode: GrepOutputCount,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "test.txt:3")
}

func TestGrepTool_GlobFilter(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "code.go"), []byte("pattern in go"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "code.js"), []byte("pattern in js"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "pattern",
		Path:    tempDir,
		Glob:    "*.go",
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "code.go")
	assert.NotContains(t, output, "code.js")
}

func TestGrepTool_TypeFilter(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("func main"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.py"), []byte("def main"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "main",
		Path:    tempDir,
		Type:    "go",
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "main.go")
	assert.NotContains(t, output, "main.py")
}

func TestGrepTool_CaseInsensitive(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("Hello HELLO hello"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern:    "hello",
		Path:       tempDir,
		CaseInsens: true,
		OutputMode: GrepOutputContent,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "Hello HELLO hello")
}

func TestGrepTool_NoMatches(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("nothing here"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "notfound",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "No matches found")
}

func TestGrepTool_InvalidRegex(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("content"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "[invalid",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "Invalid regex")
}

func TestGrepTool_OutsideWorkspace(t *testing.T) {
	tempDir := t.TempDir()
	outsideDir := t.TempDir()

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "test",
		Path:    outsideDir,
	})

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "outside workspace")
}

func TestGrepTool_NoValidator(t *testing.T) {
	tempDir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("pattern match"), 0644))

	// Create tool without workspace dir
	tool := NewGrepTool()

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "pattern",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "test.txt")
}

func TestGrepTool_MaxResults(t *testing.T) {
	tempDir := t.TempDir()

	// Create file with many matches
	content := strings.Repeat("match\n", 20)
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte(content), 0644))

	tool := NewGrepTool(GrepToolOptions{
		WorkspaceDir: tempDir,
		MaxResults:   5,
	})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern:    "match",
		Path:       tempDir,
		OutputMode: GrepOutputContent,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Display, "limited to 5")
}

func TestGrepTool_HeadLimit(t *testing.T) {
	tempDir := t.TempDir()

	content := strings.Repeat("match\n", 20)
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte(content), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern:    "match",
		Path:       tempDir,
		OutputMode: GrepOutputContent,
		HeadLimit:  3,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Display, "limited to 3")
}

func TestGrepTool_DefaultExcludes(t *testing.T) {
	tempDir := t.TempDir()

	// Create files in excluded directories
	nodeModules := filepath.Join(tempDir, "node_modules")
	assert.NoError(t, os.MkdirAll(nodeModules, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.js"), []byte("pattern"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(nodeModules, "dep.js"), []byte("pattern"), 0644))

	// Use default excludes which should skip node_modules
	tool := NewGrepTool(GrepToolOptions{
		WorkspaceDir: tempDir,
		// Default excludes include **/node_modules/**
	})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "pattern",
		Path:    tempDir,
		Glob:    "*.js", // Only search .js files in root
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "main.js")
}

func TestGrepTool_SkipsBinaryFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create a binary file (contains null bytes)
	binaryContent := []byte{0x00, 0x01, 0x02, 'p', 'a', 't', 't', 'e', 'r', 'n'}
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "binary.bin"), binaryContent, 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "text.txt"), []byte("pattern"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "pattern",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "text.txt")
	assert.NotContains(t, output, "binary.bin")
}

func TestGrepTool_PreviewCall(t *testing.T) {
	tool := NewGrepTool()
	ctx := context.Background()

	input := &GrepInput{
		Pattern: "func.*main",
		Path:    "/some/path",
		Glob:    "*.go",
	}

	preview := tool.PreviewCall(ctx, input)
	assert.Contains(t, preview.Summary, "func.*main")
	assert.Contains(t, preview.Summary, "/some/path")
	assert.Contains(t, preview.Summary, "*.go")
}

func TestGrepTool_RegexPatterns(t *testing.T) {
	tempDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "test.txt"),
		[]byte("func main()\nfunc helper()\ndef main():"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern:    `func\s+\w+\(\)`,
		Path:       tempDir,
		OutputMode: GrepOutputContent,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "func main()")
	assert.Contains(t, output, "func helper()")
	assert.NotContains(t, output, "def main()")
}

func TestGrepTool_RecursiveSearch(t *testing.T) {
	tempDir := t.TempDir()

	// Create nested structure
	subDir := filepath.Join(tempDir, "sub", "deep")
	assert.NoError(t, os.MkdirAll(subDir, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(tempDir, "root.txt"), []byte("pattern"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(subDir, "deep.txt"), []byte("pattern"), 0644))

	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: tempDir})

	result, err := tool.Call(context.Background(), &GrepInput{
		Pattern: "pattern",
		Path:    tempDir,
	})

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	output := result.Content[0].Text
	assert.Contains(t, output, "root.txt")
	assert.Contains(t, output, "deep.txt")
}
