package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/deepnoodle-ai/dive/toolkit/firecrawl"
	"github.com/deepnoodle-ai/dive/web"
)

func main() {
	var url, outFile string
	flag.StringVar(&url, "url", "", "url to scrape")
	flag.StringVar(&outFile, "out", "", "file to write output to")
	flag.Parse()

	if url == "" {
		log.Fatalf("url flag is required")
	}

	client, err := firecrawl.New()
	if err != nil {
		log.Fatalf("failed to create firecrawl client: %v", err)
	}

	response, err := client.Fetch(context.Background(), &web.FetchInput{
		URL: url,
	})
	if err != nil {
		log.Fatalf("failed to scrape url: %v", err)
	}

	fmt.Printf("scraped title: %s\n", response.Metadata.Title)
	fmt.Printf("scraped description: %s\n", response.Metadata.Description)
	fmt.Printf("scraped content: %s\n", response.Markdown)

	if outFile != "" {
		responseJSON, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			log.Fatalf("failed to marshal response: %v", err)
		}
		err = os.WriteFile(outFile, responseJSON, 0644)
		if err != nil {
			log.Fatalf("failed to write output to file: %v", err)
		}
		fmt.Printf("wrote response to file: %s\n", outFile)
	}
}
