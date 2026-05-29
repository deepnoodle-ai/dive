package openai

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestOpenAIAspectRatioToSize(t *testing.T) {
	tests := []struct {
		ar   media.AspectRatio
		size string
	}{
		{media.Aspect1x1, "1024x1024"},
		{media.Aspect16x9, "1536x1024"},
		{media.Aspect9x16, "1024x1536"},
		{media.AspectAuto, "auto"},
		{media.Aspect4x3, "1024x1024"}, // unsupported, falls to default
	}
	for _, tt := range tests {
		t.Run(string(tt.ar), func(t *testing.T) {
			assert.Equal(t, tt.size, aspectRatioToSize(tt.ar))
		})
	}
}

func TestOpenAIVideoSize(t *testing.T) {
	// sora-2 (720p)
	assert.Equal(t, "1280x720", aspectRatioToVideoSize(media.Aspect16x9, "sora-2"))
	assert.Equal(t, "720x1280", aspectRatioToVideoSize(media.Aspect9x16, "sora-2"))

	// sora-2-pro (1080p)
	assert.Equal(t, "1920x1080", aspectRatioToVideoSize(media.Aspect16x9, "sora-2-pro"))
	assert.Equal(t, "1080x1920", aspectRatioToVideoSize(media.Aspect9x16, "sora-2-pro"))
}

func TestOpenAIDurationToSeconds(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "8"},
		{4 * time.Second, "8"},
		{8 * time.Second, "8"},
		{12 * time.Second, "8"},
		{15 * time.Second, "8"},
		{16 * time.Second, "16"},
		{19 * time.Second, "16"},
		{20 * time.Second, "20"},
		{30 * time.Second, "20"},
	}
	for _, tt := range tests {
		t.Run(tt.d.String(), func(t *testing.T) {
			assert.Equal(t, tt.want, durationToSeconds(tt.d))
		})
	}
}

func TestOpenAIVideoSizeToAspectRatio(t *testing.T) {
	assert.Equal(t, media.Aspect16x9, videoSizeToAspectRatio("1280x720"))
	assert.Equal(t, media.Aspect16x9, videoSizeToAspectRatio("1920x1080"))
	assert.Equal(t, media.Aspect9x16, videoSizeToAspectRatio("720x1280"))
	assert.Equal(t, media.Aspect9x16, videoSizeToAspectRatio("1080x1920"))
	assert.Equal(t, media.Aspect16x9, videoSizeToAspectRatio("unknown"))
}

func TestOpenAIImageMatcher(t *testing.T) {
	matcher := media.PrefixMatcher("gpt-image-")
	assert.True(t, matcher("gpt-image-2"))
	assert.True(t, matcher("gpt-image-2-2026-04-21"))
	assert.True(t, matcher("gpt-image-1"))
	assert.True(t, matcher("gpt-image-1.5"))
	assert.True(t, matcher("gpt-image-1-mini"))
	assert.True(t, !matcher("gpt-4"))
	assert.True(t, !matcher("imagen-4"))
}

func TestOpenAIVideoMatcher(t *testing.T) {
	matcher := media.PrefixMatcher("sora-")
	assert.True(t, matcher("sora-2"))
	assert.True(t, matcher("sora-2-pro"))
	assert.True(t, !matcher("veo-3"))
}

func requireOpenAIMediaIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("RUN_INTEGRATION_TESTS not set")
	}
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
}

func TestOpenAIGenerateImage_Integration(t *testing.T) {
	requireOpenAIMediaIntegration(t)

	p := NewMediaProvider()
	config := &media.Config{
		Model:       ModelGPTImage2,
		AspectRatio: media.Aspect1x1,
		Count:       1,
	}
	results, err := p.GenerateImage(context.Background(), "a simple red circle on white", config)
	assert.NoError(t, err)
	assert.True(t, len(results) > 0)
	assert.True(t, len(results[0].Data) > 0)
	assert.Equal(t, ModelGPTImage2, results[0].Model)
}

func TestOpenAIGenerateVideo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping video generation test in short mode")
	}
	requireOpenAIMediaIntegration(t)

	p := NewMediaProvider()
	config := &media.Config{
		Model:       "sora-2",
		AspectRatio: media.Aspect16x9,
		Duration:    8 * time.Second,
	}
	result, err := p.GenerateVideo(context.Background(), "a gentle wave washing over sand", config)
	assert.NoError(t, err)
	assert.True(t, len(result.Data) > 0)
	assert.Equal(t, "mp4", result.Format)
}
