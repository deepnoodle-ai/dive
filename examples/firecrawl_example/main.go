// firecrawl_example demonstrates using Firecrawl as the backend for both
// the FetchTool and WebSearchTool in a Dive agent.
//
// Requires FIRECRAWL_API_KEY and ANTHROPIC_API_KEY to be set.
//
// Usage:
//
//	go run ./firecrawl_example
//	go run ./firecrawl_example -prompt "What is the latest Go release?"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/toolkit"
	"github.com/deepnoodle-ai/dive/toolkit/firecrawl"
)

func main() {
	prompt := flag.String("prompt", "What are the main features of Go 1.25? Find the release notes and summarize them.", "prompt to send to the agent")
	flag.Parse()

	fc, err := firecrawl.New()
	if err != nil {
		log.Fatal(err)
	}

	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:         "Research Assistant",
		SystemPrompt: "You are a research assistant. Use your tools to find accurate, up-to-date information.",
		Model:        anthropic.New(),
		Tools: []dive.Tool{
			toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{Searcher: fc.Searcher()}),
			toolkit.NewFetchTool(toolkit.FetchToolOptions{Fetcher: fc}),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Prompt: %s\n\n", *prompt)

	resp, err := agent.CreateResponse(context.Background(), dive.WithInput(*prompt))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.OutputText())
}
