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
		stream, err := a.StreamResponse(ctx,
			dive.WithMessage(llm.NewUserTextMessage(message)),
			dive.WithThreadID("1"),
		)
		if err != nil {
			log.Fatal(err)
		}
		defer stream.Close()

		var inToolUse bool
		toolUseAccum := ""
		toolName := ""
		toolID := ""

		for stream.Next(ctx) {
			event := stream.Event()
			// fmt.Println(event.Type)
			if event.Type == dive.EventTypeResponseFailed {
				log.Fatal(event.Error)
			}
			if event.Type != dive.EventTypeLLMEvent {
				continue
			}
			if llmEvent := event.Item.Event; llmEvent != nil {
				if llmEvent.ContentBlock != nil {
					cb := llmEvent.ContentBlock
					if cb.Type == "tool_use" {
						toolName = cb.Name
						toolID = cb.ID
					}
				}
				if llmEvent.Delta != nil {
					delta := llmEvent.Delta
					if delta.PartialJSON != "" {
						if !inToolUse {
							inToolUse = true
							fmt.Println("\n----")
						}
						toolUseAccum += delta.PartialJSON
					} else if delta.Text != "" {
						if inToolUse {
							fmt.Println("NAME:", toolName, "ID:", toolID)
							fmt.Println(toolUseAccum)
							fmt.Println("----")
							inToolUse = false
							toolUseAccum = ""
						}
						fmt.Print(delta.Text)
					}
				}
			}
		}
		fmt.Println()
	}
}
