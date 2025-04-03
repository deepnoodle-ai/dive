package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/diveagents/dive/toolkit/firecrawl"
	"github.com/diveagents/dive/web"
)

func main() {

	var url string
	flag.StringVar(&url, "url", "", "url to scrape")
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

	fmt.Printf("scraped url: %s\n", response.Metadata.URL)
	fmt.Printf("scraped title: %s\n", response.Metadata.Title)
	fmt.Printf("scraped description: %s\n", response.Metadata.Description)
	fmt.Printf("scraped content: %s\n", response.Markdown)
}
