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
	var prompt, url string
	flag.StringVar(&prompt, "prompt", "Respond with the first three sentences of the PDF.", "prompt to use")
	flag.StringVar(&url, "url", "https://pdfa.org/norm-refs/warnock_camelot.pdf", "url to use")
	flag.Parse()

	response, err := anthropic.New().Generate(
		context.Background(),
		llm.WithMessages(llm.Messages{
			llm.NewUserTextMessage(prompt),
			llm.NewUserMessage(
				&llm.DocumentContent{
					Title: "My Great PDF",
					Source: &llm.ContentSource{
						Type: llm.ContentSourceTypeURL,
						URL:  url,
					},
				},
			),
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Message().Text())
}
