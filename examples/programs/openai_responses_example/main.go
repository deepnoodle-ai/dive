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

var (
	green  = color.New(color.FgGreen)
	red    = color.New(color.FgRed)
	cyan   = color.New(color.FgCyan)
	italic = color.New(color.Italic)
	bold   = color.New(color.Bold)
)

func main() {
	provider := openai.New(openai.WithModel("gpt-4o"))

	ctx := context.Background()

	exampleBasicGeneration(ctx, provider)
	exampleWebSearch(ctx, provider)
	exampleReasoning(ctx, provider)
	exampleMCPIntegration(ctx, provider)
	exampleImageGeneration(ctx, provider)
	exampleJSONSchema(ctx, provider)
}

func fatal(err error) {
	if err != nil {
		fmt.Println(red.Sprintf("Error: %s", err))
		os.Exit(1)
	}
}

func header(text string) {
	fmt.Println("\n" + bold.Sprintf("==== %s ====", text))
}

func exampleBasicGeneration(ctx context.Context, provider llm.LLM) {
	header("Basic Generation")

	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("What is the capital of France?"),
		llm.WithMaxTokens(100),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(green.Sprint(response.Message().Text()))
}

func exampleWebSearch(ctx context.Context, provider llm.LLM) {
	header("Web Search Tool")

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
		llm.WithTemperature(0.0),
		llm.WithTools(tool),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(green.Sprint(response.Message().Text()))
}

func exampleImageGeneration(ctx context.Context, provider llm.LLM) {
	header("Image Generation")

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
	fmt.Println(green.Sprint(message.Text()))

	if image, ok := message.ImageContent(); ok {
		imageData, err := image.Source.DecodedData()
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile("sunset.png", imageData, 0644); err != nil {
			fatal(err)
		}
		fmt.Println(italic.Sprint("Image written to sunset.png"))
	} else {
		fatal(fmt.Errorf("no image content found"))
	}
}

func exampleJSONSchema(ctx context.Context, provider llm.LLM) {
	header("JSON Schema Output")

	type Person struct {
		Name string `json:"name" description:"The person's name"`
		Age  int    `json:"age" description:"The person's age"`
	}

	schema, err := schema.Generate(Person{})
	if err != nil {
		fatal(err)
	}

	response, err := provider.Generate(ctx,
		llm.WithUserTextMessage("Generate a person's information"),
		llm.WithResponseFormat(&llm.ResponseFormat{
			Name:        "person",
			Description: "A person's information",
			Type:        llm.ResponseFormatTypeJSONSchema,
			Schema:      schema,
		}),
	)
	if err != nil {
		fatal(err)
	}

	var output Person
	if err := response.Message().DecodeInto(&output); err != nil {
		fatal(err)
	}
	fmt.Println(green.Sprintf("Output: %+v", output))
}

func exampleReasoning(ctx context.Context, provider llm.LLM) {
	header("Reasoning")

	response, err := provider.Generate(ctx,
		llm.WithModel("o3"),
		llm.WithReasoningEffort("high"),
		llm.WithReasoningSummary("detailed"),
		llm.WithUserTextMessage("What is the derivative of x^3 + 2x^2 - 5x + 1?"),
		llm.WithMaxTokens(20000),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(green.Sprint(response.Message().Text()))

	if thinking, ok := response.Message().ThinkingContent(); ok {
		if thinking.Thinking != "" {
			fmt.Println()
			fmt.Println("Reasoning:", italic.Sprint(thinking.Thinking))
		}
		if thinking.Signature != "" {
			fmt.Println()
			fmt.Println("Reasoning signature:", cyan.Sprint(thinking.Signature))
		}
	}
}

func exampleMCPIntegration(ctx context.Context, provider llm.LLM) {
	header("MCP Server Integration")

	question := "What are key components of the Risor VM? (github.com/risor-io/risor)"

	response, err := provider.Generate(ctx,
		llm.WithModel("o3-mini"),
		llm.WithUserTextMessage(question),
		llm.WithMCPServers(llm.MCPServerConfig{
			Name:         "deepwiki",
			URL:          "https://mcp.deepwiki.com/mcp",
			ToolApproval: "never",
		}),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(green.Sprint(response.Message().Text()))
}
