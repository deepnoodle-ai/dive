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
		// You can still set provider-level defaults that will be used when no per-request options are specified
	)

	ctx := context.Background()

	// Example 1: Basic generation with background option
	fmt.Println("=== Example 1: Basic generation with background processing ===")
	response1, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What is the capital of France?"),
		llm.WithTemperature(0.7),
		llm.WithMaxTokens(100),
		// Generic options now available to all providers
		llm.WithBackground(false),
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
		// Enable web search with specific configuration using generic options
		llm.WithWebSearch(llm.WebSearchConfig{
			Enabled:     true,
			Domains:     []string{"arxiv.org", "openai.com"},
			ContextSize: "medium",
		}),
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
		// Enable image generation with specific options using generic config
		llm.WithImageGeneration(llm.ImageGenerationConfig{
			Enabled:    true,
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
		llm.WithJSONSchema(schema),
		llm.WithInstructions("Return the data in the specified JSON format"),
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
		llm.WithReasoningEffort("high"),
		llm.WithServiceTier("premium"),
		llm.WithTopP(0.9),
		llm.WithUser("math-student-123"),
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
		// Enable multiple tools for this request using generic options
		llm.WithWebSearchEnabled(),
		llm.WithImageGenerationEnabled(),
		llm.WithTruncationStrategy("auto"),
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

	// Example 8: Provider-specific options using the generic provider options mechanism
	fmt.Println("=== Example 8: Provider-specific options ===")
	response8, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Tell me about quantum computing"),
		llm.WithTemperature(0.8),
		// Example of using provider-specific options for features unique to OpenAI Responses
		llm.WithProviderOption("openai-responses:custom_feature", true),
		llm.WithProviderOptions(map[string]interface{}{
			"openai-responses:experimental_mode":   "enhanced",
			"openai-responses:processing_priority": "high",
		}),
	)
	if err != nil {
		log.Printf("Error in example 8: %v", err)
	} else {
		fmt.Printf("Response with provider options: %s\n\n", response8.Content[0].(*llm.TextContent).Text)
	}

	fmt.Println("=== All examples completed ===")
}
