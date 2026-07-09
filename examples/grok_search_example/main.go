// Command grok_search_example demonstrates Grok's server-side search tools:
// web search (with image search enabled) and X (Twitter) search. It prints the
// answer, any citations Grok returned, and the token usage — including the
// reasoning tokens Grok spent thinking.
//
// Run with XAI_API_KEY (or GROK_API_KEY) set:
//
//	go run ./grok_search_example -prompt "What did xAI announce recently?"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/grok"
)

const defaultPrompt = "What are the latest announcements from Anthropic? Answer in two sentences and cite your sources."

func main() {
	var prompt string
	flag.StringVar(&prompt, "prompt", defaultPrompt, "prompt to send to Grok")
	flag.Parse()

	// Web search with image search enabled (Grok can embed relevant images),
	// plus X search so Grok can pull in real-time posts from X.
	webSearch, err := grok.NewWebSearchTool(grok.WebSearchToolOptions{
		EnableImageSearch: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	xSearch, err := grok.NewXSearchTool(grok.XSearchToolOptions{
		AllowedXHandles: []string{"bcherny", "claudeai", "AnthropicAI"},
	})
	if err != nil {
		log.Fatal(err)
	}

	provider := grok.New() // defaults to grok-4.5

	response, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage(prompt)),
		llm.WithTools(webSearch, xSearch),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Answer:")
	fmt.Println(response.Message().Text())

	// Citations are attached to the assistant's text content blocks.
	fmt.Println("\nCitations:")
	for _, content := range response.Content {
		text, ok := content.(*llm.TextContent)
		if !ok {
			continue
		}
		for _, citation := range text.Citations {
			if loc, ok := citation.(*llm.WebSearchResultLocation); ok {
				fmt.Printf("  - %s (%s)\n", loc.Title, loc.URL)
			}
		}
	}

	fmt.Printf("\nUsage: input=%d output=%d reasoning=%d\n",
		response.Usage.InputTokens,
		response.Usage.OutputTokens,
		response.Usage.ReasoningTokens)
}
