package media

import "context"

// ImageProvider generates images from text prompts.
type ImageProvider interface {
	// GenerateImage generates one or more images from a prompt.
	// The number of images is controlled by config.Count.
	GenerateImage(ctx context.Context, prompt string, config *Config) ([]*ImageResult, error)
}

// ImageEditor edits images using a text prompt and reference images.
// Providers that support editing implement this in addition to ImageProvider.
type ImageEditor interface {
	// EditImage edits reference images according to the prompt.
	// Reference images are passed via config.ReferenceImages.
	EditImage(ctx context.Context, prompt string, config *Config) ([]*ImageResult, error)
}

// VideoProvider generates videos from text prompts.
type VideoProvider interface {
	// GenerateVideo generates a video from a prompt.
	// The call blocks until generation is complete or ctx is cancelled.
	GenerateVideo(ctx context.Context, prompt string, config *Config) (*VideoResult, error)
}

// SpeechProvider generates spoken audio from text.
type SpeechProvider interface {
	// GenerateSpeech generates audio from text.
	GenerateSpeech(ctx context.Context, text string, config *Config) (*AudioResult, error)
}

// SpeechRecognitionProvider transcribes speech audio into text.
type SpeechRecognitionProvider interface {
	// TranscribeSpeech transcribes audio bytes into text.
	TranscribeSpeech(ctx context.Context, audio []byte, config *Config) (*TranscriptionResult, error)
}
