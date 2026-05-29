package media

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

var registryMu sync.Mutex

// testRegistry replaces the default registry for the duration of a test.
// Not safe with t.Parallel() — tests using this must run sequentially.
func testRegistry(t *testing.T) *Registry {
	t.Helper()
	registryMu.Lock()
	old := defaultRegistry
	defaultRegistry = &Registry{}
	t.Cleanup(func() {
		defaultRegistry = old
		registryMu.Unlock()
	})
	return defaultRegistry
}

func TestGenerateImage(t *testing.T) {
	r := testRegistry(t)
	r.RegisterImage(ImageProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) ImageProvider {
			return &mockImageProvider{
				result: []*ImageResult{{
					Data:     []byte("image-data"),
					Model:    model,
					Format:   FormatPNG,
					MimeType: "image/png",
					Width:    1024,
					Height:   1024,
				}},
			}
		},
	})

	result, err := GenerateImage(context.Background(), "a cat",
		WithModel("test-model"),
	)
	assert.NoError(t, err)
	assert.Equal(t, []byte("image-data"), result.Data)
	assert.Equal(t, "test-model", result.Model)
	assert.Equal(t, 1024, result.Width)
}

func TestGenerateImage_NoModel(t *testing.T) {
	testRegistry(t)
	_, err := GenerateImage(context.Background(), "a cat")
	assert.Equal(t, ErrNoModel, err)
}

func TestGenerateImage_ProviderNotFound(t *testing.T) {
	testRegistry(t)
	_, err := GenerateImage(context.Background(), "a cat", WithModel("unknown"))
	assert.True(t, errors.Is(err, ErrProviderNotFound))
}

func TestGenerateImage_ProviderError(t *testing.T) {
	r := testRegistry(t)
	r.RegisterImage(ImageProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) ImageProvider {
			return &mockImageProvider{err: errors.New("provider failed")}
		},
	})

	_, err := GenerateImage(context.Background(), "a cat", WithModel("test-model"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider failed")
}

func TestGenerateImage_EmptyResult(t *testing.T) {
	r := testRegistry(t)
	r.RegisterImage(ImageProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) ImageProvider {
			return &mockImageProvider{result: []*ImageResult{}}
		},
	})

	_, err := GenerateImage(context.Background(), "a cat", WithModel("test-model"))
	assert.Equal(t, ErrNoResult, err)
}

func TestGenerateImages_FanOut(t *testing.T) {
	r := testRegistry(t)
	r.RegisterImage(ImageProviderEntry{
		Name:  "alpha",
		Match: PrefixMatcher("alpha-"),
		Factory: func(model string) ImageProvider {
			return &mockImageProvider{
				result: []*ImageResult{{Data: []byte("alpha"), Model: model, Format: FormatPNG}},
			}
		},
	})
	r.RegisterImage(ImageProviderEntry{
		Name:  "beta",
		Match: PrefixMatcher("beta-"),
		Factory: func(model string) ImageProvider {
			return &mockImageProvider{
				result: []*ImageResult{{Data: []byte("beta"), Model: model, Format: FormatPNG}},
			}
		},
	})

	results, err := GenerateImages(context.Background(), "a cat",
		WithModels("alpha-1", "beta-1"),
	)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "alpha-1", results[0].Model)
	assert.Equal(t, "beta-1", results[1].Model)
	assert.Nil(t, results[0].Err)
	assert.Nil(t, results[1].Err)
}

func TestGenerateImages_PartialFailure(t *testing.T) {
	r := testRegistry(t)
	r.RegisterImage(ImageProviderEntry{
		Name:  "good",
		Match: PrefixMatcher("good-"),
		Factory: func(model string) ImageProvider {
			return &mockImageProvider{
				result: []*ImageResult{{Data: []byte("ok"), Model: model, Format: FormatPNG}},
			}
		},
	})
	r.RegisterImage(ImageProviderEntry{
		Name:  "bad",
		Match: PrefixMatcher("bad-"),
		Factory: func(model string) ImageProvider {
			return &mockImageProvider{err: errors.New("boom")}
		},
	})

	results, err := GenerateImages(context.Background(), "a cat",
		WithModels("good-1", "bad-1"),
	)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Nil(t, results[0].Err)
	assert.NotNil(t, results[1].Err)
	assert.Contains(t, results[1].Err.Error(), "boom")
}

func TestGenerateImages_NoModels(t *testing.T) {
	testRegistry(t)
	_, err := GenerateImages(context.Background(), "a cat")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "WithModels is required")
}

func TestEditImage(t *testing.T) {
	r := testRegistry(t)
	r.RegisterImage(ImageProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) ImageProvider {
			return &mockImageEditor{
				editResult: []*ImageResult{{Data: []byte("edited"), Model: model, Format: FormatPNG}},
			}
		},
	})

	result, err := EditImage(context.Background(), "make it blue",
		WithModel("test-model"),
		WithReferenceImage([]byte{1, 2, 3}),
	)
	assert.NoError(t, err)
	assert.Equal(t, []byte("edited"), result.Data)
}

func TestEditImage_NotSupported(t *testing.T) {
	r := testRegistry(t)
	r.RegisterImage(ImageProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) ImageProvider {
			return &mockImageProvider{} // does not implement ImageEditor
		},
	})

	_, err := EditImage(context.Background(), "make it blue",
		WithModel("test-model"),
		WithReferenceImage([]byte{1, 2, 3}),
	)
	assert.Equal(t, ErrEditNotSupported, err)
}

func TestEditImage_NoReferenceImage(t *testing.T) {
	testRegistry(t)
	_, err := EditImage(context.Background(), "make it blue",
		WithModel("test-model"),
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "WithReferenceImage is required")
}

func TestGenerateVideo(t *testing.T) {
	r := testRegistry(t)
	r.RegisterVideo(VideoProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) VideoProvider {
			return &mockVideoProvider{
				result: &VideoResult{
					Data:     []byte("video-data"),
					Model:    model,
					Format:   "mp4",
					MimeType: "video/mp4",
					Duration: 8 * time.Second,
				},
			}
		},
	})

	result, err := GenerateVideo(context.Background(), "a sunset",
		WithModel("test-veo"),
		WithDuration(8*time.Second),
	)
	assert.NoError(t, err)
	assert.Equal(t, []byte("video-data"), result.Data)
	assert.Equal(t, "test-veo", result.Model)
}

func TestGenerateSpeech(t *testing.T) {
	r := testRegistry(t)
	r.RegisterSpeech(SpeechProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) SpeechProvider {
			return &mockSpeechProvider{
				result: &AudioResult{
					Data:     []byte("audio-data"),
					Model:    model,
					Format:   AudioFormatMP3,
					MimeType: "audio/mpeg",
				},
			}
		},
	})

	result, err := GenerateSpeech(context.Background(), "hello",
		WithModel("test-tts"),
		WithVoice("alloy"),
	)
	assert.NoError(t, err)
	assert.Equal(t, []byte("audio-data"), result.Data)
	assert.Equal(t, "test-tts", result.Model)
}

func TestGenerateSpeech_NoModel(t *testing.T) {
	testRegistry(t)
	_, err := GenerateSpeech(context.Background(), "hello")
	assert.Equal(t, ErrNoModel, err)
}

func TestGenerateSpeech_EmptyResult(t *testing.T) {
	r := testRegistry(t)
	r.RegisterSpeech(SpeechProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) SpeechProvider {
			return &mockSpeechProvider{result: &AudioResult{}}
		},
	})

	_, err := GenerateSpeech(context.Background(), "hello", WithModel("test-tts"))
	assert.Equal(t, ErrNoResult, err)
}

func TestTranscribeSpeech(t *testing.T) {
	r := testRegistry(t)
	r.RegisterSpeechRecognition(SpeechRecognitionProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) SpeechRecognitionProvider {
			return &mockSpeechRecognitionProvider{
				result: &TranscriptionResult{
					Text:  "hello world",
					Model: model,
				},
			}
		},
	})

	result, err := TranscribeSpeech(context.Background(), []byte("audio"),
		WithModel("test-transcribe"),
		WithAudioMIMEType("audio/wav"),
	)
	assert.NoError(t, err)
	assert.Equal(t, "hello world", result.Text)
	assert.Equal(t, "test-transcribe", result.Model)
}

func TestTranscribeSpeech_NoModel(t *testing.T) {
	testRegistry(t)
	_, err := TranscribeSpeech(context.Background(), []byte("audio"))
	assert.Equal(t, ErrNoModel, err)
}

func TestTranscribeSpeech_EmptyResult(t *testing.T) {
	r := testRegistry(t)
	r.RegisterSpeechRecognition(SpeechRecognitionProviderEntry{
		Name:  "test",
		Match: PrefixMatcher("test-"),
		Factory: func(model string) SpeechRecognitionProvider {
			return &mockSpeechRecognitionProvider{result: &TranscriptionResult{}}
		},
	})

	_, err := TranscribeSpeech(context.Background(), []byte("audio"), WithModel("test-transcribe"))
	assert.Equal(t, ErrNoResult, err)
}

func TestGenerateVideo_NoModel(t *testing.T) {
	testRegistry(t)
	_, err := GenerateVideo(context.Background(), "a sunset")
	assert.Equal(t, ErrNoModel, err)
}

func TestGenerateImage_ContextCancelled(t *testing.T) {
	r := testRegistry(t)
	r.RegisterImage(ImageProviderEntry{
		Name:  "slow",
		Match: PrefixMatcher("slow-"),
		Factory: func(model string) ImageProvider {
			return &slowImageProvider{}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := GenerateImage(ctx, "a cat",
		WithModel("slow-model"),
		WithTimeout(1*time.Millisecond),
	)
	assert.Error(t, err)
}

// slowImageProvider blocks until context is cancelled.
type slowImageProvider struct{}

func (s *slowImageProvider) GenerateImage(ctx context.Context, _ string, _ *Config) ([]*ImageResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
