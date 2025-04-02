package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/toolkit/firecrawl"
)

var _ llm.Tool = &FirecrawlBatchScrapeTool{}

type FirecrawlBatchScrapeInput struct {
	URLs []string `json:"urls"`
}

type FirecrawlBatchScrapeToolOptions struct {
	Client     *firecrawl.Client `json:"-"`
	MaxSize    int               `json:"max_size,omitempty"`
	MaxRetries int               `json:"max_retries,omitempty"`
}

type FirecrawlBatchScrapeTool struct {
	client     *firecrawl.Client
	maxSize    int
	maxRetries int
}

func NewFirecrawlBatchScrapeTool(options FirecrawlBatchScrapeToolOptions) *FirecrawlBatchScrapeTool {
	if options.MaxSize <= 0 {
		options.MaxSize = DefaultFirecrawlScrapeMaxSize
	}
	if options.MaxRetries <= 0 {
		options.MaxRetries = DefaultMaxRetries
	}
	if options.Client == nil {
		panic("firecrawl client is required")
	}
	return &FirecrawlBatchScrapeTool{
		client:     options.Client,
		maxSize:    options.MaxSize,
		maxRetries: options.MaxRetries,
	}
}

func (t *FirecrawlBatchScrapeTool) Definition() *llm.ToolDefinition {
	return &llm.ToolDefinition{
		Name:        "firecrawl_batch_scrape",
		Description: "Retrieves the contents of the webpages at the given URLs. The content will be truncated if it is overly long. If that happens, don't try to retrieve more content, just use what you have.",
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
		Timeout: 15000,
		Formats: []string{"markdown"},
		ExcludeTags: []string{
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
		},
	})
	if err != nil {
		return "", err
	}

	// Create a structured output with metadata and content sections
	var sb strings.Builder

	// Write metadata section
	sb.WriteString("<scrape-metadata>\n")
	metadataMap := map[string]interface{}{
		"total_pages":  len(response.Data),
		"status":       response.Status,
		"credits_used": response.CreditsUsed,
		"source_urls":  s.URLs,
	}
	metadataJSON, err := json.MarshalIndent(metadataMap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %w", err)
	}
	sb.Write(metadataJSON)
	sb.WriteString("\n</scrape-metadata>\n\n")
	sb.WriteString("---\n\n")

	// Write each page's content
	for i, doc := range response.Data {
		if doc.Metadata == nil {
			continue
		}
		// Page metadata section
		sb.WriteString(fmt.Sprintf("<page-metadata url=%q>\n", doc.Metadata.SourceURL))
		pageMetadata := map[string]interface{}{
			"title":       doc.Metadata.Title,
			"description": doc.Metadata.Description,
			"language":    doc.Metadata.Language,
			"status_code": doc.Metadata.StatusCode,
		}
		if doc.Metadata.Error != "" {
			pageMetadata["error"] = doc.Metadata.Error
		}
		pageMetadataJSON, err := json.MarshalIndent(pageMetadata, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal page metadata: %w", err)
		}
		sb.Write(pageMetadataJSON)
		sb.WriteString("\n</page-metadata>\n\n")

		// Page content section
		sb.WriteString("<page-content>\n")
		if doc.Markdown != "" {
			sb.WriteString(doc.Markdown)
		} else {
			sb.WriteString("No content available")
		}
		sb.WriteString("\n</page-content>\n\n")

		// Add separator between pages except for the last one
		if i < len(response.Data)-1 {
			sb.WriteString("---\n\n")
		}
	}

	os.WriteFile("firecrawl_batch_scrape.json", []byte(sb.String()), 0644)

	return sb.String(), nil
}

func (t *FirecrawlBatchScrapeTool) ShouldReturnResult() bool {
	return true
}
