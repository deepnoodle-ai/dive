package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/diveagents/dive/llm"
	openairesponses "github.com/diveagents/dive/llm/providers/openai-responses"
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

	provider := openairesponses.New()

	var messages []*llm.Message

	if fileID != "" {
		// Use existing file ID
		messages = []*llm.Message{
			llm.NewUserTextMessage(prompt),
			llm.NewUserFileIDMessage(fileID),
		}
	} else {
		// Read and encode PDF file
		file, err := os.Open(pdfPath)
		if err != nil {
			log.Fatalf("Error opening PDF file: %v", err)
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			log.Fatalf("Error reading PDF file: %v", err)
		}

		// Encode to base64 with data URI format
		base64Data := base64.StdEncoding.EncodeToString(data)
		fileData := fmt.Sprintf("data:application/pdf;base64,%s", base64Data)

		// Get filename from path
		filename := pdfPath
		if lastSlash := len(pdfPath) - 1; lastSlash >= 0 {
			for i := lastSlash; i >= 0; i-- {
				if pdfPath[i] == '/' || pdfPath[i] == '\\' {
					filename = pdfPath[i+1:]
					break
				}
			}
		}

		messages = []*llm.Message{
			llm.NewUserTextMessage(prompt),
			llm.NewUserFileMessage(filename, fileData),
		}
	}

	response, err := provider.Generate(
		context.Background(),
		llm.WithMessages(messages...),
	)
	if err != nil {
		log.Fatalf("Error generating response: %v", err)
	}

	fmt.Println(response.Message().Text())
}
