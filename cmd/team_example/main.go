package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/logger"
	"github.com/getstingrai/dive/providers/anthropic"
	"github.com/getstingrai/dive/providers/groq"
	"github.com/getstingrai/dive/providers/openai"
	"github.com/getstingrai/dive/tools"
	"github.com/getstingrai/dive/tools/google"
	"github.com/mendableai/firecrawl-go"
)

// groq.WithModel("deepseek-r1-distill-llama-70b")

func main() {
	var verbose bool
	var providerName, modelName string
	flag.StringVar(&providerName, "provider", "anthropic", "provider to use")
	flag.StringVar(&modelName, "model", "", "model to use")
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
		provider = groq.New()
	}

	logger := logger.NewSlogLogger(nil)

	var theTools []llm.Tool

	if key := os.Getenv("FIRECRAWL_API_KEY"); key != "" {
		app, err := firecrawl.NewFirecrawlApp(key, "")
		if err != nil {
			log.Fatal(err)
		}
		theTools = append(theTools, tools.NewFirecrawlScraper(app, 30000))

		log.Println("firecrawl enabled")
	}

	if key := os.Getenv("GOOGLE_SEARCH_CX"); key != "" {
		googleClient, err := google.New()
		if err != nil {
			log.Fatal(err)
		}
		theTools = append(theTools, tools.NewGoogleSearch(googleClient))

		log.Println("google search enabled")
	}

	supervisor := dive.NewAgent(dive.AgentOptions{
		Name: "Supervisor",
		Role: dive.Role{
			Description:  "Research Supervisor",
			IsSupervisor: true,
			Subordinates: []string{"Research Assistant"},
		},
		CacheControl: "ephemeral",
		LLM:          provider,
		LogLevel:     "info",
		Logger:       logger,
	})

	researcher := dive.NewAgent(dive.AgentOptions{
		Name: "Research Assistant",
		Role: dive.Role{
			Description: "You are an expert research assistant. Don't go too deep into the details unless specifically asked.",
		},
		CacheControl: "ephemeral",
		LLM:          provider,
		Tools:        theTools,
		LogLevel:     "info",
		Logger:       logger,
		Hooks: llm.Hooks{
			llm.BeforeGenerate: func(ctx context.Context, hookCtx *llm.HookContext) {
				if verbose {
					fmt.Println("----")
					fmt.Println("INPUT")
					fmt.Println(dive.FormatMessages(hookCtx.Messages))
				}
			},
			llm.AfterGenerate: func(ctx context.Context, hookCtx *llm.HookContext) {
				if verbose {
					fmt.Println("OUTPUT")
					fmt.Println(dive.FormatMessages([]*llm.Message{hookCtx.Response.Message()}))
					fmt.Println("----")
				}
			},
		},
	})
	team, err := dive.NewTeam(dive.TeamOptions{
		Name:        "Elite Research Team",
		Description: "A team of researchers led by a supervisor. The supervisor should delegate work as needed.",
		Agents: []dive.Agent{
			supervisor,
			researcher,
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := team.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer team.Stop(ctx)

	task := dive.NewTask(dive.TaskOptions{
		Description:    "Quickly research the history of the internet",
		ExpectedOutput: "A brief report of the key events and milestones in the history of the internet",
		OutputFormat:   dive.OutputMarkdown,
	})

	results, err := team.Work(ctx, task)
	if err != nil {
		log.Fatal(err)
	}

	for _, result := range results {
		fmt.Println("----")
		fmt.Println("task", result.Task.Name())
		fmt.Println(result.Content)
		fmt.Println()
	}
}
