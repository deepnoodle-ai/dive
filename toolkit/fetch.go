package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/retry"
	"github.com/diveagents/dive/web"
)

const (
	DefaultFetchMaxSize    = 1024 * 500 // 500k runes
	DefaultFetchMaxRetries = 1
	DefaultFetchTimeout    = 15 * time.Second
)

var DefaultFetchExcludeTags = []string{
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

var _ llm.ToolWithMetadata = &FetchTool{}

type FetchTool struct {
	fetcher    web.Fetcher
	maxSize    int
	maxRetries int
	timeout    time.Duration
}

func NewFetchTool(fetcher web.Fetcher) *FetchTool {
	return &FetchTool{
		fetcher:    fetcher,
		maxSize:    DefaultFetchMaxSize,
		maxRetries: DefaultFetchMaxRetries,
		timeout:    DefaultFetchTimeout,
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

func (t *FetchTool) WithTimeout(timeout time.Duration) *FetchTool {
	t.timeout = timeout
	return t
}

func (t *FetchTool) Name() string {
	return "fetch"
}

func (t *FetchTool) Description() string {
	return "Retrieves the contents of the webpage at the given URL."
}

func (t *FetchTool) Schema() llm.Schema {
	return llm.Schema{
		Type:     "object",
		Required: []string{"url"},
		Properties: map[string]*llm.SchemaProperty{
			"url": {
				Type:        "string",
				Description: "The URL of the webpage to retrieve, e.g. 'https://www.example.com'",
			},
		},
	}
}

func (t *FetchTool) Call(ctx context.Context, input *llm.ToolCallInput) (*llm.ToolCallOutput, error) {
	var s web.FetchInput
	if err := json.Unmarshal([]byte(input.Input), &s); err != nil {
		return nil, err
	}

	s.Formats = []string{"markdown"}

	if s.ExcludeTags == nil {
		s.ExcludeTags = DefaultFetchExcludeTags
	}

	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
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
		return llm.NewToolCallOutput(fmt.Sprintf("failed to fetch url after %d attempts: %s", t.maxRetries, err)), nil
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
	return llm.NewToolCallOutput(result), nil
}

func (t *FetchTool) Metadata() llm.ToolMetadata {
	return llm.ToolMetadata{
		Version:    "0.0.1",
		Capability: llm.ToolCapabilityReadOnly,
	}
}

func truncateText(text string, maxSize int) string {
	runes := []rune(text)
	if len(runes) <= maxSize {
		return text
	}
	return string(runes[:maxSize]) + "..."
}
