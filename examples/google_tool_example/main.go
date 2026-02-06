package main

import (
	"context"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/google"
	"github.com/deepnoodle-ai/wonton/schema"
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

	iter, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("What is the weather in Tokyo?"),
	), llm.WithTools(tool))
	if err != nil {
		log.Fatalf("Error generating response: %v", err)
	}

	accum := llm.NewResponseAccumulator()
	for iter.Next() {
		accum.AddEvent(iter.Event())
	}
	if err := iter.Err(); err != nil {
		log.Fatalf("Error streaming response: %v", err)
	}

	if text := accum.Response().Message().Text(); text != "" {
		fmt.Println(text)
	}

	calls := accum.Response().ToolCalls()
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

	iter2, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("What is the weather in Tokyo?"),
		accum.Response().Message(),
		llm.NewToolResultMessage(toolResult),
	), llm.WithTools(tool))
	if err != nil {
		log.Fatalf("Error generating response: %v", err)
	}

	accum2 := llm.NewResponseAccumulator()
	for iter2.Next() {
		accum2.AddEvent(iter2.Event())
	}
	if err := iter2.Err(); err != nil {
		log.Fatalf("Error streaming response: %v", err)
	}
	fmt.Println(accum2.Response().Message().Text())
}
