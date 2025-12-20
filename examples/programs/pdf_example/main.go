package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/providers/openai"
	openaic "github.com/deepnoodle-ai/dive/providers/openaicompletions"
)

func main() {
	var (
		provider = flag.String("provider", "anthropic", "LLM provider to use (anthropic, openai-responses)")
		prompt   = flag.String("prompt", "What are the key findings in this document?", "Prompt to use for analysis")
		pdfPath  = flag.String("pdf", "", "Path to PDF file to analyze")
		pdfURL   = flag.String("url", "", "URL to PDF file to analyze")
	)
	flag.Parse()

	if *pdfPath == "" && *pdfURL == "" {
		log.Fatal("Either -pdf or -url must be specified")
	}

	ctx := context.Background()

	// Create the appropriate LLM provider
	var model llm.LLM
	switch *provider {
	case "anthropic":
		model = anthropic.New()
	case "openai":
		model = openai.New()
	case "openai-completions":
		model = openaic.New()
	default:
		log.Fatalf("Unknown provider: %s. Use 'anthropic' or 'openai-responses'", *provider)
	}

	// Create the message with PDF content
	var message *llm.Message
	if *pdfURL != "" {
		message = createMessageFromURL(*pdfURL, *prompt)
	} else {
		message = createMessageFromFile(*pdfPath, *prompt)
	}

	// Generate response
	response, err := model.Generate(ctx,
		llm.WithMessages(message),
		llm.WithMaxTokens(4000),
	)
	if err != nil {
		log.Fatalf("Failed to generate response: %v", err)
	}

	fmt.Printf("Provider: %s\n", *provider)
	fmt.Printf("PDF: %s%s\n", *pdfPath, *pdfURL)
	fmt.Printf("Prompt: %s\n\n", *prompt)
	fmt.Printf("Response:\n%s\n", response.Message().Text())

	if response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 {
		fmt.Printf("\nUsage: %d input tokens, %d output tokens\n",
			response.Usage.InputTokens, response.Usage.OutputTokens)
	}
}

func createMessageFromURL(url, prompt string) *llm.Message {
	return &llm.Message{
		Role: llm.User,
		Content: []llm.Content{
			&llm.TextContent{Text: prompt},
			&llm.DocumentContent{
				Source: &llm.ContentSource{
					Type: llm.ContentSourceTypeURL,
					URL:  url,
				},
			},
		},
	}
}

func createMessageFromFile(filePath, prompt string) *llm.Message {
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read PDF file: %v", err)
	}
	base64Data := base64.StdEncoding.EncodeToString(data)
	filename := filepath.Base(filePath)

	return &llm.Message{
		Role: llm.User,
		Content: []llm.Content{
			&llm.TextContent{Text: prompt},
			&llm.DocumentContent{
				Title: filename,
				Source: &llm.ContentSource{
					Type:      llm.ContentSourceTypeBase64,
					MediaType: "application/pdf",
					Data:      base64Data,
				},
			},
		},
	}
}
