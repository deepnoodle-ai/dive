package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
)

const DefaultPrompt = "Using Python, calculate 53^4."

func main() {
	var prompt string
	flag.StringVar(&prompt, "p", DefaultPrompt, "prompt to use")
	flag.Parse()

	ctx := context.Background()

	response, err := anthropic.New().Generate(
		ctx,
		llm.WithMessages(llm.NewUserTextMessage(prompt)),
		llm.WithFeatures(anthropic.FeatureCodeExecution),
		llm.WithTools(
			anthropic.NewCodeExecutionTool(anthropic.CodeExecutionToolOptions{}),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Message().Text())
}
