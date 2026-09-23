package toolkit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

func grepBackends(t *testing.T, run func(t *testing.T, useRipgrep bool)) {
	t.Helper()
	for _, backend := range []struct {
		name string
		rg   bool
	}{{"go", false}, {"ripgrep", true}} {
		t.Run(backend.name, func(t *testing.T) {
			if backend.rg {
				requireRipgrep(t)
			}
			run(t, backend.rg)
		})
	}
}

func TestGrepSearch_BackendParityForNestedAndHiddenFiles(t *testing.T) {
	grepBackends(t, func(t *testing.T, useRipgrep bool) {
		dir := t.TempDir()
		assert.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "root.go"), []byte("MATCH"), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "nested.go"), []byte("MATCH"), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, ".hidden.go"), []byte("MATCH"), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("sub/\n"), 0644))
		tool := NewGrepTool(GrepToolOptions{WorkspaceDir: dir, UseRipgrep: useRipgrep, DefaultExcludes: []string{}})
		result, err := tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "MATCH", Glob: "*.go"})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		for _, path := range []string{"root.go", "sub/nested.go", ".hidden.go"} {
			assert.Contains(t, result.Content[0].Text, path)
		}
	})
}

func TestGrepSearch_ContextAndMultiline(t *testing.T) {
	grepBackends(t, func(t *testing.T, useRipgrep bool) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha\nMATCH\nomega\nfoo\nbar\n"), 0644))
		tool := NewGrepTool(GrepToolOptions{WorkspaceDir: dir, UseRipgrep: useRipgrep})
		result, err := tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "MATCH", OutputMode: GrepOutputContent, Context: 1})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "1- alpha")
		assert.Contains(t, result.Content[0].Text, "2: MATCH")
		assert.Contains(t, result.Content[0].Text, "3- omega")
		result, err = tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "foo\\nbar", OutputMode: GrepOutputContent, Multiline: true})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "foo")
		assert.Contains(t, result.Content[0].Text, "bar")
	})
}

func TestGrepSearch_MultilineCountsEachMatch(t *testing.T) {
	grepBackends(t, func(t *testing.T, useRipgrep bool) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "matches.txt"), []byte("foo bar\n"), 0644))
		tool := NewGrepTool(GrepToolOptions{WorkspaceDir: dir, UseRipgrep: useRipgrep})
		result, err := tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "foo|bar", Multiline: true, OutputMode: GrepOutputCount})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, "matches.txt:2", result.Content[0].Text)
	})
}

func TestGrepSearch_LongLineAndMissingRoot(t *testing.T) {
	grepBackends(t, func(t *testing.T, useRipgrep bool) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "long.txt"), []byte(strings.Repeat("x", 70*1024)+"MATCH\n"), 0644))
		tool := NewGrepTool(GrepToolOptions{WorkspaceDir: dir, UseRipgrep: useRipgrep})
		result, err := tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "MATCH"})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "long.txt")
		result, err = tool.Call(context.Background(), &GrepInput{Path: filepath.Join(dir, "missing"), Pattern: "MATCH"})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestGrepSearch_NoPhantomEOFLine(t *testing.T) {
	grepBackends(t, func(t *testing.T, useRipgrep bool) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "empty.txt"), nil, 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "trailing.txt"), []byte("one\n"), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "blank.txt"), []byte("one\n\n"), 0644))
		tool := NewGrepTool(GrepToolOptions{WorkspaceDir: dir, UseRipgrep: useRipgrep})
		result, err := tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "^$", OutputMode: GrepOutputCount})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, "blank.txt:1", result.Content[0].Text)
	})
}

func TestGrepSearch_RejectsSymlinkRoot(t *testing.T) {
	grepBackends(t, func(t *testing.T, useRipgrep bool) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		assert.NoError(t, os.Mkdir(target, 0755))
		assert.NoError(t, os.WriteFile(filepath.Join(target, "match.txt"), []byte("needle\n"), 0644))
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		tool := NewGrepTool(GrepToolOptions{UseRipgrep: useRipgrep})
		result, err := tool.Call(context.Background(), &GrepInput{Path: link, Pattern: "needle"})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "symlink")
	})
}

func TestGrepSearch_CountsAreCompleteAcrossPages(t *testing.T) {
	grepBackends(t, func(t *testing.T, useRipgrep bool) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte(strings.Repeat("match\n", 6)), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("match\n"), 0644))
		tool := NewGrepTool(GrepToolOptions{WorkspaceDir: dir, UseRipgrep: useRipgrep, MaxResults: 1})
		result, err := tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "match", OutputMode: GrepOutputCount})
		assert.NoError(t, err)
		assert.Equal(t, "a.txt:6", result.Content[0].Text)
		assert.Contains(t, result.Content[1].Text, "offset 1")
		result, err = tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "match", OutputMode: GrepOutputCount, Offset: 1})
		assert.NoError(t, err)
		assert.Equal(t, "b.txt:1", result.Content[0].Text)
		assert.False(t, strings.Contains(result.Content[1].Text, "continue"))
	})
}

func TestGrepSearch_SingleFileAndInputErrors(t *testing.T) {
	grepBackends(t, func(t *testing.T, useRipgrep bool) {
		dir := t.TempDir()
		path := filepath.Join(dir, "single.go")
		assert.NoError(t, os.WriteFile(path, []byte("needle\n"), 0644))
		tool := NewGrepTool(GrepToolOptions{WorkspaceDir: dir, UseRipgrep: useRipgrep})
		result, err := tool.Call(context.Background(), &GrepInput{Path: path, Pattern: "needle"})
		assert.NoError(t, err)
		assert.Equal(t, "single.go", result.Content[0].Text)
		result, err = tool.Call(context.Background(), &GrepInput{Path: path, Pattern: "needle", Glob: "*.ts"})
		assert.NoError(t, err)
		assert.Contains(t, result.Content[0].Text, "No matches found")
		result, err = tool.Call(context.Background(), &GrepInput{Path: path, Pattern: "needle", Type: "ts"})
		assert.NoError(t, err)
		assert.Contains(t, result.Content[0].Text, "No matches found")
		result, err = tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "needle", OutputMode: "unknown"})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		result, err = tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "needle", Offset: -1})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
	})
}

func TestGrepSearch_SpecialFileDoesNotBlock(t *testing.T) {
	if _, err := exec.LookPath("mkfifo"); err != nil {
		t.Skip("mkfifo not available")
	}
	dir := t.TempDir()
	pipe := filepath.Join(dir, "pipe.txt")
	assert.NoError(t, exec.Command("mkfifo", pipe).Run())
	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: dir})
	done := make(chan error, 1)
	go func() {
		result, err := tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "match"})
		if err == nil && (result.IsError || !strings.Contains(result.Content[0].Text, "No matches found")) {
			err = context.DeadlineExceeded
		}
		done <- err
	}()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("grep blocked while opening a FIFO")
	}
}

func TestGrepSearch_OutputCapIsVisibleToModel(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "large.txt"), []byte(strings.Repeat("MATCH"+strings.Repeat("x", 16000)+"\n", 100)), 0644))
	tool := NewGrepTool(GrepToolOptions{WorkspaceDir: dir})
	result, err := tool.Call(context.Background(), &GrepInput{Path: dir, Pattern: "MATCH", OutputMode: GrepOutputContent})
	assert.NoError(t, err)
	assert.False(t, result.IsError)
	assert.True(t, len(result.Content[0].Text) <= maxGrepOutputBytes)
	assert.Contains(t, result.Content[1].Text, "Output text was capped")
}
