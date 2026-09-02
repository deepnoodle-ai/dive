package meta

import "github.com/deepnoodle-ai/dive/media"

func init() {
	media.RegisterImage(media.ImageProviderEntry{
		Name:  "meta",
		Match: media.PrefixMatcher("muse-image"),
		Factory: func(model string) media.ImageProvider {
			return NewMediaProvider()
		},
	})
	media.RegisterTranscription(media.TranscriptionProviderEntry{
		Name:  "meta",
		Match: media.PrefixMatcher("muse-voice-transcribe"),
		Factory: func(model string) media.TranscriptionProvider {
			return NewMediaProvider()
		},
	})
}
