package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diveagents/dive/llm"
	"github.com/mendableai/firecrawl-go"
)

const DefaultFirecrawlScrapeMaxSize = 1024 * 200 // 200KB

var _ llm.Tool = &GoogleSearch{}

type FirecrawlScrapeInput struct {
	URL string `json:"url"`
}

type FirecrawlScrapeToolOptions struct {
	App     *firecrawl.FirecrawlApp `json:"-"`
	MaxSize int                     `json:"max_size,omitempty"`
}

type FirecrawlScrapeTool struct {
	app     *firecrawl.FirecrawlApp
	maxSize int
}

func NewFirecrawlScrapeTool(options FirecrawlScrapeToolOptions) *FirecrawlScrapeTool {
	if options.MaxSize <= 0 {
		options.MaxSize = DefaultFirecrawlScrapeMaxSize
	}
	if options.App == nil {
		panic("firecrawl app is required")
	}
	return &FirecrawlScrapeTool{app: options.App, maxSize: options.MaxSize}
}

func (t *FirecrawlScrapeTool) Definition() *llm.ToolDefinition {
	return &llm.ToolDefinition{
		Name:        "firecrawl_scrape",
		Description: "Retrieves the contents of the webpage at the given URL. The content will be truncated if it is overly long. If that happens, don't try to retrieve more content, just use what you have.",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"url"},
			Properties: map[string]*llm.SchemaProperty{
				"url": {
					Type:        "string",
					Description: "The URL of the webpage to retrieve, e.g. 'https://www.google.com'",
				},
			},
		},
	}
}

func (t *FirecrawlScrapeTool) Call(ctx context.Context, input string) (string, error) {
	var s FirecrawlScrapeInput
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		return "", err
	}
	if strings.HasSuffix(s.URL, ".pdf") {
		return "PDFs are not supported by this tool currently.", nil
	}

	response, err := t.app.ScrapeURL(s.URL, &firecrawl.ScrapeParams{
		Timeout:         ptr(15000),
		OnlyMainContent: ptr(true),
		Formats:         []string{"markdown"},
		ExcludeTags:     []string{"script", "style", "a", "img", "iframe"},
	})
	if err != nil {
		if strings.Contains(err.Error(), "403") {
			return "Scraping this website is not supported.", nil
		}
		return "", err
	}

	// Capture the page content using a string builder. We'll truncate this
	// later if it's oversized.
	var sb strings.Builder
	if metadata := response.Metadata; metadata != nil {
		if title := metadata.Title; title != nil {
			sb.WriteString(fmt.Sprintf("# Title: %s\n", *title))
		}
		if description := metadata.Description; description != nil {
			sb.WriteString(fmt.Sprintf("# Description: %s\n\n", *description))
		}
	}
	sb.WriteString(response.Markdown)

	truncatedPage := truncateText(sb.String(), t.maxSize)

	// Wrap the page content so it's clear where the content begins and ends
	text := fmt.Sprintf("<webpage url=\"%s\">\n", s.URL)
	text += truncatedPage
	text += "\n</webpage>"

	return text, nil
}

func (t *FirecrawlScrapeTool) ShouldReturnResult() bool {
	return true
}

func ptr[T any](v T) *T {
	return &v
}

func truncateText(text string, maxSize int) string {
	if len(text) <= maxSize {
		return text
	}
	return text[:maxSize] + "..."
}
