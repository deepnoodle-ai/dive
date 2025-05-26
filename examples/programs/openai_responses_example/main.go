package main

import (
	"context"
	"fmt"
	"log"

	"github.com/diveagents/dive/llm"
	openairesponses "github.com/diveagents/dive/llm/providers/openai-responses"
)

func main() {
	// Create provider with only provider-level configuration
	provider := openairesponses.New(
		openairesponses.WithModel("gpt-4.1"),
		// Note: No tool configuration here - tools are now configured per request
	)

	ctx := context.Background()

	// Example 1: Basic generation with store and background options
	fmt.Println("=== Example 1: Basic generation with store and background ===")
	response1, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What is the capital of France?"),
		llm.WithTemperature(0.7),
		llm.WithMaxTokens(100),
		// Provider-specific options using the new LLM functions
		openairesponses.LLMWithStore(true),
		openairesponses.LLMWithBackground(false),
	)
	if err != nil {
		log.Printf("Error in example 1: %v", err)
	} else {
		fmt.Printf("Response: %s\n\n", response1.Content[0].(*llm.TextContent).Text)
	}

	// Example 2: Web search with custom options
	fmt.Println("=== Example 2: Web search with custom options ===")
	response2, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What are the latest developments in AI?"),
		llm.WithTemperature(0.5),
		// Enable web search with specific configuration
		openairesponses.LLMWithWebSearchOptions(openairesponses.WebSearchOptions{
			Domains:           []string{"arxiv.org", "openai.com"},
			SearchContextSize: "medium",
		}),
		openairesponses.LLMWithStore(true),
	)
	if err != nil {
		log.Printf("Error in example 2: %v", err)
	} else {
		fmt.Printf("Response with web search: %s\n\n", response2.Content[0].(*llm.TextContent).Text)
	}

	// Example 3: Image generation
	fmt.Println("=== Example 3: Image generation ===")
	response3, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Generate an image of a sunset over mountains"),
		// Enable image generation with specific options
		openairesponses.LLMWithImageGenerationOptions(openairesponses.ImageGenerationOptions{
			Size:       "1024x1024",
			Quality:    "high",
			Background: "auto",
		}),
	)
	if err != nil {
		log.Printf("Error in example 3: %v", err)
	} else {
		fmt.Printf("Image generation response: %s\n\n", response3.Content[0].(*llm.TextContent).Text)
	}

	// Example 4: JSON schema output
	fmt.Println("=== Example 4: JSON schema output ===")
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string",
			},
			"age": map[string]interface{}{
				"type": "integer",
			},
		},
		"required": []string{"name", "age"},
	}

	response4, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Generate a person's information"),
		openairesponses.LLMWithJSONSchema(schema),
		openairesponses.LLMWithInstructions("Return the data in the specified JSON format"),
	)
	if err != nil {
		log.Printf("Error in example 4: %v", err)
	} else {
		fmt.Printf("JSON response: %s\n\n", response4.Content[0].(*llm.TextContent).Text)
	}

	// Example 5: Advanced configuration with reasoning and service tier
	fmt.Println("=== Example 5: Advanced configuration ===")
	response5, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Solve this complex math problem: What is the derivative of x^3 + 2x^2 - 5x + 1?"),
		llm.WithTemperature(0.1), // Low temperature for precise math
		openairesponses.LLMWithReasoningEffort("high"),
		openairesponses.LLMWithServiceTier("premium"),
		openairesponses.LLMWithTopP(0.9),
		openairesponses.LLMWithUser("math-student-123"),
	)
	if err != nil {
		log.Printf("Error in example 5: %v", err)
	} else {
		fmt.Printf("Math response: %s\n\n", response5.Content[0].(*llm.TextContent).Text)
	}

	// Example 6: MCP server integration
	fmt.Println("=== Example 6: MCP server integration ===")
	response6, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What transport protocols are supported in the 2025-03-26 version of the MCP spec?"),
		// Add MCP server configuration using the proper llm.Config approach
		llm.WithMCPServers(llm.MCPServerConfig{
			Type: "url",
			Name: "deepwiki",
			URL:  "https://mcp.deepwiki.com/mcp",
			ToolConfiguration: &llm.MCPToolConfiguration{
				Enabled:      true,
				AllowedTools: []string{"ask_question"},
			},
		}),
	)
	if err != nil {
		log.Printf("Error in example 6: %v", err)
	} else {
		fmt.Printf("MCP response: %s\n\n", response6.Content[0].(*llm.TextContent).Text)
	}

	// Example 6b: MCP server with authentication
	fmt.Println("=== Example 6b: MCP server with authentication ===")
	response6b, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Create a payment link for $20"),
		// Add authenticated MCP server
		llm.WithMCPServers(llm.MCPServerConfig{
			Type:               "url",
			Name:               "stripe",
			URL:                "https://mcp.stripe.com",
			AuthorizationToken: "sk_test_example", // In practice, use os.Getenv("STRIPE_API_KEY")
			ToolConfiguration: &llm.MCPToolConfiguration{
				Enabled: true,
			},
		}),
	)
	if err != nil {
		log.Printf("Error in example 6b: %v", err)
	} else {
		fmt.Printf("Stripe MCP response: %s\n\n", response6b.Content[0].(*llm.TextContent).Text)
	}

	// Example 7: Streaming with multiple tools
	fmt.Println("=== Example 7: Streaming with multiple tools ===")
	stream, err := provider.Stream(ctx,
		llm.WithUserTextMessage("Research the latest AI papers and generate a summary image"),
		// Enable multiple tools for this request
		openairesponses.LLMWithWebSearch(),
		openairesponses.LLMWithImageGeneration(),
		openairesponses.LLMWithStore(true),
		openairesponses.LLMWithTruncation("auto"),
	)
	if err != nil {
		log.Printf("Error starting stream: %v", err)
		return
	}
	defer stream.Close()

	fmt.Println("Streaming response:")
	for stream.Next() {
		event := stream.Event()
		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
			fmt.Print(event.Delta.Text)
		}
	}
	if err := stream.Err(); err != nil {
		log.Printf("Stream error: %v", err)
	}
	fmt.Println("\n")

	fmt.Println("=== All examples completed ===")
}
