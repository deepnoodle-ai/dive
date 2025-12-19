// image_cli is a standalone program for generating images.
//
// This example demonstrates basic image generation using OpenAI's DALL-E.
// For the full implementation with editing capabilities from cmd/dive/cli/image.go,
// see the dive repository.
//
// Usage:
//
//	go run ./examples/programs/image_cli --prompt "A sunset over mountains" --output image.png
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/deepnoodle-ai/dive/llm/providers/openai"
)

func main() {
	prompt := flag.String("prompt", "", "Image generation prompt")
	output := flag.String("output", "output.png", "Output file path")
	size := flag.String("size", "1024x1024", "Image size (e.g., 1024x1024)")
	model := flag.String("model", "dall-e-3", "Model to use (dall-e-2 or dall-e-3)")
	flag.Parse()

	if *prompt == "" {
		log.Fatal("--prompt is required")
	}

	if err := generateImage(*prompt, *output, *size, *model); err != nil {
		log.Fatal(err)
	}
}

func generateImage(prompt, output, size, model string) error {
	ctx := context.Background()

	provider := openai.New()
	imageProvider, ok := provider.(openai.ImageProvider)
	if !ok {
		return fmt.Errorf("provider does not support image generation")
	}

	fmt.Printf("Generating image with prompt: %q\n", prompt)

	response, err := imageProvider.GenerateImage(ctx, openai.ImageGenerationRequest{
		Prompt: prompt,
		Size:   size,
		Model:  model,
		N:      1,
	})
	if err != nil {
		return fmt.Errorf("error generating image: %v", err)
	}

	if len(response.Data) == 0 {
		return fmt.Errorf("no image data returned")
	}

	imageData := response.Data[0]
	if imageData.B64JSON != "" {
		// Save base64 image
		if err := os.WriteFile(output, []byte(imageData.B64JSON), 0644); err != nil {
			return fmt.Errorf("error writing image: %v", err)
		}
	} else if imageData.URL != "" {
		fmt.Printf("Image URL: %s\n", imageData.URL)
		return nil
	}

	fmt.Printf("Image saved to: %s\n", output)
	return nil
}
