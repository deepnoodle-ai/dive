package main

import (
	"context"
	"fmt"
	"log"

	"github.com/diveagents/dive/llm"
	openairesponses "github.com/diveagents/dive/llm/providers/openai-responses"
	"github.com/diveagents/dive/schema"
)

func main() {
	provider := openairesponses.New(
		openairesponses.WithModel("gpt-4.1"),
	)

	ctx := context.Background()

	// Example 1: Basic generation
	fmt.Println("=== Example 1: Basic generation ===")
	response1, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What is the capital of France?"),
		llm.WithTemperature(0.7),
		llm.WithMaxTokens(100),
	)
	if err != nil {
		log.Printf("Error in example 1: %v", err)
	} else {
		fmt.Printf("Response: %s\n\n", response1.Message().Text())
	}

	// Example 2: Web search with custom options using proper tool
	fmt.Println("=== Example 2: Web search with custom options ===")
	webSearchTool := openairesponses.NewWebSearchTool(openairesponses.WebSearchToolOptions{
		SearchContextSize: "medium",
		UserLocation: &openairesponses.UserLocation{
			Type:    "approximate",
			Country: "US",
		},
	})

	response2, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What are the latest developments in AI?"),
		llm.WithTemperature(0.5),
		llm.WithTools(webSearchTool),
	)
	if err != nil {
		log.Printf("Error in example 2: %v", err)
	} else {
		fmt.Printf("Response with web search: %s\n\n", response2.Message().Text())
	}

	// Example 3: Image generation using proper tool
	fmt.Println("=== Example 3: Image generation ===")
	imageGenTool := openairesponses.NewImageGenerationTool(openairesponses.ImageGenerationToolOptions{
		Size:       "1024x1024",
		Quality:    "high",
		Background: "auto",
	})

	response3, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Generate an image of a sunset over mountains"),
		llm.WithTools(imageGenTool),
	)
	if err != nil {
		log.Printf("Error in example 3: %v", err)
	} else {
		fmt.Printf("Image generation response: %s\n\n", response3.Message().Text())
	}

	// Example 4: JSON schema output
	fmt.Println("=== Example 4: JSON schema output ===")
	schema := schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Property{
			"name": {Type: "string"},
			"age":  {Type: "integer"},
		},
		Required: []string{"name", "age"},
	}

	response4, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Generate a person's information"),
		llm.WithJSONSchema(schema),
	)
	if err != nil {
		log.Printf("Error in example 4: %v", err)
	} else {
		fmt.Printf("JSON response: %s\n\n", response4.Message().Text())
	}

	// Example 5: Advanced configuration with reasoning
	fmt.Println("=== Example 5: Advanced configuration ===")
	response5, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Solve this complex math problem: What is the derivative of x^3 + 2x^2 - 5x + 1?"),
		llm.WithTemperature(0.1), // Low temperature for precise math
		llm.WithReasoningEffort("high"),
		llm.WithServiceTier("premium"),
	)
	if err != nil {
		log.Printf("Error in example 5: %v", err)
	} else {
		fmt.Printf("Math response: %s\n\n", response5.Message().Text())
	}

	// Example 6: MCP server integration (unified approach - same as Anthropic)
	fmt.Println("=== Example 6: MCP server integration ===")
	response6, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What transport protocols are supported in the 2025-03-26 version of the MCP spec?"),
		llm.WithMCPServers(llm.MCPServerConfig{
			Type: "url",
			Name: "deepwiki",
			URL:  "https://mcp.deepwiki.com/mcp",
			ToolConfiguration: &llm.MCPToolConfiguration{
				Enabled:      true,
				AllowedTools: []string{"ask_question"},
			},
			ApprovalRequirement: "never", // Skip approvals for this trusted server
		}),
	)
	if err != nil {
		log.Printf("Error in example 6: %v", err)
	} else {
		fmt.Printf("MCP response: %s\n\n", response6.Message().Text())
	}

	// Example 6b: Multiple MCP servers with authentication
	fmt.Println("=== Example 6b: Multiple MCP servers ===")
	response6b, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Search for Linear tickets and create a payment link"),
		llm.WithMCPServers(
			llm.MCPServerConfig{
				Type:               "url",
				Name:               "linear",
				URL:                "https://mcp.linear.app/sse",
				AuthorizationToken: "lin_api_example", // In practice, use os.Getenv("LINEAR_API_KEY")
				ToolConfiguration: &llm.MCPToolConfiguration{
					Enabled: true,
				},
			},
			llm.MCPServerConfig{
				Type:               "url",
				Name:               "stripe",
				URL:                "https://mcp.stripe.com",
				AuthorizationToken: "sk_test_example", // In practice, use os.Getenv("STRIPE_API_KEY")
				ToolConfiguration: &llm.MCPToolConfiguration{
					Enabled: true,
				},
				// Example of selective approval - never require approval for read-only tools
				ApprovalRequirement: llm.MCPApprovalRequirement{
					Never: &llm.MCPNeverApproval{
						ToolNames: []string{"list_products", "get_balance", "list_customers"},
					},
				},
			},
		),
	)
	if err != nil {
		log.Printf("Error in example 6b: %v", err)
	} else {
		fmt.Printf("Multiple MCP servers response: %s\n\n", response6b.Message().Text())
	}

	// Example 7: Streaming with multiple tools
	fmt.Println("=== Example 7: Streaming with multiple tools ===")

	// Create both tools for this request
	webSearchStreamTool := openairesponses.NewWebSearchTool(openairesponses.WebSearchToolOptions{
		SearchContextSize: "medium",
	})
	imageGenStreamTool := openairesponses.NewImageGenerationTool(openairesponses.ImageGenerationToolOptions{
		Size:    "1024x1024",
		Quality: "high",
	})

	stream, err := provider.Stream(ctx,
		llm.WithUserTextMessage("Research the latest AI papers and generate a summary image"),
		llm.WithTools(webSearchStreamTool, imageGenStreamTool),
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
		fmt.Printf("Response with provider options: %s\n\n", response8.Message().Text())
	}

	fmt.Println("=== All examples completed ===")
}
