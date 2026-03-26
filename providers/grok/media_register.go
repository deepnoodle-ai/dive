package grok

import "github.com/deepnoodle-ai/dive/media"

func init() {
	// grok-imagine-image-pro, grok-imagine-image
	media.RegisterImage(media.ImageProviderEntry{
		Name:  "grok",
		Match: media.PrefixMatcher("grok-imagine-image"),
		Factory: func(model string) media.ImageProvider {
			return NewMediaProvider()
		},
	})
	// grok-imagine-video
	media.RegisterVideo(media.VideoProviderEntry{
		Name:  "grok",
		Match: media.PrefixMatcher("grok-imagine-video"),
		Factory: func(model string) media.VideoProvider {
			return NewMediaProvider()
		},
	})
}
