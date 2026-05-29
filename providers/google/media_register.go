package google

import (
	"strings"

	"github.com/deepnoodle-ai/dive/media"
)

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
	media.RegisterSpeech(media.SpeechProviderEntry{
		Name: "google",
		Match: func(model string) bool {
			lower := strings.ToLower(model)
			return strings.HasPrefix(lower, "gemini-") && strings.Contains(lower, "tts")
		},
		Factory: func(model string) media.SpeechProvider {
			return NewMediaProvider()
		},
	})
	media.RegisterSpeechRecognition(media.SpeechRecognitionProviderEntry{
		Name:  "google",
		Match: media.PrefixMatcher("gemini-"),
		Factory: func(model string) media.SpeechRecognitionProvider {
			return NewMediaProvider()
		},
	})
}
