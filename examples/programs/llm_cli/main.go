// llm_cli is a standalone program for direct LLM interaction.
//
// Usage:
//
//	go run ./examples/programs/llm_cli "What is the capital of France?"
//	go run ./examples/programs/llm_cli --stream "Tell me a story"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
)

func main() {
	provider := flag.String("provider", "", "LLM provider to use")
	model := flag.String("model", "", "Model to use")
	systemPrompt := flag.String("system", "", "System prompt")
	stream := flag.Bool("stream", false, "Stream the response")
	maxTokens := flag.Int("max-tokens", 0, "Maximum tokens to generate")
	temperature := flag.Float64("temperature", -1, "Temperature for randomness")
	flag.Parse()

	message := strings.Join(flag.Args(), " ")
	if message == "" {
		log.Fatal("no message provided")
	}

	if err := runLLM(*provider, *model, *systemPrompt, message, *stream, *maxTokens, *temperature); err != nil {
		log.Fatal(err)
	}
}

func runLLM(providerName, modelName, systemPrompt, message string, streaming bool, maxTokens int, temperature float64) error {
	ctx := context.Background()

	model, err := config.GetModel(providerName, modelName)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	var options []llm.Option
	options = append(options, llm.WithUserTextMessage(message))

	if systemPrompt != "" {
		options = append(options, llm.WithSystemPrompt(systemPrompt))
	}
	if maxTokens > 0 {
		options = append(options, llm.WithMaxTokens(maxTokens))
	}
	if temperature >= 0 {
		options = append(options, llm.WithTemperature(temperature))
	}

	if streaming {
		streamingModel, ok := model.(llm.StreamingLLM)
		if !ok {
			return fmt.Errorf("model does not support streaming")
		}

		stream, err := streamingModel.Stream(ctx, options...)
		if err != nil {
			return fmt.Errorf("error streaming: %v", err)
		}
		defer stream.Close()

		for stream.Next() {
			event := stream.Event()
			if event.Delta != nil {
				if event.Delta.Text != "" {
					fmt.Print(event.Delta.Text)
				}
				if event.Delta.Thinking != "" {
					fmt.Print(event.Delta.Thinking)
				}
			}
		}
		if stream.Err() != nil {
			return fmt.Errorf("error streaming response: %v", stream.Err())
		}
		fmt.Println()
	} else {
		response, err := model.Generate(ctx, options...)
		if err != nil {
			return fmt.Errorf("error generating response: %v", err)
		}
		for _, content := range response.Content {
			switch c := content.(type) {
			case *llm.TextContent:
				fmt.Println(c.Text)
			case *llm.ToolUseContent:
				fmt.Printf("Tool: %s\n%s\n", c.Name, string(c.Input))
			}
		}
	}
	return nil
}
