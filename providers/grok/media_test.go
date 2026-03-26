package grok

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestGrokDurationToSeconds(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "5"},            // below minimum → default
		{4 * time.Second, "4"},
		{5 * time.Second, "5"},
		{8 * time.Second, "8"},
		{10 * time.Second, "10"},
		{12 * time.Second, "12"},
		{15 * time.Second, "15"},
		{20 * time.Second, "15"}, // clamped to max
	}
	for _, tt := range tests {
		t.Run(tt.d.String(), func(t *testing.T) {
			assert.Equal(t, tt.want, grokDurationToSeconds(tt.d))
		})
	}
}

func TestGrokImageMatcher(t *testing.T) {
	matcher := media.PrefixMatcher("grok-imagine-image")
	assert.True(t, matcher("grok-imagine-image"))
	assert.True(t, matcher("grok-imagine-image-pro"))
	assert.True(t, !matcher("grok-4"))
	assert.True(t, !matcher("gpt-image-1"))
}

func TestGrokVideoMatcher(t *testing.T) {
	matcher := media.PrefixMatcher("grok-imagine-video")
	assert.True(t, matcher("grok-imagine-video"))
	assert.True(t, !matcher("grok-imagine-image"))
	assert.True(t, !matcher("sora-2"))
}

func requireGrokMediaIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("RUN_INTEGRATION_TESTS not set")
	}
	if os.Getenv("XAI_API_KEY") == "" && os.Getenv("GROK_API_KEY") == "" {
		t.Skip("XAI_API_KEY or GROK_API_KEY not set")
	}
}

func TestGrokGenerateImage_Integration(t *testing.T) {
	requireGrokMediaIntegration(t)

	p := NewMediaProvider()
	config := &media.Config{
		Model: ModelImagineImage,
		Count: 1,
	}
	results, err := p.GenerateImage(context.Background(), "a simple red circle on white", config)
	assert.NoError(t, err)
	assert.True(t, len(results) > 0)
	assert.True(t, len(results[0].Data) > 0)
	assert.Equal(t, ModelImagineImage, results[0].Model)
}

func TestGrokGenerateVideo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping video generation test in short mode")
	}
	requireGrokMediaIntegration(t)

	p := NewMediaProvider()
	config := &media.Config{
		Model:    ModelImagineVideo,
		Duration: 5 * time.Second,
	}
	result, err := p.GenerateVideo(context.Background(), "a gentle wave washing over sand", config)
	assert.NoError(t, err)
	assert.True(t, len(result.Data) > 0)
	assert.Equal(t, "mp4", result.Format)
}
