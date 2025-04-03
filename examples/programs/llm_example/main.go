package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
)

func main() {
	var verbose bool
	var prompt string
	flag.StringVar(&prompt, "prompt", "", "prompt to use")
	flag.BoolVar(&verbose, "verbose", false, "verbose output")
	flag.Parse()

	if prompt == "" {
		log.Fatal("provide a prompt with -prompt")
	}

	model := anthropic.New()
	response, err := model.Generate(
		context.Background(),
		llm.Messages{llm.NewUserMessage(prompt)},
		llm.WithMaxTokens(2048),
		llm.WithTemperature(0.7),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Message().Text())
}
