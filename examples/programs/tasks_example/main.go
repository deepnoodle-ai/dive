package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/providers/anthropic"
	"github.com/diveagents/dive/providers/groq"
	"github.com/diveagents/dive/providers/openai"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/google"
	"github.com/mendableai/firecrawl-go"
)

func main() {
	var verbose bool
	var providerName string
	flag.StringVar(&providerName, "provider", "anthropic", "provider to use")
	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.Parse()

	ctx := context.Background()

	var provider llm.LLM
	switch providerName {
	case "anthropic":
		provider = anthropic.New()
	case "openai":
		provider = openai.New()
	case "groq":
		provider = groq.New(groq.WithModel("deepseek-r1-distill-llama-70b"))
	}

	var theTools []llm.Tool

	if key := os.Getenv("FIRECRAWL_API_KEY"); key != "" {
		app, err := firecrawl.NewFirecrawlApp(key, "")
		if err != nil {
			log.Fatal(err)
		}
		scraper := toolkit.NewFirecrawlScrapeTool(toolkit.FirecrawlScrapeToolOptions{
			App: app,
		})
		theTools = append(theTools, scraper)

		log.Println("firecrawl enabled")
	}

	if key := os.Getenv("GOOGLE_SEARCH_CX"); key != "" {
		googleClient, err := google.New()
		if err != nil {
			log.Fatal(err)
		}
		theTools = append(theTools, toolkit.NewGoogleSearch(googleClient))

		log.Println("google search enabled")
	}

	a, err := agent.NewAgent(agent.AgentOptions{
		Name:         "Research Assistant",
		CacheControl: "ephemeral",
		LLM:          provider,
		Tools:        theTools,
		Logger:       slogger.New(slogger.LevelDebug),
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := a.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer a.Stop(ctx)

	task := agent.NewTask(agent.TaskOptions{
		Name: "Research the history of beer",
		Prompt: &dive.Prompt{
			Text:         "Research the history of beer",
			Output:       "The history of beer",
			OutputFormat: string(dive.OutputMarkdown),
		},
	})

	iterator, err := a.Work(ctx, task)
	if err != nil {
		log.Fatal(err)
	}
	defer iterator.Close()

	for iterator.Next(ctx) {
		event := iterator.Event()
		switch p := event.Payload.(type) {
		case *dive.TaskResult:
			fmt.Println("result:\n", p.Content)
		}
	}
}
