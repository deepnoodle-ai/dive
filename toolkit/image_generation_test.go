package toolkit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestImageGenerationTool_Name(t *testing.T) {
	tool := NewImageGenerationTool("test-model")
	assert.Equal(t, "ImageGeneration", tool.Name())
}

func TestImageGenerationTool_Description(t *testing.T) {
	tool := NewImageGenerationTool("imagen-4")
	desc := tool.Description()
	assert.Contains(t, desc, "imagen-4")
	assert.Contains(t, desc, "image")
}

func TestImageGenerationTool_Annotations(t *testing.T) {
	tool := NewImageGenerationTool("test-model")
	ann := tool.Annotations()
	assert.Equal(t, "ImageGeneration", ann.Title)
	assert.True(t, !ann.ReadOnlyHint)
	assert.True(t, !ann.DestructiveHint)
	assert.True(t, ann.OpenWorldHint)
}

func TestImageGenerationTool_Schema(t *testing.T) {
	tool := NewImageGenerationTool("test-model")
	s := tool.Schema()
	assert.Equal(t, "object", string(s.Type))
	assert.Contains(t, s.Required, "prompt")
	assert.NotNil(t, s.Properties["prompt"])
	assert.NotNil(t, s.Properties["aspect_ratio"])
	assert.NotNil(t, s.Properties["output_path"])
	assert.NotNil(t, s.Properties["format"])

	// Verify only supported aspect ratios are advertised (no 4:3 or 3:4)
	arEnum := s.Properties["aspect_ratio"].Enum
	assert.Equal(t, 3, len(arEnum))
	assert.Contains(t, arEnum, any("1:1"))
	assert.Contains(t, arEnum, any("16:9"))
	assert.Contains(t, arEnum, any("9:16"))
}

func TestImageGenerationTool_Schema_NoDurationEnum(t *testing.T) {
	// Verify video tool duration has no enum (provider-specific)
	tool := NewVideoGenerationTool("test-model")
	s := tool.Schema()
	assert.Nil(t, s.Properties["duration"].Enum)
}

func TestImageGenerationTool_WorkDir(t *testing.T) {
	dir := t.TempDir()
	tool := NewImageGenerationTool("test-model", WithImageToolWorkDir(dir))
	inner := tool.Unwrap().(*imageGenerationTool)
	assert.Equal(t, dir, inner.workDir)
}

func TestValidateOutputPath(t *testing.T) {
	workDir := t.TempDir()
	// Resolve symlinks in workDir itself (e.g. /var -> /private/var on macOS)
	realWorkDir, err := filepath.EvalSymlinks(workDir)
	assert.Nil(t, err)

	// Valid relative path
	path, err := validateOutputPath("output.png", workDir)
	assert.Nil(t, err)
	assert.True(t, filepath.IsAbs(path))
	assert.Equal(t, filepath.Join(realWorkDir, "output.png"), path)

	// Valid nested relative path
	path, err = validateOutputPath(filepath.FromSlash("subdir/output.png"), workDir)
	assert.Nil(t, err)
	assert.True(t, filepath.IsAbs(path))
	assert.Equal(t, filepath.Join(realWorkDir, "subdir", "output.png"), path)

	// Absolute path rejected
	absPath := filepath.Join(workDir, "other", "file.png")
	_, err = validateOutputPath(absPath, workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "relative path")

	// Traversal attack rejected
	_, err = validateOutputPath(filepath.FromSlash("../../etc/passwd"), workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "within the working directory")

	// Traversal with nested path rejected
	_, err = validateOutputPath(filepath.FromSlash("subdir/../../etc/passwd"), workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "within the working directory")
}

func TestValidateOutputPath_SymlinkTraversal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require elevated privileges on Windows")
	}

	workDir := t.TempDir()
	outsideDir := t.TempDir()

	// Create a file outside the workDir
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	os.WriteFile(outsideFile, []byte("secret"), 0o644)

	// Create a symlink inside workDir pointing outside
	linkPath := filepath.Join(workDir, "escape")
	err := os.Symlink(outsideDir, linkPath)
	assert.Nil(t, err)

	// Attempting to write through the symlink should be rejected
	_, err = validateOutputPath(filepath.FromSlash("escape/secret.txt"), workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "within the working directory")
}
