package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/openai"
)

func main() {
	provider := openai.New(
		openai.WithModel("gpt-4.1"),
	)

	ctx := context.Background()

	// Create output directory for generated images
	outputDir := "generated_images"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Example 1: Basic image generation
	fmt.Println("=== Example 1: Basic image generation ===")
	basicImageTool := openai.NewImageGenerationTool(openai.ImageGenerationToolOptions{})

	response1, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Generate an image of a majestic mountain landscape at sunset"),
		llm.WithTools(basicImageTool),
	)
	if err != nil {
		log.Printf("Error in example 1: %v", err)
	} else {
		fmt.Printf("Response: %s\n", response1.Message().Text())
		if err := saveImageFromResponse(*response1, filepath.Join(outputDir, "mountain_sunset.png")); err != nil {
			log.Printf("Failed to save image: %v", err)
		} else {
			fmt.Println("✓ Image saved as mountain_sunset.png")
		}
	}
	fmt.Println()

	// Example 2: High-quality image with specific dimensions
	fmt.Println("=== Example 2: High-quality image with custom options ===")
	hqImageTool := openai.NewImageGenerationTool(openai.ImageGenerationToolOptions{
		Size:       "1024x1536", // Portrait orientation
		Quality:    "high",
		Background: "auto",
	})

	response2, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Draw a detailed portrait of a wise old wizard with a long white beard, wearing a starry blue robe"),
		llm.WithTools(hqImageTool),
	)
	if err != nil {
		log.Printf("Error in example 2: %v", err)
	} else {
		fmt.Printf("Response: %s\n", response2.Message().Text())
		if err := saveImageFromResponse(*response2, filepath.Join(outputDir, "wizard_portrait.png")); err != nil {
			log.Printf("Failed to save image: %v", err)
		} else {
			fmt.Println("✓ High-quality image saved as wizard_portrait.png")
		}
	}
	fmt.Println()

	// Example 3: Square image with transparent background
	fmt.Println("=== Example 3: Square image with transparent background ===")
	squareImageTool := openai.NewImageGenerationTool(openai.ImageGenerationToolOptions{
		Size:       "1024x1024",
		Quality:    "high",
		Background: "transparent",
	})

	response3, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Create a cute cartoon cat sitting with a friendly expression, suitable for a logo"),
		llm.WithTools(squareImageTool),
	)
	if err != nil {
		log.Printf("Error in example 3: %v", err)
	} else {
		fmt.Printf("Response: %s\n", response3.Message().Text())
		if err := saveImageFromResponse(*response3, filepath.Join(outputDir, "cat_logo.png")); err != nil {
			log.Printf("Failed to save image: %v", err)
		} else {
			fmt.Println("✓ Square transparent image saved as cat_logo.png")
		}
	}
	fmt.Println()

	// Example 4: Multi-turn editing - Generate then modify
	fmt.Println("=== Example 4: Multi-turn editing ===")
	editImageTool := openai.NewImageGenerationTool(openai.ImageGenerationToolOptions{
		Size:    "1024x1024",
		Quality: "high",
	})

	// First generation
	fmt.Println("Step 1: Generate initial image...")
	initialResponse, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Generate an image of a simple wooden cabin in a forest clearing"),
		llm.WithTools(editImageTool),
	)
	if err != nil {
		log.Printf("Error in initial generation: %v", err)
	} else {
		fmt.Printf("Initial response: %s\n", initialResponse.Message().Text())
		if err := saveImageFromResponse(*initialResponse, filepath.Join(outputDir, "cabin_initial.png")); err != nil {
			log.Printf("Failed to save initial image: %v", err)
		} else {
			fmt.Println("✓ Initial cabin image saved as cabin_initial.png")
		}

		// Multi-turn edit using previous response ID
		fmt.Println("Step 2: Edit the image...")
		editResponse, err := provider.Generate(ctx,
			llm.WithUserTextMessage("Now add snow falling and make it a winter scene with warm light glowing from the cabin windows"),
			llm.WithTools(editImageTool),
			llm.WithProviderOption("openai-responses:previous_response_id", initialResponse.ID),
		)
		if err != nil {
			log.Printf("Error in editing: %v", err)
		} else {
			fmt.Printf("Edit response: %s\n", editResponse.Message().Text())
			if err := saveImageFromResponse(*editResponse, filepath.Join(outputDir, "cabin_winter.png")); err != nil {
				log.Printf("Failed to save edited image: %v", err)
			} else {
				fmt.Println("✓ Winter cabin image saved as cabin_winter.png")
			}
		}
	}
	fmt.Println()

	// Example 5: Forced image generation with tool_choice
	fmt.Println("=== Example 5: Forced image generation ===")
	forcedImageTool := openai.NewImageGenerationTool(openai.ImageGenerationToolOptions{
		Size:    "1024x1024",
		Quality: "medium",
	})

	response5, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Create an abstract geometric pattern with vibrant colors"),
		llm.WithTools(forcedImageTool),
		llm.WithToolChoice(llm.ToolChoiceAuto), // Force tool usage
	)
	if err != nil {
		log.Printf("Error in example 5: %v", err)
	} else {
		fmt.Printf("Response: %s\n", response5.Message().Text())
		if err := saveImageFromResponse(*response5, filepath.Join(outputDir, "abstract_pattern.png")); err != nil {
			log.Printf("Failed to save image: %v", err)
		} else {
			fmt.Println("✓ Abstract pattern saved as abstract_pattern.png")
		}
	}
	fmt.Println()

	// Example 6: Streaming image generation (simplified)
	fmt.Println("=== Example 6: Streaming image generation ===")
	partialImagesCount := 2
	streamImageTool := openai.NewImageGenerationTool(openai.ImageGenerationToolOptions{
		Size:          "1024x1024",
		Quality:       "high",
		PartialImages: &partialImagesCount, // Get 2 partial images during generation
	})

	stream, err := provider.Stream(ctx,
		llm.WithUserTextMessage("Draw a beautiful Japanese garden with a koi pond, cherry blossoms, and a traditional bridge"),
		llm.WithTools(streamImageTool),
	)
	if err != nil {
		log.Printf("Error starting stream: %v", err)
	} else {
		fmt.Println("Streaming image generation...")

		for stream.Next() {
			event := stream.Event()
			switch event.Type {
			case llm.EventTypeContentBlockDelta:
				if event.Delta != nil && event.Delta.Text != "" {
					fmt.Print(event.Delta.Text)
				}
			case llm.EventTypeContentBlockStart:
				if event.ContentBlock != nil && event.ContentBlock.Type == llm.ContentTypeToolUse {
					fmt.Printf("\n🎨 Image generation tool called: %s\n", event.ContentBlock.Name)
				}
			}
		}

		if err := stream.Err(); err != nil {
			log.Printf("Stream error: %v", err)
		} else {
			fmt.Println("\n✓ Streaming completed")
		}
		stream.Close()
	}
	fmt.Println()

	// Example 7: Multiple images in sequence
	fmt.Println("=== Example 7: Generate a series of related images ===")
	seriesImageTool := openai.NewImageGenerationTool(openai.ImageGenerationToolOptions{
		Size:    "1024x1024",
		Quality: "medium",
	})

	themes := []string{
		"A sunrise over a calm lake with mist",
		"The same lake at midday with bright sunlight",
		"The lake at sunset with golden reflections",
		"The lake at night with moonlight and stars",
	}

	for i, theme := range themes {
		fmt.Printf("Generating image %d/4: %s\n", i+1, theme)
		response, err := provider.Generate(ctx,
			llm.WithUserTextMessage(fmt.Sprintf("Generate an image: %s", theme)),
			llm.WithTools(seriesImageTool),
		)
		if err != nil {
			log.Printf("Error generating image %d: %v", i+1, err)
			continue
		}

		filename := fmt.Sprintf("lake_series_%d.png", i+1)
		if err := saveImageFromResponse(*response, filepath.Join(outputDir, filename)); err != nil {
			log.Printf("Failed to save image %d: %v", i+1, err)
		} else {
			fmt.Printf("✓ Saved as %s\n", filename)
		}

		// Small delay between requests
		time.Sleep(time.Second)
	}
	fmt.Println()

	fmt.Printf("=== All examples completed! ===\n")
	fmt.Printf("Generated images are saved in the '%s' directory.\n", outputDir)
	fmt.Println("\nImage generation features demonstrated:")
	fmt.Println("• Basic image generation with text prompts")
	fmt.Println("• Custom options (size, quality, background)")
	fmt.Println("• Multi-turn editing using previous response ID")
	fmt.Println("• Forced tool usage with tool_choice")
	fmt.Println("• Streaming image generation")
	fmt.Println("• Series generation for related images")
}

// saveImageFromResponse extracts base64 image data from a response and saves it to a file
func saveImageFromResponse(response llm.Response, filename string) error {
	// Look through the response content for image content that contains generated image data
	for _, content := range response.Content {
		if imageContent, ok := content.(*llm.ImageContent); ok {
			// Check if this has a base64 source
			if imageContent.Source != nil && imageContent.Source.Type == llm.ContentSourceTypeBase64 {
				return saveBase64Image(imageContent.Source.Data, filename)
			}
		}
	}

	return fmt.Errorf("no image data found in response")
}

// saveBase64Image decodes base64 image data and saves it to a file
func saveBase64Image(base64Data, filename string) error {
	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return fmt.Errorf("failed to decode base64 image: %w", err)
	}

	if err := os.WriteFile(filename, imageData, 0644); err != nil {
		return fmt.Errorf("failed to write image file: %w", err)
	}

	return nil
}
