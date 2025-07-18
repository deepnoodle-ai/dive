package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/config"
	"github.com/diveagents/dive/environment"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/firecrawl"
	"github.com/diveagents/dive/toolkit/google"
	"github.com/diveagents/dive/workflow"
)

func main() {
	var verbose bool
	var providerName, modelName string
	flag.StringVar(&providerName, "provider", "anthropic", "provider to use")
	flag.StringVar(&modelName, "model", "", "model to use")
	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.Parse()

	ctx := context.Background()

	model, err := config.GetModel(providerName, modelName)
	if err != nil {
		log.Fatal(err)
	}

	logLevel := "info"
	if verbose {
		logLevel = "debug"
	}
	logger := slogger.New(slogger.LevelFromString(logLevel))

	var theTools []dive.Tool

	if key := os.Getenv("FIRECRAWL_API_KEY"); key != "" {
		client, err := firecrawl.New(firecrawl.WithAPIKey(key))
		if err != nil {
			log.Fatal(err)
		}
		scraper := toolkit.NewFetchTool(toolkit.FetchToolOptions{Fetcher: client})
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
		theTools = append(theTools, toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{Searcher: googleClient}))
		logger.Info("google search enabled")
	} else {
		logger.Info("google search is not enabled")
	}

	if len(theTools) == 0 {
		logger.Warn("no tools enabled")
	}

	supervisor, err := agent.New(agent.Options{
		Name:         "Supervisor",
		Instructions: "Research Supervisor and Renowned Author. Assign research tasks to the research assistant, but prepare the final reports or biographies yourself.",
		IsSupervisor: true,
		Subordinates: []string{"Research Assistant"},
		Model:        model,
		Logger:       logger,
	})
	if err != nil {
		log.Fatal(err)
	}

	researcher, err := agent.New(agent.Options{
		Name:         "Research Assistant",
		Instructions: "You are an expert research assistant. Don't go too deep into the details unless specifically asked.",
		Model:        model,
		Tools:        theTools,
		Logger:       logger,
	})
	if err != nil {
		log.Fatal(err)
	}

	w, err := workflow.New(workflow.Options{
		Name:        "Research Workflow",
		Description: "A workflow for the research assistant. The supervisor will assign tasks to the research assistant.",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:   "Research Step",
				Prompt: "Research the history of maple syrup production in Vermont.",
			}),
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	env, err := environment.New(environment.Options{
		Name:        "Research Environment",
		Description: "A research environment for the research assistant. The supervisor will assign tasks to the research assistant.",
		Agents:      []dive.Agent{supervisor, researcher},
		Workflows:   []*workflow.Workflow{w},
		AutoStart:   true,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer env.Stop(ctx)

	execution, err := environment.NewExecution(environment.ExecutionOptions{
		Workflow:    w,
		Environment: env,
		Inputs:      map[string]interface{}{},
		Logger:      slogger.DefaultLogger,
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := execution.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
