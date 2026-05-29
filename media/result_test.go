package media

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestImageResult_WriteTo(t *testing.T) {
	dir := t.TempDir()
	r := &ImageResult{
		Data:   []byte("fake-png-data"),
		Format: FormatPNG,
	}

	path := filepath.Join(dir, "test.png")
	actual, err := r.WriteTo(path)
	assert.NoError(t, err)
	assert.Equal(t, path, actual)

	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, []byte("fake-png-data"), data)
}

func TestImageResult_WriteTo_AutoExtension(t *testing.T) {
	dir := t.TempDir()
	r := &ImageResult{
		Data:   []byte("fake-jpeg-data"),
		Format: FormatJPEG,
	}

	path := filepath.Join(dir, "test")
	actual, err := r.WriteTo(path)
	assert.NoError(t, err)

	expected := filepath.Join(dir, "test.jpg")
	assert.Equal(t, expected, actual)
	_, err = os.Stat(expected)
	assert.NoError(t, err)
}

func TestVideoResult_WriteTo(t *testing.T) {
	dir := t.TempDir()
	r := &VideoResult{
		Data:   []byte("fake-mp4-data"),
		Format: "mp4",
	}

	path := filepath.Join(dir, "test.mp4")
	actual, err := r.WriteTo(path)
	assert.NoError(t, err)
	assert.Equal(t, path, actual)

	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, []byte("fake-mp4-data"), data)
}

func TestVideoResult_WriteTo_AutoExtension(t *testing.T) {
	dir := t.TempDir()
	r := &VideoResult{
		Data:   []byte("fake-webm-data"),
		Format: "webm",
	}

	path := filepath.Join(dir, "test")
	actual, err := r.WriteTo(path)
	assert.NoError(t, err)

	expected := filepath.Join(dir, "test.webm")
	assert.Equal(t, expected, actual)
	data, err := os.ReadFile(expected)
	assert.NoError(t, err)
	assert.Equal(t, []byte("fake-webm-data"), data)
}

func TestAudioResult_WriteTo(t *testing.T) {
	dir := t.TempDir()
	r := &AudioResult{
		Data:   []byte("fake-audio-data"),
		Format: AudioFormatMP3,
	}

	path := filepath.Join(dir, "test.mp3")
	actual, err := r.WriteTo(path)
	assert.NoError(t, err)
	assert.Equal(t, path, actual)

	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, []byte("fake-audio-data"), data)
}

func TestAudioResult_WriteTo_AutoExtension(t *testing.T) {
	dir := t.TempDir()
	r := &AudioResult{
		Data:   []byte("fake-wav-data"),
		Format: AudioFormatWAV,
	}

	path := filepath.Join(dir, "test")
	actual, err := r.WriteTo(path)
	assert.NoError(t, err)

	expected := filepath.Join(dir, "test.wav")
	assert.Equal(t, expected, actual)
	data, err := os.ReadFile(expected)
	assert.NoError(t, err)
	assert.Equal(t, []byte("fake-wav-data"), data)
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()

	// Non-existent path returns unchanged.
	path := filepath.Join(dir, "new.png")
	assert.Equal(t, path, UniquePath(path))

	// Create the file, then UniquePath should return path with suffix 1.
	err := os.WriteFile(path, []byte("x"), 0644)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "new1.png"), UniquePath(path))

	// Create that too, should get suffix 2.
	err = os.WriteFile(filepath.Join(dir, "new1.png"), []byte("x"), 0644)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "new2.png"), UniquePath(path))
}

func TestImageResult_WriteTo_NoOverwrite(t *testing.T) {
	dir := t.TempDir()
	r := &ImageResult{
		Data:   []byte("second"),
		Format: FormatPNG,
	}

	path := filepath.Join(dir, "img.png")
	os.WriteFile(path, []byte("first"), 0644)

	actual, err := r.WriteTo(path)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "img1.png"), actual)

	// Original file is untouched.
	data, _ := os.ReadFile(path)
	assert.Equal(t, []byte("first"), data)

	// New file has new data.
	data, _ = os.ReadFile(actual)
	assert.Equal(t, []byte("second"), data)
}

func TestVideoResult_SetVideoFormat(t *testing.T) {
	r := &VideoResult{}

	r.SetVideoFormat("video/mp4")
	assert.Equal(t, "mp4", r.Format)
	assert.Equal(t, "video/mp4", r.MimeType)

	r.SetVideoFormat("video/webm")
	assert.Equal(t, "webm", r.Format)
	assert.Equal(t, "video/webm", r.MimeType)

	r.SetVideoFormat("video/unknown")
	assert.Equal(t, "mp4", r.Format)
	assert.Equal(t, "video/mp4", r.MimeType)
}

func TestAudioResult_SetAudioFormat(t *testing.T) {
	r := &AudioResult{}

	r.SetAudioFormat("audio/wav")
	assert.Equal(t, AudioFormatWAV, r.Format)
	assert.Equal(t, "audio/wav", r.MimeType)

	r.SetAudioFormat("audio/mpeg")
	assert.Equal(t, AudioFormatMP3, r.Format)
	assert.Equal(t, "audio/mpeg", r.MimeType)
}

func TestSlugifyPrompt(t *testing.T) {
	tests := []struct {
		prompt string
		maxLen int
		expect string
	}{
		{"a cat in space", 50, "a-cat-in-space"},
		{"Hello, World! 123", 50, "hello-world-123"},
		{"  multiple   spaces  ", 50, "multiple-spaces"},
		{"", 50, "generated"},
		{"!@#$%^&*()", 50, "generated"},
		{"very long prompt that should be truncated", 10, "very-long"},
	}
	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			result := SlugifyPrompt(tt.prompt, tt.maxLen)
			assert.Equal(t, tt.expect, result)
		})
	}
}
