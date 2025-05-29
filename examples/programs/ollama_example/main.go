package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/ollama"
	"github.com/diveagents/dive/schema"
)

func main() {
	var modelName string
	flag.StringVar(&modelName, "model", ollama.ModelMistral_7B, "The model to use")
	flag.Parse()

	// Create an Ollama provider. Assumes Ollama is running locally on the
	// default port (11434)
	provider := ollama.New(
		ollama.WithModel(modelName),
		ollama.WithMaxTokens(2048),
		// ollama.WithEndpoint("http://localhost:11434"),
		// ollama.WithAPIKey("ollama"),
	)

	ctx := context.Background()

	// Example 1: Simple text generation
	fmt.Println("=== Simple Text Generation ===")
	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Explain quantum computing in two sentences."),
		llm.WithMaxTokens(500),
		llm.WithTemperature(0.7),
	)
	if err != nil {
		log.Fatalf("Error generating response: %v", err)
	}

	fmt.Printf("Model: %s\n", response.Model)
	fmt.Printf("Response: %s\n", response.Message().Text())
	fmt.Printf("Usage: %d input tokens, %d output tokens\n\n",
		response.Usage.InputTokens, response.Usage.OutputTokens)

	// Example 2: Streaming response
	fmt.Println("=== Streaming Response ===")
	iterator, err := provider.Stream(ctx,
		llm.WithUserTextMessage("Write a short poem about artificial intelligence."),
		llm.WithMaxTokens(300),
		llm.WithTemperature(0.8),
	)
	if err != nil {
		log.Fatalf("Error creating stream: %v", err)
	}
	defer iterator.Close()

	fmt.Print("Streaming response: ")
	for iterator.Next() {
		event := iterator.Event()
		switch event.Type {
		case llm.EventTypeContentBlockDelta:
			if event.Delta != nil && event.Delta.Type == llm.EventDeltaTypeText {
				fmt.Print(event.Delta.Text)
			}
		case llm.EventTypeMessageStop:
			fmt.Println()
			fmt.Println()
		}
	}

	if err := iterator.Err(); err != nil {
		log.Fatalf("Error during streaming: %v", err)
	}

	// Example 3: Using tools (if your Ollama model supports function calling)
	fmt.Println("=== Tool Usage Example ===")

	// Define a simple tool
	weatherTool := llm.NewToolDefinition().
		WithName("get_weather").
		WithDescription("Get the current weather for a location").
		WithSchema(schema.Schema{
			Type:     "object",
			Required: []string{"location"},
			Properties: map[string]*schema.Property{
				"location": {
					Type:        "string",
					Description: "The city and state/country",
				},
			},
		})

	toolResponse, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What's the weather like in San Francisco?"),
		llm.WithTools(weatherTool),
		llm.WithMaxTokens(500),
	)
	if err != nil {
		log.Fatalf("Error with tool usage: %v", err)
	}

	fmt.Printf("Tool response: %s\n", toolResponse.Message().Text())

	// Check if the model made any tool calls
	for _, content := range toolResponse.Content {
		if toolUse, ok := content.(*llm.ToolUseContent); ok {
			fmt.Printf("Tool called: %s with arguments: %s\n",
				toolUse.Name, string(toolUse.Input))
		}
	}
}
