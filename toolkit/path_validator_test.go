package toolkit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestNewPathValidator(t *testing.T) {
	t.Run("creates validator with given workspace", func(t *testing.T) {
		dir := t.TempDir()
		v, err := NewPathValidator(dir)
		assert.Nil(t, err)
		assert.True(t, v.WorkspaceDir != "")
	})

	t.Run("defaults to cwd when workspace is empty", func(t *testing.T) {
		v, err := NewPathValidator("")
		assert.Nil(t, err)
		cwd, _ := os.Getwd()
		realCwd, _ := filepath.EvalSymlinks(cwd)
		assert.Equal(t, realCwd, v.WorkspaceDir)
	})
}

func TestPathValidator_IsInWorkspace(t *testing.T) {
	dir := t.TempDir()
	v, err := NewPathValidator(dir)
	assert.Nil(t, err)

	t.Run("path inside workspace", func(t *testing.T) {
		path := filepath.Join(dir, "subdir", "file.txt")
		ok, err := v.IsInWorkspace(path)
		assert.Nil(t, err)
		assert.True(t, ok)
	})

	t.Run("workspace root itself", func(t *testing.T) {
		ok, err := v.IsInWorkspace(dir)
		assert.Nil(t, err)
		assert.True(t, ok)
	})

	t.Run("path outside workspace", func(t *testing.T) {
		ok, err := v.IsInWorkspace("/tmp")
		assert.Nil(t, err)
		assert.True(t, !ok)
	})

	t.Run("path traversal with dot-dot", func(t *testing.T) {
		path := filepath.Join(dir, "subdir", "..", "..", "etc", "passwd")
		ok, err := v.IsInWorkspace(path)
		assert.Nil(t, err)
		assert.True(t, !ok)
	})
}

func TestPathValidator_ValidateRead(t *testing.T) {
	dir := t.TempDir()
	v, err := NewPathValidator(dir)
	assert.Nil(t, err)

	t.Run("allows read inside workspace", func(t *testing.T) {
		err := v.ValidateRead(filepath.Join(dir, "file.txt"))
		assert.Nil(t, err)
	})

	t.Run("denies read outside workspace", func(t *testing.T) {
		err := v.ValidateRead("/etc/passwd")
		assert.NotNil(t, err)
		assert.True(t, IsPathAccessError(err))
	})
}

func TestPathValidator_ValidateWrite(t *testing.T) {
	dir := t.TempDir()
	v, err := NewPathValidator(dir)
	assert.Nil(t, err)

	t.Run("allows write inside workspace", func(t *testing.T) {
		err := v.ValidateWrite(filepath.Join(dir, "output.txt"))
		assert.Nil(t, err)
	})

	t.Run("denies write outside workspace", func(t *testing.T) {
		err := v.ValidateWrite("/tmp/malicious.txt")
		assert.NotNil(t, err)
		assert.True(t, IsPathAccessError(err))
	})
}

func TestPathValidator_ResolvePath(t *testing.T) {
	dir := t.TempDir()
	v, err := NewPathValidator(dir)
	assert.Nil(t, err)

	t.Run("resolves existing file", func(t *testing.T) {
		filePath := filepath.Join(dir, "test.txt")
		err := os.WriteFile(filePath, []byte("test"), 0644)
		assert.Nil(t, err)

		resolved, err := v.ResolvePath(filePath)
		assert.Nil(t, err)
		assert.True(t, filepath.IsAbs(resolved))
	})

	t.Run("resolves non-existent file in existing directory", func(t *testing.T) {
		filePath := filepath.Join(dir, "nonexistent.txt")
		resolved, err := v.ResolvePath(filePath)
		assert.Nil(t, err)
		assert.True(t, filepath.IsAbs(resolved))
	})

	t.Run("resolves symlinks", func(t *testing.T) {
		realDir := filepath.Join(dir, "real")
		err := os.Mkdir(realDir, 0755)
		assert.Nil(t, err)

		realFile := filepath.Join(realDir, "file.txt")
		err = os.WriteFile(realFile, []byte("hello"), 0644)
		assert.Nil(t, err)

		linkPath := filepath.Join(dir, "link")
		err = os.Symlink(realDir, linkPath)
		assert.Nil(t, err)

		resolved, err := v.ResolvePath(filepath.Join(linkPath, "file.txt"))
		assert.Nil(t, err)
		// The resolved path should point to the real location
		assert.True(t, filepath.IsAbs(resolved))
	})
}

func TestPathAccessError(t *testing.T) {
	t.Run("error message format", func(t *testing.T) {
		err := &PathAccessError{
			Path:      "/secret/file",
			Operation: "read",
			Reason:    "path is outside workspace",
			Workspace: "/workspace",
		}
		msg := err.Error()
		assert.True(t, len(msg) > 0)
		assert.True(t, IsPathAccessError(err))
	})

	t.Run("non-path-access error returns false", func(t *testing.T) {
		err := os.ErrNotExist
		assert.True(t, !IsPathAccessError(err))
	})
}
