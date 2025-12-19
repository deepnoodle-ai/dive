// summarize_cli is a standalone program for summarizing text using AI.
//
// Usage:
//
//	cat document.txt | go run ./examples/programs/summarize_cli
//	cat document.txt | go run ./examples/programs/summarize_cli --length short
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
)

func main() {
	provider := flag.String("provider", "", "LLM provider to use")
	model := flag.String("model", "", "Model to use")
	length := flag.String("length", "medium", "Summary length: short, medium, or long")
	flag.Parse()

	if err := runSummarize(*provider, *model, *length); err != nil {
		log.Fatal(err)
	}
}

func readStdin() (string, error) {
	var content strings.Builder
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if line != "" {
					content.WriteString(line)
				}
				break
			}
			return "", fmt.Errorf("error reading from stdin: %v", err)
		}
		content.WriteString(line)
	}
	return strings.TrimSpace(content.String()), nil
}

func summarizationPrompts(length string) string {
	switch length {
	case "short":
		return "You are a concise summarization assistant. Create a brief, focused summary that captures only the most essential points. Aim for 1-3 sentences that distill the core message or findings. Be direct and eliminate any redundancy."
	case "medium":
		return "You are a balanced summarization assistant. Create a well-structured summary that covers the main points while maintaining important context and details. Aim for a paragraph or two that provides a comprehensive overview without being verbose."
	case "long":
		return "You are a detailed summarization assistant. Create a thorough summary that preserves important details, context, and nuances while organizing the information clearly. Include key supporting points and maintain the structure of the original content."
	default:
		return "You are a summarization assistant. Create a clear, well-organized summary of the provided text that captures the main points and key details."
	}
}

func runSummarize(providerName, modelName, length string) error {
	ctx := context.Background()

	input, err := readStdin()
	if err != nil {
		return fmt.Errorf("failed to read input: %v", err)
	}

	if input == "" {
		return fmt.Errorf("no input provided - please pipe text to this command (e.g., cat document.txt | summarize_cli)")
	}

	llmInstance, err := config.GetModel(providerName, modelName)
	if err != nil {
		return fmt.Errorf("error getting model: %v", err)
	}

	systemPrompt := summarizationPrompts(length)
	userPrompt := fmt.Sprintf("Please summarize the following text:\n\n%s", input)

	opts := []llm.Option{
		llm.WithSystemPrompt(systemPrompt),
		llm.WithUserTextMessage(userPrompt),
	}

	if streamingLLM, ok := llmInstance.(llm.StreamingLLM); ok {
		stream, err := streamingLLM.Stream(ctx, opts...)
		if err != nil {
			return fmt.Errorf("error streaming: %v", err)
		}
		defer stream.Close()

		for stream.Next() {
			event := stream.Event()
			if event.Delta != nil && event.Delta.Text != "" {
				fmt.Print(event.Delta.Text)
			}
		}
		if stream.Err() != nil {
			return fmt.Errorf("error streaming response: %v", stream.Err())
		}
	} else {
		response, err := llmInstance.Generate(ctx, opts...)
		if err != nil {
			return fmt.Errorf("error generating response: %v", err)
		}
		if len(response.Content) > 0 {
			if textContent, ok := response.Content[0].(*llm.TextContent); ok {
				fmt.Print(textContent.Text)
			}
		}
	}

	fmt.Println()
	return nil
}
