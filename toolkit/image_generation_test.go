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
}

func TestImageGenerationTool_WorkDir(t *testing.T) {
	tool := NewImageGenerationTool("test-model", WithImageToolWorkDir("/tmp/images"))
	inner := tool.Unwrap().(*imageGenerationTool)
	assert.Equal(t, "/tmp/images", inner.workDir)
}
