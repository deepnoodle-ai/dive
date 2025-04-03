package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/toolkit/firecrawl"
	"github.com/goccy/go-yaml"
)

var _ llm.Tool = &FirecrawlBatchScrapeTool{}

var DefaultFirecrawlExcludeTags = []string{
	"script",
	"style",
	"hr",
	"noscript",
	"iframe",
	"select",
	"input",
	"button",
	"svg",
	"form",
	"header",
	"nav",
	"footer",
}

type FirecrawlBatchScrapeInput struct {
	URLs []string `json:"urls"`
}

type FirecrawlBatchScrapeToolOptions struct {
	Client      *firecrawl.Client `json:"-"`
	MaxSize     int               `json:"max_size,omitempty"`
	MaxRetries  int               `json:"max_retries,omitempty"`
	ExcludeTags []string          `json:"exclude_tags,omitempty"`
}

type FirecrawlBatchScrapeTool struct {
	client      *firecrawl.Client
	maxSize     int
	maxRetries  int
	excludeTags []string
}

func NewFirecrawlBatchScrapeTool(options FirecrawlBatchScrapeToolOptions) *FirecrawlBatchScrapeTool {
	if options.MaxSize <= 0 {
		options.MaxSize = 500 * 1024
	}
	if options.MaxRetries <= 0 {
		options.MaxRetries = 2
	}
	if options.Client == nil {
		panic("firecrawl client is required")
	}
	if options.ExcludeTags == nil {
		options.ExcludeTags = DefaultFirecrawlExcludeTags
	}
	return &FirecrawlBatchScrapeTool{
		client:      options.Client,
		maxSize:     options.MaxSize,
		maxRetries:  options.MaxRetries,
		excludeTags: options.ExcludeTags,
	}
}

func (t *FirecrawlBatchScrapeTool) Definition() *llm.ToolDefinition {
	return &llm.ToolDefinition{
		Name:        "firecrawl_batch_scrape",
		Description: "Efficiently scrapes and retrieves content from multiple webpages simultaneously. This tool accepts a list of URLs and returns their contents in markdown format, along with metadata such as title, description, and status code. Use this when you need to gather information from several web sources at once, as it's significantly faster than sequential scraping.",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"urls"},
			Properties: map[string]*llm.SchemaProperty{
				"urls": {
					Type:        "array",
					Description: "The URLs of the webpages to retrieve, e.g. ['https://www.google.com', 'https://www.facebook.com']",
				},
			},
		},
	}
}

func (t *FirecrawlBatchScrapeTool) Call(ctx context.Context, input string) (string, error) {
	var s FirecrawlBatchScrapeInput
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		return "", err
	}
	if len(s.URLs) == 0 {
		return "", fmt.Errorf("no urls were provided")
	}
	response, err := t.client.BatchScrape(ctx, s.URLs, &firecrawl.ScrapeOptions{
		Timeout:     15000,
		Formats:     []string{"markdown"},
		ExcludeTags: t.excludeTags,
	})
	if err != nil {
		return "", err
	}

	// Create a structured output with metadata and content sections
	var sb strings.Builder

	// Write each page's content
	for i, doc := range response.Data {
		if doc.Metadata == nil {
			continue
		}
		// Page metadata section
		pageMetadata := map[string]interface{}{
			"url":         doc.Metadata.SourceURL,
			"title":       doc.Metadata.Title,
			"description": doc.Metadata.Description,
			"keywords":    doc.Metadata.Keywords,
			"language":    doc.Metadata.Language,
			"status_code": doc.Metadata.StatusCode,
		}
		if doc.Metadata.Error != "" {
			pageMetadata["error"] = doc.Metadata.Error
		}
		pageMetadataYAML, err := yaml.Marshal(pageMetadata)
		if err != nil {
			return "", fmt.Errorf("failed to marshal page metadata to YAML: %w", err)
		}
		sb.Write(pageMetadataYAML)
		sb.WriteString("\n---\n\n")
		sb.WriteString("# " + doc.Metadata.Title + "\n\n")
		if doc.Markdown != "" {
			sb.WriteString(doc.Markdown)
		} else {
			sb.WriteString("No content available")
		}
		// Add separator between pages except for the last one
		if i < len(response.Data)-1 {
			sb.WriteString("---\n\n")
		}
	}
	return sb.String(), nil
}

func (t *FirecrawlBatchScrapeTool) ShouldReturnResult() bool {
	return true
}
