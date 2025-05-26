package main

import (
	"context"
	"fmt"
	"log"

	"github.com/diveagents/dive/llm"
	openairesponses "github.com/diveagents/dive/llm/providers/openai-responses"
)

func main() {
	// Example 1: Basic text generation
	fmt.Println("=== Basic Text Generation ===")
	basicExample()

	// Example 2: Web search
	fmt.Println("\n=== Web Search Example ===")
	webSearchExample()

	// Example 3: Image generation
	fmt.Println("\n=== Image Generation Example ===")
	imageGenerationExample()

	// Example 4: MCP server integration
	fmt.Println("\n=== MCP Server Example ===")
	mcpExample()
}

func basicExample() {
	// Create a basic provider
	provider := openairesponses.New(
		openairesponses.WithModel("gpt-4.1"),
	)

	response, err := provider.Generate(context.Background(),
		llm.WithUserTextMessage("Tell me a short story about a robot learning to paint."),
	)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Printf("Model: %s\n", response.Model)
	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok {
			fmt.Printf("Response: %s\n", textContent.Text)
		}
	}
	fmt.Printf("Usage: %d input tokens, %d output tokens\n",
		response.Usage.InputTokens, response.Usage.OutputTokens)
}

func webSearchExample() {
	// Create provider with web search enabled
	provider := openairesponses.New(
		openairesponses.WithModel("gpt-4.1"),
		openairesponses.WithWebSearchOptions(openairesponses.WebSearchOptions{
			SearchContextSize: "medium",
			UserLocation: &openairesponses.UserLocation{
				Type:    "approximate",
				Country: "US",
			},
		}),
	)

	response, err := provider.Generate(context.Background(),
		llm.WithUserTextMessage("What are the latest developments in AI safety research?"),
	)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Printf("Model: %s\n", response.Model)
	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok {
			fmt.Printf("Response: %s\n", textContent.Text)
		}
	}
}

func imageGenerationExample() {
	// Create provider with image generation enabled
	provider := openairesponses.New(
		openairesponses.WithModel("gpt-4.1"),
		openairesponses.WithImageGenerationOptions(openairesponses.ImageGenerationOptions{
			Size:    "1024x1024",
			Quality: "high",
			// Format field is not supported by the OpenAI Responses API
		}),
	)

	response, err := provider.Generate(context.Background(),
		llm.WithUserTextMessage("Generate an image of a serene mountain landscape at sunset."),
	)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Printf("Model: %s\n", response.Model)
	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok {
			fmt.Printf("Response: %s\n", textContent.Text)
		}
	}
}

func mcpExample() {
	// Create provider with MCP server integration
	provider := openairesponses.New(
		openairesponses.WithModel("gpt-4.1"),
		openairesponses.WithMCPServer("deepwiki", "https://mcp.deepwiki.com/mcp", nil),
	)

	response, err := provider.Generate(context.Background(),
		llm.WithUserTextMessage("What are the main features of the Model Context Protocol?"),
	)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	fmt.Printf("Model: %s\n", response.Model)
	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok {
			fmt.Printf("Response: %s\n", textContent.Text)
		}
	}
}
