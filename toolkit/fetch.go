package toolkit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/internal/retry"
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/deepnoodle-ai/wonton/fetch"
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

var _ dive.TypedTool[*fetch.Request] = &FetchTool{}
var _ dive.TypedToolPreviewer[*fetch.Request] = &FetchTool{}

type FetchTool struct {
	fetcher         fetch.Fetcher
	maxSize         int
	maxRetries      int
	timeout         time.Duration
	onlyMainContent bool
}

type FetchToolOptions struct {
	MaxSize         int           `json:"max_size,omitempty"`
	MaxRetries      int           `json:"max_retries,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty"`
	OnlyMainContent bool          `json:"only_main_content,omitempty"`
	Fetcher         fetch.Fetcher `json:"-"`
}

func NewFetchTool(options FetchToolOptions) *dive.TypedToolAdapter[*fetch.Request] {
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

func (t *FetchTool) PreviewCall(ctx context.Context, req *fetch.Request) *dive.ToolCallPreview {
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Fetch %s", req.URL),
	}
}

func (t *FetchTool) Call(ctx context.Context, req *fetch.Request) (*dive.ToolResult, error) {
	req.Formats = []string{"markdown"}

	if req.ExcludeTags == nil {
		req.ExcludeTags = DefaultFetchExcludeTags
	}
	if t.onlyMainContent {
		req.OnlyMainContent = true
	}

	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	var response *fetch.Response
	err := retry.Do(ctx, func() error {
		var err error
		response, err = t.fetcher.Fetch(ctx, req)
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

	content := truncateText(sb.String(), t.maxSize)
	contentLen := len([]rune(content))

	// Build display summary
	display := fmt.Sprintf("Fetched %s", req.URL)
	if title := response.Metadata.Title; title != "" {
		display = fmt.Sprintf("Fetched %s (%s)", req.URL, title)
	}
	display = fmt.Sprintf("%s - %d chars", display, contentLen)

	return NewToolResultText(content).WithDisplay(display), nil
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
