package openai

import (
	"context"
	"os"
	"strings"
	"testing"

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

func TestOpenAISpeechMatcher(t *testing.T) {
	matcher := media.PrefixesMatcher("tts-", "gpt-4o-mini-tts")
	assert.True(t, matcher("tts-1"))
	assert.True(t, matcher("tts-1-hd"))
	assert.True(t, matcher("gpt-4o-mini-tts"))
	assert.True(t, matcher("gpt-4o-mini-tts-2025-12-15"))
	assert.True(t, !matcher("gpt-4o-mini-transcribe"))
}

func TestOpenAITranscriptionMatcher(t *testing.T) {
	matcher := media.PrefixesMatcher("whisper-", "gpt-4o-transcribe", "gpt-4o-mini-transcribe")
	assert.True(t, matcher("whisper-1"))
	assert.True(t, matcher("gpt-4o-transcribe"))
	assert.True(t, matcher("gpt-4o-mini-transcribe"))
	assert.True(t, matcher("gpt-4o-transcribe-diarize"))
	assert.True(t, !matcher("gpt-4o-mini-tts"))
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

func TestOpenAITextToSpeech_Integration(t *testing.T) {
	requireOpenAIMediaIntegration(t)

	p := NewMediaProvider()
	config := &media.Config{
		Model:       "gpt-4o-mini-tts",
		Voice:       "alloy",
		AudioFormat: media.AudioFormatMP3,
	}
	result, err := p.TextToSpeech(context.Background(), "Hello from Dive.", config)
	assert.NoError(t, err)
	assert.True(t, len(result.Data) > 0)
	assert.Equal(t, media.AudioFormatMP3, result.Format)
}

func TestOpenAITranscribe_Integration(t *testing.T) {
	requireOpenAIMediaIntegration(t)

	speech, err := NewMediaProvider().TextToSpeech(context.Background(), "This is a Dive transcription test.", &media.Config{
		Model:       "gpt-4o-mini-tts",
		Voice:       "alloy",
		AudioFormat: media.AudioFormatMP3,
	})
	assert.NoError(t, err)

	p := NewMediaProvider()
	result, err := p.Transcribe(context.Background(), speech.Data, &media.Config{
		Model:         "gpt-4o-mini-transcribe",
		AudioMIMEType: "audio/mpeg",
	})
	assert.NoError(t, err)
	// The model capitalizes the name as "Dive", "DIve", or "dive".
	assert.Contains(t, strings.ToLower(result.Text), "dive")
}
