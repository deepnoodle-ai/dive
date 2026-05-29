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
	// tts-1, tts-1-hd, gpt-4o-mini-tts
	media.RegisterSpeech(media.SpeechProviderEntry{
		Name:  "openai",
		Match: media.PrefixesMatcher("tts-", "gpt-4o-mini-tts"),
		Factory: func(model string) media.SpeechProvider {
			return NewMediaProvider()
		},
	})
	// whisper-1, gpt-4o-transcribe, gpt-4o-mini-transcribe, gpt-4o-transcribe-diarize
	media.RegisterSpeechRecognition(media.SpeechRecognitionProviderEntry{
		Name:  "openai",
		Match: media.PrefixesMatcher("whisper-", "gpt-4o-transcribe", "gpt-4o-mini-transcribe"),
		Factory: func(model string) media.SpeechRecognitionProvider {
			return NewMediaProvider()
		},
	})
}
