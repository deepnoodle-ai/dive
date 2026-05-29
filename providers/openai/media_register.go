package openai

import "github.com/deepnoodle-ai/dive/media"

func init() {
	// gpt-image-2, gpt-image-1.5, gpt-image-1, gpt-image-1-mini
	media.RegisterImage(media.ImageProviderEntry{
		Name:  "openai",
		Match: media.PrefixMatcher("gpt-image-"),
		Factory: func(model string) media.ImageProvider {
			return NewMediaProvider()
		},
	})
	// sora-2, sora-2-pro
	media.RegisterVideo(media.VideoProviderEntry{
		Name:  "openai",
		Match: media.PrefixMatcher("sora-"),
		Factory: func(model string) media.VideoProvider {
			return NewMediaProvider()
		},
	})
}
