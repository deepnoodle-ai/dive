package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/diveagents/dive/llm/providers/openai"
	openaic "github.com/diveagents/dive/llm/providers/openaicompletions"
	"github.com/diveagents/dive/slogger"
)

func main() {
	var (
		provider = flag.String("provider", "anthropic", "LLM provider to use (anthropic, openai-responses)")
		pdfPath  = flag.String("pdf", "", "Path to PDF file to analyze")
		pdfURL   = flag.String("url", "", "URL to PDF file to analyze")
		prompt   = flag.String("prompt", "What are the key findings in this document?", "Prompt to use for analysis")
		logLevel = flag.String("log", "info", "Log level (debug, info, warn, error)")
	)
	flag.Parse()

	if *pdfPath == "" && *pdfURL == "" {
		log.Fatal("Either -pdf or -url must be specified")
	}

	ctx := context.Background()
	logger := slogger.New(slogger.LevelFromString(*logLevel))

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
		message = createMessageFromURL(*provider, *pdfURL, *prompt)
	} else {
		message = createMessageFromFile(*provider, *pdfPath, *prompt)
	}

	// Generate response
	response, err := model.Generate(ctx, llm.WithMessages(message), llm.WithLogger(logger))
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

func createMessageFromURL(provider, url, prompt string) *llm.Message {
	switch provider {
	case "anthropic":
		// Anthropic supports direct URL references
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
	case "openai-responses":
		// OpenAI Responses API also supports URL references
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
	default:
		log.Fatalf("Unsupported provider: %s", provider)
		return nil
	}
}

func createMessageFromFile(provider, filePath, prompt string) *llm.Message {
	// Read and encode the PDF file
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read PDF file: %v", err)
	}

	base64Data := base64.StdEncoding.EncodeToString(data)
	filename := filepath.Base(filePath)

	switch provider {
	case "anthropic":
		// Use DocumentContent for Anthropic (preferred method)
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
	case "openai-responses":
		// Use DocumentContent for unified approach across providers
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
	default:
		log.Fatalf("Unsupported provider: %s", provider)
		return nil
	}
}
