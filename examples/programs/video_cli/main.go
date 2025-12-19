// video_cli is a standalone program for generating videos.
//
// This example demonstrates basic video generation using Google's Veo models.
// For the full implementation from cmd/dive/cli/video.go with status polling,
// see the dive repository.
//
// Usage:
//
//	go run ./examples/programs/video_cli --prompt "A cat playing piano"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm/providers/google"
)

func main() {
	prompt := flag.String("prompt", "", "Video generation prompt")
	model := flag.String("model", "veo-002", "Model to use (veo-001 or veo-002)")
	duration := flag.Int("duration", 5, "Video duration in seconds (default: 5)")
	flag.Parse()

	if *prompt == "" {
		log.Fatal("--prompt is required")
	}

	if err := generateVideo(*prompt, *model, *duration); err != nil {
		log.Fatal(err)
	}
}

func generateVideo(prompt, model string, duration int) error {
	ctx := context.Background()

	provider := google.New()
	videoProvider, ok := provider.(google.VideoProvider)
	if !ok {
		return fmt.Errorf("provider does not support video generation")
	}

	fmt.Printf("Generating video with prompt: %q\n", prompt)
	fmt.Printf("Duration: %d seconds\n", duration)
	fmt.Printf("Model: %s\n", model)
	fmt.Println("\nNote: Video generation is asynchronous and may take several minutes.")
	fmt.Println("The operation will return an ID that you can use to check status.")

	response, err := videoProvider.GenerateVideo(ctx, google.VideoGenerationRequest{
		Prompt:   prompt,
		Model:    model,
		Duration: duration,
	})
	if err != nil {
		return fmt.Errorf("error generating video: %v", err)
	}

	fmt.Printf("\nVideo generation started!\n")
	fmt.Printf("Operation ID: %s\n", response.OperationID)
	fmt.Printf("\nUse this ID to check the status of your video generation.\n")

	return nil
}
