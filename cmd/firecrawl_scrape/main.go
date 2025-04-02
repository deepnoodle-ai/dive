package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/diveagents/dive/toolkit/firecrawl"
)

func main() {

	var urlSpec string
	flag.StringVar(&urlSpec, "urls", "", "comma separated list of urls to scrape")
	flag.Parse()

	if urlSpec == "" {
		log.Fatalf("urls flag is required")
	}

	urls := strings.Split(urlSpec, ",")

	client, err := firecrawl.NewClient()
	if err != nil {
		log.Fatalf("failed to create firecrawl client: %v", err)
	}

	response, err := client.BatchScrape(context.Background(), urls, nil)
	if err != nil {
		log.Fatalf("failed to scrape urls: %v", err)
	}

	fmt.Printf("scraped %d urls\n", len(response.Data))

	for _, doc := range response.Data {
		log.Printf("scraped url: %s", doc.Metadata.SourceURL)
		log.Printf("scraped title: %s", doc.Metadata.Title)
		log.Printf("scraped description: %s", doc.Metadata.Description)
		log.Printf("scraped content: %s", doc.Markdown)
	}
}
