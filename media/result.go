package media

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImageResult is the output of an image generation or edit operation.
type ImageResult struct {
	// Data is the raw image bytes.
	Data []byte

	// Model is the model that generated this image.
	Model string

	// Format is the detected image format (png, jpeg, webp).
	Format Format

	// MimeType is the MIME type of the image.
	MimeType string

	// Width is the image width in pixels.
	Width int

	// Height is the image height in pixels.
	Height int

	// Metadata contains provider-specific metadata.
	Metadata map[string]any

	// Err is non-nil if this result represents a provider failure
	// during fan-out generation. Other fields may be empty.
	Err error
}

// WriteTo writes the image data to the given file path.
// If the path has no extension, the format's extension is appended.
// If the path already exists, a numeric suffix is appended to avoid
// overwriting (e.g., "photo1.png", "photo2.png").
func (r *ImageResult) WriteTo(path string) (string, error) {
	if filepath.Ext(path) == "" {
		path += r.Format.FileExtension()
	}
	path = UniquePath(path)
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
	}
	return path, os.WriteFile(path, r.Data, 0644)
}

// VideoResult is the output of a video generation operation.
type VideoResult struct {
	// Data is the raw video bytes.
	Data []byte

	// Model is the model that generated this video.
	Model string

	// Format is the video container format (mp4, webm).
	Format string

	// MimeType is the MIME type of the video.
	MimeType string

	// Width is the video width in pixels.
	Width int

	// Height is the video height in pixels.
	Height int

	// Duration is the video duration.
	Duration time.Duration

	// AspectRatio is the video aspect ratio.
	AspectRatio AspectRatio

	// Metadata contains provider-specific metadata.
	Metadata map[string]any
}

// AudioResult is the output of a speech generation operation.
type AudioResult struct {
	// Data is the raw audio bytes.
	Data []byte

	// Model is the model that generated this audio.
	Model string

	// Format is the audio container or codec format.
	Format AudioFormat

	// MimeType is the MIME type of the audio.
	MimeType string

	// Duration is the audio duration when known.
	Duration time.Duration

	// Metadata contains provider-specific metadata.
	Metadata map[string]any
}

// WriteTo writes the audio data to the given file path.
// If the path has no extension, the format's extension is appended.
// If the path already exists, a numeric suffix is appended to avoid overwriting.
func (r *AudioResult) WriteTo(path string) (string, error) {
	if filepath.Ext(path) == "" {
		path += r.Format.FileExtension()
	}
	path = UniquePath(path)
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
	}
	return path, os.WriteFile(path, r.Data, 0644)
}

// SetAudioFormat sets format fields from a MIME type.
func (r *AudioResult) SetAudioFormat(mimeType string) {
	r.MimeType = mimeType
	r.Format = AudioFormatFromMIME(mimeType)
}

// TranscriptionResult is the output of a speech recognition operation.
type TranscriptionResult struct {
	// Text is the recognized transcript.
	Text string

	// Model is the model that transcribed the audio.
	Model string

	// Language is the recognized or requested language when known.
	Language string

	// Duration is the input audio duration when known.
	Duration time.Duration

	// Metadata contains provider-specific metadata.
	Metadata map[string]any
}

// WriteTo writes the video data to the given file path.
// If the path has no extension, ".mp4" is appended.
// If the path already exists, a numeric suffix is appended to avoid
// overwriting (e.g., "clip1.mp4", "clip2.mp4").
func (r *VideoResult) WriteTo(path string) (string, error) {
	if filepath.Ext(path) == "" {
		ext := ".mp4"
		if r.Format == "webm" {
			ext = ".webm"
		}
		path += ext
	}
	path = UniquePath(path)
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
	}
	return path, os.WriteFile(path, r.Data, 0644)
}

// SetVideoFormat sets format fields from a MIME type.
func (r *VideoResult) SetVideoFormat(mimeType string) {
	r.MimeType = mimeType
	switch mimeType {
	case "video/mp4":
		r.Format = "mp4"
	case "video/webm":
		r.Format = "webm"
	default:
		r.Format = "mp4"
		r.MimeType = "video/mp4"
	}
}

// UniquePath returns path unchanged if it does not exist. If it does exist,
// a numeric suffix is inserted before the extension (e.g., "photo1.png",
// "photo2.png") until an available name is found.
func UniquePath(path string) string {
	if _, err := os.Stat(path); err != nil {
		return path // doesn't exist or can't stat — use as-is
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	const maxAttempts = 10000
	for i := 1; i <= maxAttempts; i++ {
		candidate := fmt.Sprintf("%s%d%s", base, i, ext)
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
	// Fallback: use timestamp suffix
	return fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext)
}

// SlugifyPrompt generates a short filename-safe slug from a prompt string.
func SlugifyPrompt(prompt string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 50
	}
	s := strings.ToLower(prompt)
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			if b.Len() > 0 {
				last := b.String()
				if last[len(last)-1] != '-' {
					b.WriteByte('-')
				}
			}
		}
		if b.Len() >= maxLen {
			break
		}
	}
	result := strings.TrimRight(b.String(), "-")
	if result == "" {
		result = "generated"
	}
	return result
}
