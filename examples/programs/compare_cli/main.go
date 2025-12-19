// compare_cli is a standalone program for comparing LLM providers.
//
// This example demonstrates how to benchmark different LLM providers for speed,
// cost, and quality. For the full implementation from cmd/dive/cli/compare.go,
// see the dive repository.
//
// Usage:
//
//	go run ./examples/programs/compare_cli --prompt "Hello" --providers "anthropic,openai"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/config"
	"github.com/deepnoodle-ai/dive/llm"
)

func main() {
	prompt := flag.String("prompt", "", "Prompt to send to each provider")
	providers := flag.String("providers", "", "Comma-separated list of providers to compare")
	flag.Parse()

	if *prompt == "" {
		log.Fatal("--prompt is required")
	}
	if *providers == "" {
		log.Fatal("--providers is required")
	}

	providerList := strings.Split(*providers, ",")
	for i, p := range providerList {
		providerList[i] = strings.TrimSpace(p)
	}

	if err := runCompare(*prompt, providerList); err != nil {
		log.Fatal(err)
	}
}

func runCompare(prompt string, providers []string) error {
	ctx := context.Background()

	fmt.Printf("Comparing %d providers with prompt: %q\n\n", len(providers), prompt)

	for _, providerName := range providers {
		fmt.Printf("Testing %s...\n", providerName)

		model, err := config.GetModel(providerName, "")
		if err != nil {
			fmt.Printf("  Error getting model: %v\n\n", err)
			continue
		}

		start := time.Now()
		response, err := model.Generate(ctx, llm.WithUserTextMessage(prompt))
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("  Error: %v\n\n", err)
			continue
		}

		var responseText string
		for _, content := range response.Content {
			if textContent, ok := content.(*llm.TextContent); ok {
				responseText = textContent.Text
				break
			}
		}

		fmt.Printf("  Duration: %v\n", duration)
		fmt.Printf("  Response length: %d chars\n", len(responseText))
		fmt.Printf("  Response preview: %.100s...\n\n", responseText)
	}

	return nil
}
