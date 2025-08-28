package main

import (
	"context"
	"fmt"
	"os"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/llm/providers/openai"
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/fatih/color"
)

var (
	green  = color.New(color.FgGreen)
	red    = color.New(color.FgRed)
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
	exampleJSONSchema(ctx, provider)
	exampleStreaming(ctx, provider)
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
	}
}

func exampleMCPIntegration(ctx context.Context, provider llm.LLM) {
	header("MCP Server Integration")

	question := "What are key components of the Risor VM? (github.com/risor-io/risor)"

	response, err := provider.Generate(ctx,
		llm.WithModel("o3-mini"),
		llm.WithUserTextMessage(question),
		llm.WithMCPServers(llm.MCPServerConfig{
			Name:              "deepwiki",
			URL:               "https://mcp.deepwiki.com/mcp",
			ToolConfiguration: &llm.MCPToolConfiguration{ApprovalMode: "never"},
		}),
	)
	if err != nil {
		fatal(err)
	}
	fmt.Println(green.Sprint(response.Message().Text()))
}

func exampleStreaming(ctx context.Context, provider llm.StreamingLLM) {
	header("Streaming")

	iterator, err := provider.Stream(ctx,
		llm.WithUserTextMessage("Explain Docker containers in 3 sentences"),
	)
	if err != nil {
		fatal(err)
	}
	for iterator.Next() {
		event := iterator.Event()
		if event.Delta != nil && event.Delta.Text != "" {
			fmt.Print(green.Sprint(event.Delta.Text))
		}
	}
	fmt.Println()
}
