package main

import (
	"context"
	"log"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/providers/anthropic"
	"github.com/getstingrai/dive/tools"
	"github.com/getstingrai/dive/tools/google"
)

func main() {
	ctx := context.Background()

	// Create a Google Search API client
	googleClient, err := google.New()
	if err != nil {
		log.Fatal(err)
	}

	// Create a new agent
	agent := dive.NewAgent(dive.AgentOptions{
		Name:         "Chris",
		Description:  "Research Assistant",
		Instructions: "Use Google to research assigned topics",
		LLM:          anthropic.New(),
		IsSupervisor: false,
		Tools:        []llm.Tool{tools.NewGoogleSearch(googleClient)},
	})

	// Start the agent
	if err := agent.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer agent.Stop(ctx)

	// Create and assign a task
	task := dive.NewTask(dive.TaskOptions{
		Description: "Research the history of AI and summarize in 3 paragraphs",
	})

	promise, err := agent.Work(ctx, task)
	if err != nil {
		log.Fatal(err)
	}

	// Wait for the result
	result, err := promise.Get(ctx)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Task result: %s", result.Output)
}
