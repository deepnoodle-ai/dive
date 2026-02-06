package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/openai"
)

const DefaultPrompt = "What is the scientific name of the largest animal on Earth?"

func main() {
	var prompt string
	flag.StringVar(&prompt, "p", DefaultPrompt, "prompt to use")
	flag.Parse()

	ctx := context.Background()

	response, err := openai.New().Generate(
		ctx,
		llm.WithModel(openai.ModelGPT5),
		llm.WithUserTextMessage(prompt),
		llm.WithMaxTokens(2048),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Message().Text())
}
