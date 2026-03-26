package media

import (
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestOptions(t *testing.T) {
	c := &Config{}
	c.Apply(
		WithModel("imagen-4.0-generate-001"),
		WithAspectRatio(Aspect16x9),
		WithOutputFormat(FormatPNG),
		WithCount(3),
		WithDuration(8*time.Second),
		WithTimeout(2*time.Minute),
		WithReferenceImage([]byte{1, 2, 3}),
	)

	assert.Equal(t, "imagen-4.0-generate-001", c.Model)
	assert.Equal(t, Aspect16x9, c.AspectRatio)
	assert.Equal(t, FormatPNG, c.OutputFormat)
	assert.Equal(t, 3, c.Count)
	assert.Equal(t, 8*time.Second, c.Duration)
	assert.Equal(t, 2*time.Minute, c.Timeout)
	assert.Equal(t, 1, len(c.ReferenceImages))
	assert.Equal(t, []byte{1, 2, 3}, c.ReferenceImages[0])
}

func TestOptions_Defaults(t *testing.T) {
	c := &Config{}
	c.Apply()
	assert.Equal(t, 1, c.Count)
	assert.Equal(t, "", c.Model)
	assert.Equal(t, AspectAuto, c.AspectRatio)
}

func TestWithModels(t *testing.T) {
	c := &Config{}
	c.Apply(WithModels("model-a", "model-b", "model-c"))
	assert.Equal(t, 3, len(c.Models))
	assert.Equal(t, "model-a", c.Models[0])
	assert.Equal(t, "model-c", c.Models[2])
}

func TestWithReferenceImage_Multiple(t *testing.T) {
	c := &Config{}
	c.Apply(
		WithReferenceImage([]byte{1}),
		WithReferenceImage([]byte{2}),
	)
	assert.Equal(t, 2, len(c.ReferenceImages))
}
