package toolkit

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestVideoGenerationTool_Name(t *testing.T) {
	tool := NewVideoGenerationTool("test-model")
	assert.Equal(t, "VideoGeneration", tool.Name())
}

func TestVideoGenerationTool_Description(t *testing.T) {
	tool := NewVideoGenerationTool("veo-3")
	desc := tool.Description()
	assert.Contains(t, desc, "veo-3")
	assert.Contains(t, desc, "video")
}

func TestVideoGenerationTool_Annotations(t *testing.T) {
	tool := NewVideoGenerationTool("test-model")
	ann := tool.Annotations()
	assert.Equal(t, "VideoGeneration", ann.Title)
	assert.True(t, !ann.ReadOnlyHint)
	assert.True(t, !ann.DestructiveHint)
	assert.True(t, ann.OpenWorldHint)
}

func TestVideoGenerationTool_Schema(t *testing.T) {
	tool := NewVideoGenerationTool("test-model")
	s := tool.Schema()
	assert.Equal(t, "object", string(s.Type))
	assert.Contains(t, s.Required, "prompt")
	assert.NotNil(t, s.Properties["prompt"])
	assert.NotNil(t, s.Properties["duration"])
	assert.NotNil(t, s.Properties["aspect_ratio"])
	assert.NotNil(t, s.Properties["output_path"])
}

func TestVideoGenerationTool_WorkDir(t *testing.T) {
	tool := NewVideoGenerationTool("test-model", WithVideoToolWorkDir("/tmp/videos"))
	inner := tool.Unwrap().(*videoGenerationTool)
	assert.Equal(t, "/tmp/videos", inner.workDir)
}
