package media

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestStandardImageDimensions(t *testing.T) {
	tests := []struct {
		ar     AspectRatio
		width  int
		height int
	}{
		{Aspect1x1, 1024, 1024},
		{Aspect16x9, 1344, 768},
		{Aspect9x16, 768, 1344},
		{Aspect4x3, 1024, 768},
		{Aspect3x4, 768, 1024},
		{Aspect4x1, 2048, 512},
		{Aspect1x4, 512, 2048},
		{AspectAuto, 1024, 1024},         // default
		{AspectRatio("7:3"), 1024, 1024}, // unknown
	}
	for _, tt := range tests {
		t.Run(string(tt.ar), func(t *testing.T) {
			w, h := StandardImageDimensions(tt.ar)
			assert.Equal(t, tt.width, w)
			assert.Equal(t, tt.height, h)
		})
	}
}

func TestStandardVideoDimensions(t *testing.T) {
	tests := []struct {
		ar     AspectRatio
		width  int
		height int
	}{
		{Aspect16x9, 1920, 1080},
		{Aspect9x16, 1080, 1920},
		{Aspect1x1, 1080, 1080},
		{AspectAuto, 1920, 1080}, // default
	}
	for _, tt := range tests {
		t.Run(string(tt.ar), func(t *testing.T) {
			w, h := StandardVideoDimensions(tt.ar)
			assert.Equal(t, tt.width, w)
			assert.Equal(t, tt.height, h)
		})
	}
}
