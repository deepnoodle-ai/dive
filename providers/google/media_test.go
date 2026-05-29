package google

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/wonton/assert"
	"google.golang.org/genai"
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

func TestGoogleSpeechMatcher(t *testing.T) {
	matcher := func(model string) bool {
		lower := strings.ToLower(model)
		return strings.HasPrefix(lower, "gemini-") && strings.Contains(lower, "tts")
	}
	assert.True(t, matcher("gemini-3.1-flash-tts-preview"))
	assert.True(t, matcher("gemini-2.5-flash-preview-tts"))
	assert.True(t, matcher("gemini-2.5-pro-preview-tts"))
	assert.True(t, !matcher("gemini-2.5-flash"))
	assert.True(t, !matcher("gpt-4o-mini-tts"))
}

func TestGoogleTranscriptionMatcher(t *testing.T) {
	matcher := media.PrefixMatcher("gemini-")
	assert.True(t, matcher("gemini-2.5-flash"))
	assert.True(t, matcher("gemini-3.1-flash-lite"))
	assert.True(t, !matcher("gpt-4o-transcribe"))
}

func TestFirstInlineAudio(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Parts: []*genai.Part{genai.NewPartFromBytes([]byte{1, 2, 3}, "audio/pcm")},
			},
		}},
	}

	data, mimeType, err := firstInlineAudio(resp)
	assert.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, data)
	assert.Equal(t, "audio/pcm", mimeType)
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

func requireGoogleMediaIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_INTEGRATION_TESTS") == "" {
		t.Skip("RUN_INTEGRATION_TESTS not set")
	}
	if os.Getenv("GOOGLE_API_KEY") == "" && os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY or GEMINI_API_KEY not set")
	}
}

func TestGoogleGenerateImage_Gemini_Integration(t *testing.T) {
	requireGoogleMediaIntegration(t)

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
	requireGoogleMediaIntegration(t)

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
	requireGoogleMediaIntegration(t)

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
	requireGoogleMediaIntegration(t)

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

func TestGoogleTextToSpeech_Integration(t *testing.T) {
	requireGoogleMediaIntegration(t)

	p := NewMediaProvider()
	config := &media.Config{
		Model:       "gemini-3.1-flash-tts-preview",
		Voice:       "Kore",
		AudioFormat: media.AudioFormatWAV,
	}
	result, err := p.TextToSpeech(context.Background(), "Say cheerfully: Hello from Dive.", config)
	assert.NoError(t, err)
	assert.True(t, len(result.Data) > 0)
	assert.Equal(t, media.AudioFormatWAV, result.Format)
}

func TestGoogleTranscribe_Integration(t *testing.T) {
	requireGoogleMediaIntegration(t)

	speech, err := NewMediaProvider().TextToSpeech(context.Background(), "This is a Dive transcription test.", &media.Config{
		Model:       "gemini-3.1-flash-tts-preview",
		Voice:       "Kore",
		AudioFormat: media.AudioFormatWAV,
	})
	assert.NoError(t, err)

	p := NewMediaProvider()
	result, err := p.Transcribe(context.Background(), speech.Data, &media.Config{
		Model:         "gemini-3.5-flash",
		AudioMIMEType: "audio/wav",
	})
	assert.NoError(t, err)
	assert.Contains(t, result.Text, "Dive")
}
