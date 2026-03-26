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
