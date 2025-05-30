package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/openai"
)

func main() {
	var pdfPath, prompt, fileID string
	flag.StringVar(&pdfPath, "pdf", "", "path to PDF file to analyze")
	flag.StringVar(&fileID, "file-id", "", "OpenAI file ID to use instead of uploading")
	flag.StringVar(&prompt, "prompt", "What is this document about? Provide a brief summary.", "prompt to use")
	flag.Parse()

	if pdfPath == "" && fileID == "" {
		log.Fatal("Either -pdf or -file-id must be provided")
	}

	ctx := context.Background()
	provider := openai.New()

	var content []llm.Content

	if fileID != "" {
		// Pass existing file ID
		content = append(content,
			llm.NewTextContent(prompt),
			llm.NewDocumentContent(llm.FileID(fileID)),
		)
	} else {
		// Pass PDF file contents directly
		data, err := os.ReadFile(pdfPath)
		if err != nil {
			log.Fatalf("Error opening PDF file: %v", err)
		}
		content = append(content,
			llm.NewTextContent(prompt),
			llm.NewDocumentContent(llm.RawData("application/pdf", data)),
		)
	}

	response, err := provider.Generate(ctx, llm.WithMessages(llm.NewUserMessage(content...)))
	if err != nil {
		log.Fatalf("Error generating response: %v", err)
	}

	fmt.Println(response.Message().Text())
}
