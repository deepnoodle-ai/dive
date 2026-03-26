package toolkit

import (
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
	tool := NewImageGenerationTool("test-model", WithImageToolWorkDir("/tmp/images"))
	inner := tool.Unwrap().(*imageGenerationTool)
	assert.Equal(t, "/tmp/images", inner.workDir)
}

func TestResolveOutputPath(t *testing.T) {
	workDir := "/tmp/workdir"

	// Valid relative path
	path, err := resolveOutputPath("output.png", workDir)
	assert.Nil(t, err)
	assert.Equal(t, "/tmp/workdir/output.png", path)

	// Valid nested relative path
	path, err = resolveOutputPath("subdir/output.png", workDir)
	assert.Nil(t, err)
	assert.Equal(t, "/tmp/workdir/subdir/output.png", path)

	// Absolute path rejected
	_, err = resolveOutputPath("/etc/passwd", workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "relative path")

	// Traversal attack rejected
	_, err = resolveOutputPath("../../etc/passwd", workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "within the working directory")

	// Traversal with nested path rejected
	_, err = resolveOutputPath("subdir/../../etc/passwd", workDir)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "within the working directory")
}
