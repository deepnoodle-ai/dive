package toolkit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/internal/retry"
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/deepnoodle-ai/dive/web"
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

var _ dive.TypedTool[*web.FetchInput] = &FetchTool{}

type FetchTool struct {
	fetcher         web.Fetcher
	maxSize         int
	maxRetries      int
	timeout         time.Duration
	onlyMainContent *bool
	storeInCache    *bool
	maxAge          *int64
}

type FetchToolOptions struct {
	MaxSize         int           `json:"max_size,omitempty"`
	MaxRetries      int           `json:"max_retries,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty"`
	StoreInCache    *bool         `json:"store_in_cache,omitempty"`
	MaxAge          *int64        `json:"max_age,omitempty"`
	OnlyMainContent *bool         `json:"only_main_content,omitempty"`
	Fetcher         web.Fetcher   `json:"-"`
}

func NewFetchTool(options FetchToolOptions) *dive.TypedToolAdapter[*web.FetchInput] {
	if options.MaxSize <= 0 {
		options.MaxSize = DefaultFetchMaxSize
	}
	if options.Timeout <= 0 {
		options.Timeout = DefaultFetchTimeout
	}
	return dive.ToolAdapter(&FetchTool{
		fetcher:         options.Fetcher,
		maxSize:         options.MaxSize,
		maxRetries:      options.MaxRetries,
		timeout:         options.Timeout,
		onlyMainContent: options.OnlyMainContent,
		storeInCache:    options.StoreInCache,
		maxAge:          options.MaxAge,
	})
}

func (t *FetchTool) Name() string {
	return "fetch"
}

func (t *FetchTool) Description() string {
	return "Fetches the contents of the webpage at the given URL."
}

func (t *FetchTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"url"},
		Properties: map[string]*schema.Property{
			"url": {
				Type:        "string",
				Description: "The URL of the webpage to fetch, e.g. https://www.example.com",
			},
		},
	}
}

func (t *FetchTool) Call(ctx context.Context, input *web.FetchInput) (*dive.ToolResult, error) {
	input.Formats = []web.FetchFormat{web.FetchFormatMarkdown}

	if input.ExcludeTags == nil {
		input.ExcludeTags = DefaultFetchExcludeTags
	}
	if t.onlyMainContent != nil {
		input.OnlyMainContent = t.onlyMainContent
	}
	if t.storeInCache != nil {
		input.StoreInCache = t.storeInCache
	}
	if t.maxAge != nil {
		input.MaxAge = t.maxAge
	}

	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	var response *web.FetchOutput
	err := retry.Do(ctx, func() error {
		var err error
		response, err = t.fetcher.Fetch(ctx, input)
		if err != nil {
			return err
		}
		return nil
	}, retry.WithMaxRetries(t.maxRetries))

	if err != nil {
		return NewToolResultError(fmt.Sprintf("failed to fetch url after %d attempts: %s", t.maxRetries, err)), nil
	}

	var sb strings.Builder
	if value := response.Metadata.Title; value != "" {
		sb.WriteString(fmt.Sprintf("# %s\n\n", value))
	}
	if value := response.Metadata.Description; value != "" {
		sb.WriteString(fmt.Sprintf("## %s\n\n", value))
	}
	sb.WriteString(response.Markdown)

	result := truncateText(sb.String(), t.maxSize)
	return NewToolResultText(result), nil
}

func (t *FetchTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Fetch",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   true,
	}
}

func truncateText(text string, maxSize int) string {
	runes := []rune(text)
	if len(runes) <= maxSize {
		return text
	}
	return string(runes[:maxSize]) + "..."
}
