package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/retry"
	"github.com/diveagents/dive/web"
)

const (
	DefaultFetchMaxSize    = 1024 * 500 // 500k runes
	DefaultFetchMaxRetries = 2
)

var _ llm.Tool = &FetchTool{}

type FetchTool struct {
	fetcher    web.Fetcher
	maxSize    int
	maxRetries int
}

func NewFetchTool(fetcher web.Fetcher) *FetchTool {
	return &FetchTool{
		fetcher:    fetcher,
		maxSize:    DefaultFetchMaxSize,
		maxRetries: DefaultFetchMaxRetries,
	}
}

func (t *FetchTool) WithMaxSize(maxSize int) *FetchTool {
	t.maxSize = maxSize
	return t
}

func (t *FetchTool) WithMaxRetries(maxRetries int) *FetchTool {
	t.maxRetries = maxRetries
	return t
}

func (t *FetchTool) Definition() *llm.ToolDefinition {
	return &llm.ToolDefinition{
		Name:        "fetch",
		Description: "Retrieves the contents of the webpage at the given URL.",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"url"},
			Properties: map[string]*llm.SchemaProperty{
				"url": {
					Type:        "string",
					Description: "The URL of the webpage to retrieve, e.g. 'https://www.example.com'",
				},
			},
		},
	}
}

func (t *FetchTool) Call(ctx context.Context, input string) (string, error) {
	var s web.FetchInput
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		return "", err
	}

	var response *web.Document
	err := retry.Do(ctx, func() error {
		var err error
		response, err = t.fetcher.Fetch(ctx, &s)
		if err != nil {
			return err
		}
		return nil
	}, retry.WithMaxRetries(t.maxRetries))

	if err != nil {
		return fmt.Sprintf("failed to fetch url after %d attempts: %s", t.maxRetries, err), nil
	}

	var sb strings.Builder
	if response.Metadata != nil {
		metadata := *response.Metadata
		if metadata.Title != "" {
			sb.WriteString(fmt.Sprintf("# %s\n\n", metadata.Title))
		}
		if metadata.Description != "" {
			sb.WriteString(fmt.Sprintf("## %s\n\n", metadata.Description))
		}
	}
	sb.WriteString(response.Markdown)

	result := truncateText(sb.String(), t.maxSize)
	return result, nil
}

func (t *FetchTool) ShouldReturnResult() bool {
	return true
}

func truncateText(text string, maxSize int) string {
	runes := []rune(text)
	if len(runes) <= maxSize {
		return text
	}
	return string(runes[:maxSize]) + "..."
}
