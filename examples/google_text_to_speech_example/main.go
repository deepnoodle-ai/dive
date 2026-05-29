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
	const defaultText = "Alice was beginning to get very tired of sitting by her sister on the bank, and of having nothing to do. " +
		"Once or twice she had peeped into the book her sister was reading, but it had no pictures or conversations in it. " +
		"What is the use of a book, thought Alice, without pictures or conversations?"

	var (
		text               string
		out                string
		voice              string
		format             string
		ttsModel           string
		transcriptionModel string
		transcribe         bool
	)
	flag.StringVar(&text, "text", defaultText, "text to synthesize")
	flag.StringVar(&out, "out", "google-text-to-speech", "output path; extension is added if omitted")
	flag.StringVar(&voice, "voice", "Kore", "Gemini prebuilt voice name")
	flag.StringVar(&format, "format", "wav", "audio format: wav or pcm")
	flag.StringVar(&ttsModel, "tts-model", google.ModelGemini31FlashTTSPreview, "Gemini text-to-speech model")
	flag.StringVar(&transcriptionModel, "transcription-model", google.ModelGemini35Flash, "Gemini transcription model")
	flag.BoolVar(&transcribe, "transcribe", true, "transcribe the generated audio")
	flag.Parse()

	audioFormat := media.AudioFormat(format)
	if err := media.ValidateAudioFormat(audioFormat); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	audio, err := media.TextToSpeech(ctx, text,
		media.WithModel(ttsModel),
		media.WithVoice(voice),
		media.WithAudioFormat(audioFormat),
	)
	if err != nil {
		log.Fatal(err)
	}

	path, err := audio.WriteTo(out)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Saved %s (%s, %s, %d bytes)\n", path, audio.Model, audio.Format, len(audio.Data))

	if !transcribe {
		return
	}

	transcript, err := media.Transcribe(ctx, audio.Data,
		media.WithModel(transcriptionModel),
		media.WithAudioMIMEType(audio.MimeType),
		media.WithTranscriptionPrompt("Generate a transcript of the speech."),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nTranscript:\n%s\n", transcript.Text)
}
