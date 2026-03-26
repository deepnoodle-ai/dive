// Package media provides unified multi-provider image and video generation.
package media

// AspectRatio represents the aspect ratio for generated media.
type AspectRatio string

const (
	// AspectAuto lets the provider choose a default aspect ratio.
	AspectAuto AspectRatio = ""

	Aspect1x1  AspectRatio = "1:1"
	Aspect16x9 AspectRatio = "16:9"
	Aspect9x16 AspectRatio = "9:16"
	Aspect4x3  AspectRatio = "4:3"
	Aspect3x4  AspectRatio = "3:4"
	Aspect4x1  AspectRatio = "4:1"
	Aspect1x4  AspectRatio = "1:4"
)

// String returns the string representation of the aspect ratio.
func (ar AspectRatio) String() string {
	return string(ar)
}

// StandardImageDimensions returns default pixel dimensions for an aspect ratio.
// Returns (1024, 1024) for unrecognized ratios.
func StandardImageDimensions(ar AspectRatio) (width, height int) {
	switch ar {
	case Aspect1x1:
		return 1024, 1024
	case Aspect4x3:
		return 1024, 768
	case Aspect3x4:
		return 768, 1024
	case Aspect16x9:
		return 1344, 768
	case Aspect9x16:
		return 768, 1344
	case Aspect4x1:
		return 2048, 512
	case Aspect1x4:
		return 512, 2048
	default:
		return 1024, 1024
	}
}

// StandardVideoDimensions returns default pixel dimensions for video at
// the given aspect ratio. Returns (1920, 1080) for unrecognized ratios.
func StandardVideoDimensions(ar AspectRatio) (width, height int) {
	switch ar {
	case Aspect16x9:
		return 1920, 1080
	case Aspect9x16:
		return 1080, 1920
	case Aspect1x1:
		return 1080, 1080
	default:
		return 1920, 1080
	}
}
