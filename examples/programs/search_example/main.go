package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/providers/anthropic"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/google"
)

func main() {

	var logLevel string
	if os.Getenv("LOG_LEVEL") != "" {
		logLevel = os.Getenv("LOG_LEVEL")
	} else {
		logLevel = "info"
	}

	ctx := context.Background()

	// You'll need to have these set:
	// - GOOGLE_SEARCH_CX
	// - GOOGLE_SEARCH_API_KEY
	// https://developers.google.com/custom-search/v1/introduction
	googleClient, err := google.New()
	if err != nil {
		log.Fatal(err)
	}

	researcher, err := agent.New(agent.Options{
		Name:   "Research Assistant",
		Goal:   "Use Google to research assigned topics",
		LLM:    anthropic.New(),
		Logger: slogger.New(slogger.LevelFromString(logLevel)),
		Tools:  []llm.Tool{toolkit.NewGoogleSearch(googleClient)},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := researcher.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer researcher.Stop(ctx)

	task := agent.NewTask(agent.TaskOptions{
		Timeout: 10 * time.Second,
		Prompt: &dive.Prompt{
			Text:         "Research the history of computing",
			Output:       "A brief report",
			OutputFormat: "markdown",
		},
	})

	stream, err := researcher.Work(ctx, task)
	if err != nil {
		log.Fatal(err)
	}

	result, err := dive.WaitForEvent[*dive.TaskResult](ctx, stream)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Content)
}
