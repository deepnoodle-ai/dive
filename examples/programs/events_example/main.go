package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/agent"
	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
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

	a, err := agent.New(agent.Options{Model: model})
	if err != nil {
		log.Fatal(err)
	}

	message := llm.NewUserTextMessage("count to 10")

	stream, err := a.StreamResponse(ctx, dive.WithMessage(message))
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()

	var events []*dive.ResponseEvent

	for stream.Next(ctx) {
		event := stream.Event()
		events = append(events, event)
		eventJSON, err := json.MarshalIndent(event, "", "  ")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(boldStyle.Sprintf("\nEvent: %s", event.Type), string(eventJSON))
	}
	fmt.Println()

	eventJSON, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("events.json", eventJSON, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Println(boldStyle.Sprintf("Events written to events.json"))
}
