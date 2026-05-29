package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/deepnoodle-ai/dive/media"
	_ "github.com/deepnoodle-ai/dive/providers/google"
	_ "github.com/deepnoodle-ai/dive/providers/openai"
)

func main() {
	var (
		input    string
		mimeType string
		model    string
		language string
		prompt   string
	)
	flag.StringVar(&input, "input", "", "audio file to transcribe")
	flag.StringVar(&mimeType, "mime", "", "input audio MIME type; auto-detected if omitted")
	flag.StringVar(&model, "model", "", "transcription model; auto-detected from available API keys if omitted")
	flag.StringVar(&language, "language", "", "optional language hint, such as en")
	flag.StringVar(&prompt, "prompt", "", "optional transcription context prompt")
	flag.Parse()

	if input == "" {
		log.Fatal("set -input to an audio file path")
	}
	if model == "" {
		model = defaultTranscriptionModel()
	}
	if model == "" {
		log.Fatal("set -model, or configure OPENAI_API_KEY, GOOGLE_API_KEY, or GEMINI_API_KEY")
	}

	audio, err := os.ReadFile(input)
	if err != nil {
		log.Fatal(err)
	}
	if mimeType == "" {
		mimeType = media.DetectAudioMIMEFromBytes(audio)
	}

	result, err := media.Transcribe(context.Background(), audio,
		media.WithModel(model),
		media.WithAudioMIMEType(mimeType),
		media.WithLanguage(language),
		media.WithTranscriptionPrompt(prompt),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Model: %s\n", result.Model)
	if result.Language != "" {
		fmt.Printf("Language: %s\n", result.Language)
	}
	fmt.Printf("\n%s\n", result.Text)
}

func defaultTranscriptionModel() string {
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "gpt-4o-mini-transcribe"
	}
	if os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
		return "gemini-3.5-flash"
	}
	return ""
}
