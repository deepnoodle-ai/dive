package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/environment"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/providers/anthropic"
	"github.com/diveagents/dive/providers/groq"
	"github.com/diveagents/dive/providers/openai"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/google"
	"github.com/diveagents/dive/workflow"
	"github.com/mendableai/firecrawl-go"
)

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
		var opts []anthropic.Option
		if modelName != "" {
			opts = append(opts, anthropic.WithModel(modelName))
		}
		provider = anthropic.New(opts...)
	case "openai":
		var opts []openai.Option
		if modelName != "" {
			opts = append(opts, openai.WithModel(modelName))
		}
		provider = openai.New(opts...)
	case "groq":
		var opts []groq.Option
		if modelName != "" {
			opts = append(opts, groq.WithModel(modelName))
		}
		provider = groq.New(opts...)
	}

	logLevel := "info"
	if verbose {
		logLevel = "debug"
	}
	logger := slogger.New(slogger.LevelFromString(logLevel))

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
		logger.Info("firecrawl enabled")
	} else {
		logger.Info("firecrawl is not enabled")
	}

	if key := os.Getenv("GOOGLE_SEARCH_CX"); key != "" {
		googleClient, err := google.New()
		if err != nil {
			log.Fatal(err)
		}
		theTools = append(theTools, toolkit.NewGoogleSearch(googleClient))
		logger.Info("google search enabled")
	} else {
		logger.Info("google search is not enabled")
	}

	if len(theTools) == 0 {
		logger.Warn("no tools enabled")
	}

	supervisor, err := agent.NewAgent(agent.AgentOptions{
		Name:         "Supervisor",
		Backstory:    "Research Supervisor and Renowned Author. Assign research tasks to the research assistant, but prepare the final reports or biographies yourself.",
		IsSupervisor: true,
		Subordinates: []string{"Research Assistant"},
		CacheControl: "ephemeral",
		LLM:          provider,
		Logger:       logger,
	})
	if err != nil {
		log.Fatal(err)
	}

	researcher, err := agent.NewAgent(agent.AgentOptions{
		Name:         "Research Assistant",
		Backstory:    "You are an expert research assistant. Don't go too deep into the details unless specifically asked.",
		CacheControl: "ephemeral",
		LLM:          provider,
		Tools:        theTools,
		Logger:       logger,
	})
	if err != nil {
		log.Fatal(err)
	}

	w, err := workflow.NewWorkflow(workflow.WorkflowOptions{
		Name:        "Research Workflow",
		Description: "A workflow for the research assistant. The supervisor will assign tasks to the research assistant.",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name: "Research Step",
				Prompt: &dive.Prompt{
					Text: "Research the history of maple syrup production in Vermont.",
				},
			}),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	env, err := environment.New(environment.EnvironmentOptions{
		Name:        "Research Environment",
		Description: "A research environment for the research assistant. The supervisor will assign tasks to the research assistant.",
		Agents:      []dive.Agent{supervisor, researcher},
		Workflows:   []*workflow.Workflow{w},
	})
	if err != nil {
		log.Fatal(err)
	}

	execution, err := env.ExecuteWorkflow(ctx, w.Name(), map[string]interface{}{})
	if err != nil {
		log.Fatal(err)
	}

	if err := execution.Wait(); err != nil {
		log.Fatal(err)
	}

	for stepName, output := range execution.StepOutputs() {
		fmt.Printf("---- step %s ----\n", stepName)
		fmt.Println(output)
		fmt.Println()
	}
}
