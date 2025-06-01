package main

import (
	"context"
	"fmt"
	"os"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/openai"
	"github.com/diveagents/dive/schema"
	"github.com/fatih/color"
)

func main() {
	provider := openai.New(openai.WithModel("gpt-4o"))

	ctx := context.Background()

	exampleBasicGeneration(ctx, provider)
	// exampleWebSearch(ctx, provider)
	exampleImageGeneration(ctx, provider)
	exampleJSONSchema(ctx, provider)
	exampleReasoning(ctx, provider)
	exampleMCPIntegration(ctx, provider)
	exampleMultipleMCPServers(ctx, provider)

	fmt.Println("=== All examples completed ===")
}

func fatal(err error) {
	if err != nil {
		fmt.Println(color.RedString("Error: %s", err))
		os.Exit(1)
	}
}

func exampleBasicGeneration(ctx context.Context, provider llm.LLM) {
	fmt.Println("=== Basic Generation ===")

	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What is the capital of France?"),
		llm.WithMaxTokens(100),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(response.Message().Text())
}

func exampleWebSearch(ctx context.Context, provider llm.LLM) {
	fmt.Println("=== Web Search Preview Tool ===")

	tool := openai.NewWebSearchPreviewTool(
		openai.WebSearchPreviewToolOptions{
			SearchContextSize: "medium",
			UserLocation: &openai.UserLocation{
				Type:    "approximate",
				Country: "US",
			},
		})

	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What is the population of Spain?"),
		llm.WithTools(tool),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(response.Message().Text())
}

func exampleImageGeneration(ctx context.Context, provider llm.LLM) {
	fmt.Println("=== Image Generation ===")

	tool := openai.NewImageGenerationTool(
		openai.ImageGenerationToolOptions{
			Size:       "1024x1024",
			Quality:    "low",
			Moderation: "low",
		})

	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Generate an image of a sunset over mountains"),
		llm.WithTools(tool),
	)
	if err != nil {
		fatal(err)
	}

	message := response.Message()
	fmt.Println(message.Text())

	for _, content := range message.Content {
		switch content := content.(type) {
		case *llm.ImageContent:
			imageData, err := content.Source.DecodedData()
			if err != nil {
				fatal(err)
			}
			if err := os.WriteFile("sunset.png", imageData, 0644); err != nil {
				fatal(err)
			}
			fmt.Println(color.GreenString("Image written to sunset.png"))
		}
	}
}

func exampleJSONSchema(ctx context.Context, provider llm.LLM) {
	fmt.Println("=== JSON Schema Output ===")

	schema := schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Property{
			"name": {Type: "string"},
			"age":  {Type: "integer"},
		},
		Required: []string{"name", "age"},
	}

	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Generate a person's information"),
		llm.WithJSONSchema(schema),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(response.Message().Text())
}

func exampleReasoning(ctx context.Context, provider llm.LLM) {
	fmt.Println("=== Reasoning ===")

	response, err := provider.Generate(ctx,
		llm.WithModel("o3"),
		llm.WithReasoningEffort("high"),
		llm.WithUserTextMessage("What is the derivative of x^3 + 2x^2 - 5x + 1?"),
		llm.WithMaxTokens(20000),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(response.Message().Text())
}

func exampleMCPIntegration(ctx context.Context, provider llm.LLM) {
	fmt.Println("=== MCP Server Integration ===")

	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Name three Stephen King books"),
		llm.WithMCPServers(llm.MCPServerConfig{
			Type: "url",
			Name: "deepwiki",
			URL:  "https://mcp.deepwiki.com/mcp",
			ToolConfiguration: &llm.MCPToolConfiguration{
				Enabled:      true,
				AllowedTools: []string{"ask_question"},
			},
			ToolApproval: "never",
		}),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(response.Message().Text())
}

func exampleMultipleMCPServers(ctx context.Context, provider llm.LLM) {
	fmt.Println("=== Multiple MCP Servers ===")

	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Search for Linear tickets and create a payment link"),
		llm.WithMCPServers(
			llm.MCPServerConfig{
				Type:               "url",
				Name:               "linear",
				URL:                "https://mcp.linear.app/sse",
				AuthorizationToken: "lin_api_example",
				ToolConfiguration: &llm.MCPToolConfiguration{
					Enabled: true,
				},
			},
			llm.MCPServerConfig{
				Type:               "url",
				Name:               "stripe",
				URL:                "https://mcp.stripe.com",
				AuthorizationToken: "sk_test_example",
				ToolConfiguration: &llm.MCPToolConfiguration{
					Enabled: true,
				},
				ToolApprovalFilter: &llm.MCPToolApprovalFilter{
					Always: []string{"list_products", "get_balance"},
					Never:  []string{"list_customers"},
				},
			},
		),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(response.Message().Text())
}
