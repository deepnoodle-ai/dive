package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/diveagents/dive/llm/providers/openai"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/google"
	"github.com/fatih/color"
)

var (
	errorStyle = color.New(color.FgRed)
)

func fatal(msg string, args ...interface{}) {
	fmt.Printf(errorStyle.Sprint(msg)+"\n", args...)
	os.Exit(1)
}

func main() {
	var logLevel, prompt string
	flag.StringVar(&logLevel, "log-level", "debug", "Log level (debug, info, warn, error)")
	flag.StringVar(&prompt, "prompt", "Count to 5", "Prompt to send to the LLM")
	flag.Parse()

	messages := llm.Messages{llm.NewUserMessage(prompt)}

	fmt.Println("====")
	fmt.Println("Anthropic")
	stream(anthropic.New(), messages)

	fmt.Println("====")
	fmt.Println("OpenAI")
	stream(openai.New(), messages)
}

func stream(model llm.StreamingLLM, messages llm.Messages) {
	var modelTools []llm.Tool
	if key := os.Getenv("GOOGLE_SEARCH_CX"); key != "" {
		googleClient, err := google.New()
		if err != nil {
			fatal("failed to initialize Google Search: %s", err)
		}
		modelTools = append(modelTools, toolkit.NewSearchTool(googleClient))
	}

	stream, err := model.Stream(
		context.Background(),
		messages,
		llm.WithTools(modelTools...),
		llm.WithTemperature(0.1),
		llm.WithSystemPrompt("You are a helpful assistant."),
	)
	if err != nil {
		fatal("error: %s", err)
	}
	defer stream.Close()

	accumulator := llm.NewResponseAccumulator()

	for stream.Next() {
		event := stream.Event()
		if err := accumulator.AddEvent(event); err != nil {
			fatal("error: %s", err)
		}
		eventData, err := json.Marshal(event)
		if err != nil {
			fatal("error: %s", err)
		}
		fmt.Println(string(eventData))
	}

	if err := stream.Err(); err != nil {
		fatal("error: %s", err)
	}

	if !accumulator.IsComplete() {
		fatal("incomplete response")
	}
	response := accumulator.Response()

	fmt.Println("----")
	fmt.Println("hydrated response:")

	responseData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		fatal("error: %s", err)
	}
	fmt.Println(string(responseData))
}
