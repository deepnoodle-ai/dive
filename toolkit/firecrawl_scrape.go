package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/retry"
	"github.com/mendableai/firecrawl-go"
)

const (
	DefaultFirecrawlScrapeMaxSize = 1024 * 200 // 200KB
	DefaultMaxRetries             = 2
)

var _ llm.Tool = &GoogleSearch{}

type FirecrawlScrapeInput struct {
	URL string `json:"url"`
}

type FirecrawlScrapeToolOptions struct {
	App        *firecrawl.FirecrawlApp `json:"-"`
	MaxSize    int                     `json:"max_size,omitempty"`
	MaxRetries int                     `json:"max_retries,omitempty"`
}

type FirecrawlScrapeTool struct {
	app        *firecrawl.FirecrawlApp
	maxSize    int
	maxRetries int
}

// scrapeError represents an error that occurred during scraping
type scrapeError struct {
	err        error
	statusCode int
}

func (e *scrapeError) Error() string {
	return e.err.Error()
}

func (e *scrapeError) IsRecoverable() bool {
	// Consider rate limits and server errors as recoverable
	return e.statusCode == 429 || // Too Many Requests
		e.statusCode == 500 || // Internal Server Error
		e.statusCode == 502 || // Bad Gateway
		e.statusCode == 503 || // Service Unavailable
		e.statusCode == 504 // Gateway Timeout
}

func NewFirecrawlScrapeTool(options FirecrawlScrapeToolOptions) *FirecrawlScrapeTool {
	if options.MaxSize <= 0 {
		options.MaxSize = DefaultFirecrawlScrapeMaxSize
	}
	if options.MaxRetries <= 0 {
		options.MaxRetries = DefaultMaxRetries
	}
	if options.App == nil {
		panic("firecrawl app is required")
	}
	return &FirecrawlScrapeTool{
		app:        options.App,
		maxSize:    options.MaxSize,
		maxRetries: options.MaxRetries,
	}
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

	var response *firecrawl.FirecrawlDocument
	err := retry.Do(ctx, func() error {
		var err error
		response, err = t.app.ScrapeURL(s.URL, &firecrawl.ScrapeParams{
			Timeout: ptr(15000),
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
			if strings.Contains(err.Error(), "403") {
				return err // Non-recoverable error
			}
			// Check if error contains a status code
			var statusCode int
			if strings.Contains(err.Error(), "429") {
				statusCode = 429
			} else if strings.Contains(err.Error(), "500") {
				statusCode = 500
			} else if strings.Contains(err.Error(), "502") {
				statusCode = 502
			} else if strings.Contains(err.Error(), "503") {
				statusCode = 503
			} else if strings.Contains(err.Error(), "504") {
				statusCode = 504
			}
			if statusCode > 0 {
				return &scrapeError{err: err, statusCode: statusCode}
			}
			if strings.Contains(err.Error(), "timeout") {
				return &scrapeError{err: err, statusCode: 504}
			}
			return err
		}
		return nil
	}, retry.WithMaxRetries(t.maxRetries))

	if err != nil {
		return fmt.Sprintf("Unable to scrape URL after %d attempts: %s", t.maxRetries, err), nil
	}

	// Capture the page content using a string builder. We'll truncate this
	// later if it's oversized.
	var sb strings.Builder
	if response.Metadata != nil {
		metadata := *response.Metadata
		if metadata.Title != nil && *metadata.Title != "" {
			sb.WriteString(fmt.Sprintf("# Title: %s\n", *metadata.Title))
		}
		if metadata.Description != nil && *metadata.Description != "" {
			sb.WriteString(fmt.Sprintf("# Description: %s\n\n", *metadata.Description))
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
