package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/llm/providers/anthropic"
)

const DefaultPrompt = "What is the capital of Tanzania? Respond with a name and approximate population only."

func main() {
	var prompt string
	flag.StringVar(&prompt, "prompt", DefaultPrompt, "prompt to use")
	flag.Parse()

	webSearchTool := anthropic.NewWebSearchTool(anthropic.WebSearchToolOptions{})

	iterator, err := anthropic.New().Stream(
		context.Background(),
		llm.WithTools(webSearchTool),
		llm.WithMessages(llm.NewUserTextMessage(prompt)),
	)
	if err != nil {
		log.Fatal(err)
	}

	for iterator.Next() {
		event := iterator.Event()
		switch event.Type {
		case llm.EventTypeMessageStart:
			fmt.Println("----")
		case llm.EventTypeContentBlockDelta:
			if event.Delta.PartialJSON != "" {
				fmt.Print(event.Delta.PartialJSON)
			} else if event.Delta.Text != "" {
				fmt.Print(event.Delta.Text)
			} else if event.Delta.Thinking != "" {
				fmt.Print(event.Delta.Thinking)
			}
		case llm.EventTypeContentBlockStop:
			fmt.Println()
		case llm.EventTypeMessageStop:
			fmt.Println("----")
		}
	}
}
