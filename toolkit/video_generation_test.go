package toolkit

import (
	"context"
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

func TestVideoGenerationTool_VeoDurations(t *testing.T) {
	veo3 := NewVideoGenerationTool("veo-3.1-fast-generate-001").Schema().Properties["duration"]
	assert.Equal(t, []any{"4s", "6s", "8s"}, veo3.Enum)

	veo2 := NewVideoGenerationTool("veo-2.0-generate-001").Schema().Properties["duration"]
	assert.Equal(t, []any{"5s", "6s", "7s", "8s"}, veo2.Enum)

	other := NewVideoGenerationTool("sora-2").Schema().Properties["duration"]
	assert.Nil(t, other.Enum)
	assert.NotContains(t, other.Description, "16s")
}

func TestVideoGenerationTool_RejectsVeoDuration(t *testing.T) {
	tool := NewVideoGenerationTool("veo-3.1-fast-generate-001")
	inner := tool.Unwrap().(*videoGenerationTool)
	result, err := inner.Call(context.Background(), &VideoGenerationInput{
		Prompt:   "clouds",
		Duration: "16s",
	})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "4s, 6s, 8s")
}
