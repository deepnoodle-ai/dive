package media

import "time"

// Config holds the resolved options for a media generation call.
type Config struct {
	// Model is the model to use for generation.
	Model string

	// Models is used for fan-out generation across multiple models.
	Models []string

	// AspectRatio controls the output dimensions.
	AspectRatio AspectRatio

	// OutputFormat requests a specific image format (png, jpeg, webp).
	OutputFormat Format

	// AudioFormat requests a specific audio output format.
	AudioFormat AudioFormat

	// AudioMIMEType hints the MIME type of input audio bytes.
	AudioMIMEType string

	// Voice selects the voice for speech generation.
	Voice string

	// SpeechInstructions controls voice style for speech generation when supported.
	SpeechInstructions string

	// SpeechSpeed controls generated speech speed when supported.
	SpeechSpeed *float64

	// Language sets the transcription or speech language when supported.
	Language string

	// TranscriptionPrompt gives recognition models context for transcription.
	TranscriptionPrompt string

	// Count is the number of images to generate per model.
	Count int

	// ReferenceImages are input images for editing operations.
	ReferenceImages [][]byte

	// Duration is the target video duration.
	Duration time.Duration

	// Timeout is the maximum time to wait for generation.
	// Defaults to 5 minutes for images, 15 minutes for video.
	Timeout time.Duration
}

// Option configures a media generation call.
type Option func(*Config)

// Apply applies all options and sets defaults for unset fields.
func (c *Config) Apply(opts ...Option) {
	for _, opt := range opts {
		opt(c)
	}
	if c.Count < 1 {
		c.Count = 1
	}
}

// WithModel sets the model for generation.
func WithModel(model string) Option {
	return func(c *Config) {
		c.Model = model
	}
}

// WithModels sets multiple models for fan-out generation.
func WithModels(models ...string) Option {
	return func(c *Config) {
		c.Models = models
	}
}

// WithAspectRatio sets the aspect ratio.
func WithAspectRatio(ar AspectRatio) Option {
	return func(c *Config) {
		c.AspectRatio = ar
	}
}

// WithOutputFormat sets the desired output format.
func WithOutputFormat(f Format) Option {
	return func(c *Config) {
		c.OutputFormat = f
	}
}

// WithAudioFormat sets the desired audio output format.
func WithAudioFormat(f AudioFormat) Option {
	return func(c *Config) {
		c.AudioFormat = f
	}
}

// WithAudioMIMEType sets the MIME type for input audio bytes.
func WithAudioMIMEType(mimeType string) Option {
	return func(c *Config) {
		c.AudioMIMEType = mimeType
	}
}

// WithVoice sets the voice for speech generation.
func WithVoice(voice string) Option {
	return func(c *Config) {
		c.Voice = voice
	}
}

// WithSpeechInstructions sets style instructions for speech generation.
func WithSpeechInstructions(instructions string) Option {
	return func(c *Config) {
		c.SpeechInstructions = instructions
	}
}

// WithSpeechSpeed sets the generated speech speed when supported.
func WithSpeechSpeed(speed float64) Option {
	return func(c *Config) {
		c.SpeechSpeed = &speed
	}
}

// WithLanguage sets the language for transcription or speech generation.
func WithLanguage(language string) Option {
	return func(c *Config) {
		c.Language = language
	}
}

// WithTranscriptionPrompt gives recognition models context for transcription.
func WithTranscriptionPrompt(prompt string) Option {
	return func(c *Config) {
		c.TranscriptionPrompt = prompt
	}
}

// WithCount sets the number of images to generate.
func WithCount(n int) Option {
	return func(c *Config) {
		c.Count = n
	}
}

// WithReferenceImage adds a reference image for editing operations.
func WithReferenceImage(data []byte) Option {
	return func(c *Config) {
		c.ReferenceImages = append(c.ReferenceImages, data)
	}
}

// WithDuration sets the target video duration.
func WithDuration(d time.Duration) Option {
	return func(c *Config) {
		c.Duration = d
	}
}

// WithTimeout sets the maximum time to wait for generation.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.Timeout = d
	}
}
