package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/getstingrai/dive/llm"
	"github.com/mendableai/firecrawl-go"
)

var _ llm.Tool = &GoogleSearch{}

type FirecrawlScrapeInput struct {
	URL string `json:"url"`
}

type FirecrawlScraper struct {
	app     *firecrawl.FirecrawlApp
	maxSize int
}

func NewFirecrawlScraper(app *firecrawl.FirecrawlApp, maxSize int) *FirecrawlScraper {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &FirecrawlScraper{app: app, maxSize: maxSize}
}

func (t *FirecrawlScraper) Name() string {
	return "FirecrawlScraper"
}

func (t *FirecrawlScraper) Description() string {
	return "Retrieves the contents of the webpage at the given URL. The content will be truncated if it is overly long. If that happens, don't try to retrieve more content, just use what you have."
}

func (t *FirecrawlScraper) Definition() *llm.ToolDefinition {
	return &llm.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
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

func (t *FirecrawlScraper) Call(ctx context.Context, input string) (string, error) {
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

func (t *FirecrawlScraper) ShouldReturnResult() bool {
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
