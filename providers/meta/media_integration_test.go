//go:build integration

package meta

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestIntegration_GenerateImage(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := NewMediaProvider()
	ctx := testContext(t, 300*time.Second)

	config := &media.Config{Model: ModelMuseImage, OutputFormat: media.FormatPNG}
	config.Apply()

	results, err := provider.GenerateImage(ctx, "a simple flat-design red circle on a white background", config)
	assert.NoError(t, err)
	assert.True(t, len(results) > 0)

	image := results[0]
	t.Logf("image: %d bytes, %s, %dx%d", len(image.Data), image.Format, image.Width, image.Height)
	assert.True(t, len(image.Data) > 0)
	assert.Equal(t, image.Model, ModelMuseImage)
	assert.True(t, image.Width > 0 && image.Height > 0, "dimensions should decode")

	if dir := os.Getenv("META_IMAGE_OUT"); dir != "" {
		path := filepath.Join(dir, "muse-image"+image.Format.FileExtension())
		assert.NoError(t, os.WriteFile(path, image.Data, 0o644))
		t.Logf("wrote %s", path)
	}
}

// TestIntegration_EditImage covers the multipart upload path, which is where
// the malformed part filename lived.
func TestIntegration_EditImage(t *testing.T) {
	skipIfNoAPIKey(t)

	provider := NewMediaProvider()
	ctx := testContext(t, 300*time.Second)

	seedConfig := &media.Config{Model: ModelMuseImage, OutputFormat: media.FormatPNG}
	seedConfig.Apply()
	seed, err := provider.GenerateImage(ctx, "a plain white square on a black background", seedConfig)
	assert.NoError(t, err)
	assert.True(t, len(seed) > 0)

	editConfig := &media.Config{
		Model:           ModelMuseImage,
		OutputFormat:    media.FormatPNG,
		ReferenceImages: [][]byte{seed[0].Data},
	}
	editConfig.Apply()

	edited, err := provider.EditImage(ctx, "make the square blue", editConfig)
	assert.NoError(t, err)
	assert.True(t, len(edited) > 0)
	t.Logf("edited: %d bytes, %s, %dx%d",
		len(edited[0].Data), edited[0].Format, edited[0].Width, edited[0].Height)
	assert.True(t, len(edited[0].Data) > 0)
}

func TestIntegration_EditImageRequiresAReference(t *testing.T) {
	skipIfNoAPIKey(t)

	config := &media.Config{Model: ModelMuseImage}
	config.Apply()
	_, err := NewMediaProvider().EditImage(testContext(t, 30*time.Second), "anything", config)
	assert.ErrorContains(t, err, "reference image")
}
