package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/logger"
	"github.com/getstingrai/dive/providers/anthropic"
	"github.com/getstingrai/dive/providers/groq"
	"github.com/getstingrai/dive/providers/openai"
	"github.com/getstingrai/dive/tools"
	"github.com/getstingrai/dive/tools/google"
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

	googleClient, err := google.New()
	if err != nil {
		log.Fatal(err)
	}

	logger := logger.NewSlogLogger(nil)

	a := dive.NewAgent(dive.AgentOptions{
		Name:     "Research Assistant",
		Role:     dive.Role{Description: "Research assistant"},
		LLM:      provider,
		Tools:    []llm.Tool{tools.NewGoogleSearch(googleClient)},
		LogLevel: "info",
		Logger:   logger,
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

	if err := a.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer a.Stop(ctx)

	task := dive.NewTask(dive.TaskOptions{
		Description:    "Research the history of the internet",
		ExpectedOutput: "A report of the key events and milestones in the history of the internet",
		OutputFormat:   dive.OutputMarkdown,
	})

	promise, err := a.Work(ctx, task)
	if err != nil {
		log.Fatal(err)
	}

	response, err := promise.Get(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(response.Content)
}
