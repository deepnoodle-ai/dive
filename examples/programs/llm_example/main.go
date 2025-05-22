package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
)

const DefaultPrompt = "What is the scientific name of the largest animal on Earth?"

func main() {
	var prompt string
	flag.StringVar(&prompt, "p", DefaultPrompt, "prompt to use")
	flag.Parse()

	ctx := context.Background()

	response, err := anthropic.New().Generate(
		ctx,
		llm.WithModel(anthropic.ModelClaudeSonnet4),
		llm.WithUserTextMessage(prompt),
		llm.WithMaxTokens(2048),
		llm.WithTemperature(0.7),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Message().Text())
}
