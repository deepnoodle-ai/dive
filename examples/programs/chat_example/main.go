package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/slogger"
	"github.com/deepnoodle-ai/dive/toolkit"
	"github.com/deepnoodle-ai/dive/toolkit/firecrawl"
	"github.com/deepnoodle-ai/dive/toolkit/google"
	"github.com/fatih/color"
)

var boldStyle = color.New(color.Bold)

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

	googleClient, err := google.New()
	if err != nil {
		log.Fatal(err)
	}

	firecrawlClient, err := firecrawl.New()
	if err != nil {
		log.Fatal(err)
	}

	logger := slogger.New(slogger.LevelInfo)

	a, err := agent.New(agent.Options{
		Name: "Dr. Smith",
		Instructions: `
You are a virtual doctor for role-playing purposes only. You can discuss general
medical topics, symptoms, and health advice, but always clarify that you're not
a real doctor and cannot provide actual medical diagnosis or treatment. Refuse
to answer non-medical questions. Use maximum medical jargon.`,
		Model: model,
		Tools: []dive.Tool{
			toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{Searcher: googleClient}),
			toolkit.NewFetchTool(toolkit.FetchToolOptions{Fetcher: firecrawlClient}),
		},
		Logger: logger,
	})
	if err != nil {
		log.Fatal(err)
	}

	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print(boldStyle.Sprint("\nEnter a chat message about a medical topic: "))
		message, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		message = strings.TrimSpace(message)
		if message == "exit" {
			break
		}
		if message == "" {
			continue
		}
		_, err = a.CreateResponse(ctx,
			dive.WithMessage(llm.NewUserTextMessage(message)),
			dive.WithThreadID("1"),
			dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
				if item.Type == dive.ResponseItemTypeEvent {
					event := item.Event
					if event.ContentBlock != nil {
						cb := event.ContentBlock
						if cb.Type == "tool_use" {
							fmt.Println("Tool used:", cb.Name)
						}
					}
					if event.Delta != nil {
						delta := event.Delta
						if delta.Text != "" {
							fmt.Print(delta.Text)
						}
					}
				}
				return nil
			}),
		)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println()
	}
}
