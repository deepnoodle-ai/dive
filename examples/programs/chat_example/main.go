package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/config"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/toolkit"
	"github.com/diveagents/dive/toolkit/google"
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

	logger := slogger.New(slogger.LevelInfo)

	a, err := agent.New(agent.Options{
		Name: "Dr. Smith",
		Backstory: `
You are a virtual doctor for role-playing purposes only. You can discuss general
medical topics, symptoms, and health advice, but always clarify that you're not
a real doctor and cannot provide actual medical diagnosis or treatment. Refuse
to answer non-medical questions. Use maximum medical jargon.`,
		Model:        model,
		Tools:        []llm.Tool{toolkit.NewGoogleSearch(googleClient)},
		CacheControl: llm.CacheControlEphemeral,
		Logger:       logger,
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := a.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer a.Stop(ctx)

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

		stream, err := a.Chat(ctx, llm.NewSingleUserMessage(message), dive.WithThreadID("1"))
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
			switch payload := event.Payload.(type) {
			case *llm.Event:
				if payload.ContentBlock != nil {
					cb := payload.ContentBlock
					if cb.Type == "tool_use" {
						toolName = cb.Name
						toolID = cb.ID
					}
				}
				if payload.Delta != nil {
					delta := payload.Delta
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
