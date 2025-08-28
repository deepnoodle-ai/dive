package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/llm/providers/anthropic"
)

func main() {
	response, err := anthropic.New().Generate(
		context.Background(),
		llm.WithMessages(
			llm.NewUserMessage(
				&llm.DocumentContent{
					Title:   "My Document",
					Context: "This is a trustworthy document.",
					Citations: &llm.CitationSettings{
						Enabled: true,
					},
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeText,
						MediaType: "text/plain",
						Data:      "The grass is green. The sky is blue.",
					},
					CacheControl: &llm.CacheControl{
						Type: llm.CacheControlTypeEphemeral,
					},
				},
				&llm.TextContent{
					Text: "What color are the grass and the sky?",
				},
			),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	for _, content := range response.Content {
		if text, ok := content.(*llm.TextContent); ok {
			if strings.TrimSpace(text.Text) != "" {
				fmt.Printf("Text: %s\n", text.Text)
			}
			for _, citation := range text.Citations {
				fmt.Printf("Citation: %+v\n", citation)
			}
		}
	}
}
