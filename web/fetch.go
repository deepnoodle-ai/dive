package web

import (
	"context"
	"fmt"

	"github.com/deepnoodle-ai/dive/schema"
)

// FetchFormat specifies a format that should be returned when
// fetching a web page.
type FetchFormat string

const (
	// FetchFormatMarkdown indicates that the page should be converted
	// to markdown and that should be returned.
	FetchFormatMarkdown FetchFormat = "markdown"

	// FetchFormatHTML indicates that the page should be returned as an
	// HTML string.
	FetchFormatHTML FetchFormat = "html"

	// FetchFormatSummary indicates that the page should be returned as a
	// summary of the page.
	FetchFormatSummary FetchFormat = "summary"

	// FetchFormatLinks indicates that a list of links on the page should
	// be included in the response.
	FetchFormatLinks FetchFormat = "links"
)

// PageMetadata contains metadata about a web page. This intentionally
// aligns with Firecrawl's scrape API response.
type PageMetadata struct {
	Title             string   `json:"title,omitempty"`
	Description       string   `json:"description,omitempty"`
	Language          string   `json:"language,omitempty"`
	Keywords          string   `json:"keywords,omitempty"`
	Robots            string   `json:"robots,omitempty"`
	OGTitle           string   `json:"og_title,omitempty"`
	OGDescription     string   `json:"og_description,omitempty"`
	OGURL             string   `json:"og_url,omitempty"`
	OGImage           string   `json:"og_image,omitempty"`
	OGLocaleAlternate []string `json:"og_locale_alternate,omitempty"`
	OGSiteName        string   `json:"og_site_name,omitempty"`
	SourceURL         string   `json:"source_url,omitempty"`
	StatusCode        int      `json:"status_code,omitempty"`
}

// Location is used to configure a proxy location for the fetch operation.
type Location struct {
	Country   string   `json:"country,omitempty"`
	Languages []string `json:"languages,omitempty"`
}

// Viewport is used to configure the viewport for the screenshot.
type Viewport struct {
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// ScreenshotOptions is used to configure the screenshot options.
type ScreenshotOptions struct {
	FullPage bool     `json:"full_page,omitempty"`
	Quality  int      `json:"quality,omitempty"`
	Viewport Viewport `json:"viewport,omitempty"`
}

// JSONSchemaOptions is used to specify that a JSON schema should be used to
// extract structured data from the page.
type JSONSchemaOptions struct {
	Schema *schema.Schema `json:"schema,omitempty"`
	Prompt string         `json:"prompt,omitempty"`
}

// FetchInput contains parameters for fetching a web page. This intentionally
// aligns _mostly_ with Firecrawl's scrape API v2 endpoint:
// https://docs.firecrawl.dev/features/scrape
type FetchInput struct {
	URL             string             `json:"url"`
	Formats         []FetchFormat      `json:"formats,omitempty"`
	Headers         map[string]string  `json:"headers,omitempty"`
	IncludeTags     []string           `json:"include_tags,omitempty"`
	ExcludeTags     []string           `json:"exclude_tags,omitempty"`
	Timeout         int                `json:"timeout,omitempty"`
	OnlyMainContent *bool              `json:"only_main_content,omitempty"`
	MaxAge          *int64             `json:"max_age,omitempty"`
	StoreInCache    *bool              `json:"store_in_cache,omitempty"`
	WaitFor         *int               `json:"wait_for,omitempty"`
	Screenshot      *ScreenshotOptions `json:"screenshot,omitempty"`
	JSONSchema      *JSONSchemaOptions `json:"json_schema,omitempty"`
}

// FetchOutput contains the result of a web page fetch operation.
type FetchOutput struct {
	Markdown   string       `json:"markdown,omitempty"`
	HTML       string       `json:"html,omitempty"`
	Summary    string       `json:"summary,omitempty"`
	Screenshot string       `json:"screenshot,omitempty"`
	Links      []string     `json:"links,omitempty"`
	Warning    string       `json:"warning,omitempty"`
	Metadata   PageMetadata `json:"metadata"`
}

// Fetcher defines the interface for fetching web pages. Not all Fetcher
// implementations support all options. All fetchers SHOULD support the
// basic "fetch this URL as markdown" functionality.
type Fetcher interface {
	Fetch(ctx context.Context, input *FetchInput) (*FetchOutput, error)
}

// FetchError contains information about a fetch error.
type FetchError struct {
	StatusCode int
	Err        error
}

// NewFetchError creates a new FetchError with the given status code and error.
func NewFetchError(statusCode int, err error) *FetchError {
	return &FetchError{StatusCode: statusCode, Err: err}
}

func (e *FetchError) Error() string {
	return fmt.Sprintf("fetch failed with status code %d: %s", e.StatusCode, e.Err)
}

func (e *FetchError) Unwrap() error {
	return e.Err
}

func (e *FetchError) IsRecoverable() bool {
	return e.StatusCode == 429 || // Too Many Requests
		e.StatusCode == 500 || // Internal Server Error
		e.StatusCode == 502 || // Bad Gateway
		e.StatusCode == 503 || // Service Unavailable
		e.StatusCode == 504 // Gateway Timeout
}
