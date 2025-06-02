package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/openai"
)

func main() {
	var prompt, file string
	flag.StringVar(&prompt, "prompt", "Respond with the first three sentences of the PDF.", "prompt to use")
	flag.StringVar(&file, "file", "", "file to use")
	flag.Parse()

	// read file in base64
	data, err := os.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}

	response, err := openai.New(openai.WithModel("gpt-4.1-mini")).Generate(
		context.Background(),
		llm.WithMessages(
			llm.NewUserTextMessage(prompt),
			llm.NewUserMessage(
				&llm.DocumentContent{
					Title: file,
					Source: &llm.ContentSource{
						Type:      llm.ContentSourceTypeBase64,
						MediaType: "application/pdf",
						Data:      base64.StdEncoding.EncodeToString(data),
					},
					CacheControl: &llm.CacheControl{
						Type: llm.CacheControlTypeEphemeral,
					},
				},
			),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(response.Message().Text())
}
