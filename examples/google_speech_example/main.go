package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/media"
	"github.com/deepnoodle-ai/dive/providers/google"
)

func main() {
	var (
		text               string
		out                string
		voice              string
		format             string
		speechModel        string
		transcriptionModel string
		transcribe         bool
	)
	flag.StringVar(&text, "text", "Say cheerfully: Welcome to Dive speech generation.", "text to synthesize")
	flag.StringVar(&out, "out", "google-speech", "output path; extension is added if omitted")
	flag.StringVar(&voice, "voice", "Kore", "Gemini prebuilt voice name")
	flag.StringVar(&format, "format", "wav", "audio format: wav or pcm")
	flag.StringVar(&speechModel, "speech-model", google.ModelGemini31FlashTTSPreview, "Gemini TTS model")
	flag.StringVar(&transcriptionModel, "transcription-model", google.ModelGemini35Flash, "Gemini transcription model")
	flag.BoolVar(&transcribe, "transcribe", true, "transcribe the generated audio")
	flag.Parse()

	audioFormat := media.AudioFormat(format)
	if err := media.ValidateAudioFormat(audioFormat); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	speech, err := media.GenerateSpeech(ctx, text,
		media.WithModel(speechModel),
		media.WithVoice(voice),
		media.WithAudioFormat(audioFormat),
	)
	if err != nil {
		log.Fatal(err)
	}

	path, err := speech.WriteTo(out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Saved %s (%s, %s, %d bytes)\n", path, speech.Model, speech.Format, len(speech.Data))

	if !transcribe {
		return
	}

	transcript, err := media.TranscribeSpeech(ctx, speech.Data,
		media.WithModel(transcriptionModel),
		media.WithAudioMIMEType(speech.MimeType),
		media.WithTranscriptionPrompt("Generate a transcript of the speech."),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nTranscript:\n%s\n", transcript.Text)
}
