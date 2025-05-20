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
	var prompt string
	flag.StringVar(&prompt, "prompt", "", "prompt to use")
	flag.Parse()

	if prompt == "" {
		log.Fatal("provide a prompt with -prompt")
	}

	ctx := context.Background()

	response, err := anthropic.New().Generate(
		ctx,
		llm.WithUserTextMessage(prompt),
		llm.WithMaxTokens(2048),
		llm.WithTemperature(0.7),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Message().Text())
}
