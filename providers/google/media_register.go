package google

import "github.com/deepnoodle-ai/dive/media"

func init() {
	media.RegisterImage(media.ImageProviderEntry{
		Name:  "google",
		Match: media.PrefixesMatcher("gemini-", "imagen-"),
		Factory: func(model string) media.ImageProvider {
			return NewMediaProvider()
		},
	})
	media.RegisterVideo(media.VideoProviderEntry{
		Name:  "google",
		Match: media.PrefixMatcher("veo-"),
		Factory: func(model string) media.VideoProvider {
			return NewMediaProvider()
		},
	})
}
