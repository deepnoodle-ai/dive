package main

import (
	"context"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/openrouter"
)

func main() {
	ctx := context.Background()

	// Create OpenRouter provider with optional site information for rankings
	provider := openrouter.New(
		openrouter.WithModel("openai/gpt-4o"),
	)

	// Generate a response
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("What is the capital of France?"),
	))
	if err != nil {
		log.Fatalf("Error generating response: %v", err)
	}

	// Print the response
	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok {
			fmt.Println(textContent.Text)
		}
	}
}
