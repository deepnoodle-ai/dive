package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/providers/anthropic"
	"github.com/diveagents/dive/providers/openai"
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
	var logLevel, message string
	flag.StringVar(&logLevel, "log-level", "debug", "Log level (debug, info, warn, error)")
	flag.StringVar(&message, "message", "Count to 5", "Message to send to the LLM")
	flag.Parse()

	messages := []*llm.Message{llm.NewUserMessage(message)}

	fmt.Println("Anthropic")

	stream(anthropic.New(), messages)

	fmt.Println("====")

	fmt.Println("OpenAI")

	stream(openai.New(), messages)
}

func stream(model llm.StreamingLLM, messages []*llm.Message) {
	var modelTools []llm.Tool
	if key := os.Getenv("GOOGLE_SEARCH_CX"); key != "" {
		googleClient, err := google.New()
		if err != nil {
			fatal("failed to initialize Google Search: %s", err)
		}
		modelTools = append(modelTools, toolkit.NewGoogleSearch(googleClient))
	}

	stream, err := model.Stream(
		context.Background(),
		messages,
		llm.WithTools(modelTools...),
	)
	if err != nil {
		fatal("Error: %s", err)
	}
	defer stream.Close()

	for stream.Next() {
		event := stream.Event()
		fmt.Println("----")
		fmt.Println(event.Type, event.Index)
		fmt.Printf("Content Block: %+v\n", event.ContentBlock)
		fmt.Printf("Delta: %+v\n", event.Delta)
		if event.Response != nil {
			fmt.Printf("Response: %+v\n", event.Response)
			message := event.Response.Message()
			fmt.Printf("Message: %+v\n", message.CompleteText())
		}
		fmt.Println()
	}

	if err := stream.Err(); err != nil {
		fatal("Error: %s", err)
	}
}
