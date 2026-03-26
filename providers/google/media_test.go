package google

import (
	"context"
	"os"
	"testing"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestGoogleImageMatcher(t *testing.T) {
	matcher := media.PrefixesMatcher("gemini-", "imagen-")
	// Gemini image models
	assert.True(t, matcher("gemini-3.1-flash-image-preview"))
	assert.True(t, matcher("gemini-2.5-flash-image"))
	assert.True(t, matcher("gemini-3-pro-image-preview"))
	// Imagen 4 models (current)
	assert.True(t, matcher("imagen-4.0-generate-001"))
	assert.True(t, matcher("imagen-4.0-ultra-generate-001"))
	assert.True(t, matcher("imagen-4.0-fast-generate-001"))
	// Negative
	assert.True(t, !matcher("gpt-image-1"))
	assert.True(t, !matcher("veo-3.1-generate-preview"))
}

func TestGoogleVideoMatcher(t *testing.T) {
	matcher := media.PrefixMatcher("veo-")
	assert.True(t, matcher("veo-3.1-generate-preview"))
	assert.True(t, matcher("veo-3-generate-preview"))
	assert.True(t, matcher("veo-2-generate-preview"))
	assert.True(t, !matcher("sora-2"))
	assert.True(t, !matcher("gemini-3.1-flash-image-preview"))
}

func TestMediaProviderOptions(t *testing.T) {
	p := NewMediaProvider(
		WithMediaAPIKey("test-key"),
		WithMediaVertexAI("us-central1"),
		WithMediaImagenLocation("us-east1"),
	)
	assert.Equal(t, "test-key", p.apiKey)
	assert.True(t, p.vertexAI)
	assert.Equal(t, "us-central1", p.location)
	assert.Equal(t, "us-east1", p.imagenLocation)
}

func requireGoogleMediaAPIKey(t *testing.T) {
	t.Helper()
	if os.Getenv("GOOGLE_API_KEY") == "" && os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY or GEMINI_API_KEY not set")
	}
}

func TestGoogleGenerateImage_Gemini_Integration(t *testing.T) {
	requireGoogleMediaAPIKey(t)

	p := NewMediaProvider()
	config := &media.Config{
		Model:       "gemini-2.5-flash-image",
		AspectRatio: media.Aspect1x1,
		Count:       1,
	}
	results, err := p.GenerateImage(context.Background(), "a simple red circle on a white background", config)
	assert.NoError(t, err)
	assert.True(t, len(results) > 0)
	assert.True(t, len(results[0].Data) > 0)
	assert.Equal(t, "gemini-2.5-flash-image", results[0].Model)
	assert.True(t, results[0].Width > 0)
	assert.True(t, results[0].Height > 0)
}

func TestGoogleGenerateImage_Imagen4_Integration(t *testing.T) {
	requireGoogleMediaAPIKey(t)

	p := NewMediaProvider()
	config := &media.Config{
		Model:       "imagen-4.0-generate-001",
		AspectRatio: media.Aspect1x1,
		Count:       1,
	}
	results, err := p.GenerateImage(context.Background(), "a simple blue square on a white background", config)
	assert.NoError(t, err)
	assert.True(t, len(results) > 0)
	assert.True(t, len(results[0].Data) > 0)
	assert.Equal(t, "imagen-4.0-generate-001", results[0].Model)
}

func TestGoogleGenerateImage_FormatConversion_Integration(t *testing.T) {
	requireGoogleMediaAPIKey(t)

	p := NewMediaProvider()
	config := &media.Config{
		Model:        "gemini-2.5-flash-image",
		AspectRatio:  media.Aspect1x1,
		Count:        1,
		OutputFormat: media.FormatJPEG,
	}
	results, err := p.GenerateImage(context.Background(), "a green triangle", config)
	assert.NoError(t, err)
	assert.True(t, len(results) > 0)
	assert.Equal(t, media.FormatJPEG, results[0].Format)
}

func TestGoogleGenerateVideo_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping video generation test in short mode")
	}
	requireGoogleMediaAPIKey(t)

	p := NewMediaProvider()
	config := &media.Config{
		Model:       "veo-3.1-generate-preview",
		AspectRatio: media.Aspect16x9,
	}
	result, err := p.GenerateVideo(context.Background(), "a leaf falling from a tree", config)
	assert.NoError(t, err)
	assert.True(t, len(result.Data) > 0)
	assert.Equal(t, "veo-3.1-generate-preview", result.Model)
	assert.Equal(t, "mp4", result.Format)
}
