package main

import (
	"context"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/llm/providers/google"
	"github.com/deepnoodle-ai/dive/schema"
)

func main() {
	provider := google.New(google.WithModel(google.ModelGemini25FlashPro))

	ctx := context.Background()

	// Create a simple tool for testing
	tool := llm.NewToolDefinition().
		WithName("get_weather").
		WithDescription("Get weather information").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"location"},
			Properties: map[string]*schema.Property{
				"location": {Type: "string", Description: "The location to get weather for"},
			},
		})

	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("What is the weather in Tokyo?"),
	), llm.WithTools(tool))
	if err != nil {
		log.Fatalf("Error generating response: %v", err)
	}

	if text := response.Message().Text(); text != "" {
		fmt.Println(response.Message().Text())
	}

	calls := response.ToolCalls()
	if len(calls) != 1 {
		log.Fatalf("Expected 1 tool call, got %d", len(calls))
	}

	fmt.Printf("Tool call: %+v\n", calls[0])

	// Create a conversation with the tool result
	toolResult := &llm.ToolResultContent{
		ToolUseID: calls[0].ID,
		Content:   `{"temperature": "29°C", "condition": "partly cloudy"}`,
		IsError:   false,
	}

	response2, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("What is the weather in Tokyo?"),
		response.Message(),
		llm.NewToolResultMessage(toolResult),
	), llm.WithTools(tool))
	if err != nil {
		log.Fatalf("Error generating response: %v", err)
	}

	fmt.Println(response2.Message().Text())
}
