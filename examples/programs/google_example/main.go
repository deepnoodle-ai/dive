package main

import (
	"context"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/google"
)

func main() {
	// Create the Google provider
	provider := google.New(google.WithModel("gemini-2.0-flash-exp"))

	ctx := context.Background()

	// Example 1: Simple text generation
	fmt.Println("=== Simple Text Generation ===")
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Explain quantum computing in one sentence."),
	))
	if err != nil {
		log.Fatalf("Error generating response: %v", err)
	}
	fmt.Printf("Response: %s\n\n", response.Message().Text())

	// Example 2: Streaming response
	fmt.Println("=== Streaming Response ===")
	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Count from 1 to 5, one number per line."),
	))
	if err != nil {
		log.Fatalf("Error creating stream: %v", err)
	}
	defer iterator.Close()

	fmt.Print("Streaming response: ")
	for iterator.Next() {
		event := iterator.Event()
		if event.Type == llm.EventTypeContentBlockDelta {
			fmt.Print(event.Delta.Text)
		}
	}
	if err := iterator.Err(); err != nil {
		log.Fatalf("Error reading stream: %v", err)
	}
	fmt.Println()

	// Example 3: With system prompt and temperature
	fmt.Println("=== With System Prompt and Temperature ===")
	response, err = provider.Generate(ctx,
		llm.WithMessages(
			llm.NewUserTextMessage("Write a haiku about programming."),
		),
		llm.WithSystemPrompt("You are a poetic assistant who speaks in haikus."),
		llm.WithTemperature(0.8),
	)
	if err != nil {
		log.Fatalf("Error generating response: %v", err)
	}
	fmt.Printf("Response: %s\n", response.Message().Text())
}
