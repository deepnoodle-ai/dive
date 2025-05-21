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
	flag.StringVar(&prompt, "prompt", "What are top Go projects on GitHub?", "prompt to use")
	flag.Parse()

	webSearchTool := anthropic.NewWebSearchTool(anthropic.WebSearchToolOptions{})

	response, err := anthropic.New().Generate(
		context.Background(),
		llm.WithTools(webSearchTool),
		llm.WithMessages(llm.NewUserTextMessage(prompt)),
	)
	if err != nil {
		log.Fatal(err)
	}

	message := response.Message()

	for _, content := range message.Content {
		fmt.Println("================")
		switch content := content.(type) {
		case *llm.TextContent:
			fmt.Println("Text:")
			fmt.Println(content.Text)
			for _, citation := range content.Citations {
				switch citation := citation.(type) {
				case *llm.WebSearchResultLocation:
					fmt.Println("Citation:")
					fmt.Println(citation.URL)
					fmt.Println("Cited text:", citation.CitedText)
				}
			}
		case *llm.ToolUseContent:
			fmt.Println("Tool use:")
			fmt.Println(content.Name)
			fmt.Println(content.Input)
		case *llm.ToolResultContent:
			fmt.Println("Tool result:")
			fmt.Println(content.ToolUseID)
			fmt.Println(content.Content)
		case *llm.ServerToolUseContent:
			fmt.Println("Server tool use:")
			fmt.Println(content.Name)
			fmt.Println(content.Input)
		case *llm.WebSearchToolResultContent:
			fmt.Println("Web search tool result:")
			fmt.Println(content.ToolUseID)
			for _, item := range content.Content {
				fmt.Println(item.Title)
				fmt.Println(item.URL)
			}
		}
	}
}
