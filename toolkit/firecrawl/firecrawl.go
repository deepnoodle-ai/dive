// Package firecrawl provides a client for interacting with the Firecrawl API.
package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/deepnoodle-ai/dive/web"
)

var _ web.Fetcher = &Client{}

// ClientOption is a function that modifies the client configuration.
type ClientOption func(*Client)

// WithAPIKey sets the API key for the client.
func WithAPIKey(apiKey string) ClientOption {
	return func(c *Client) {
		c.apiKey = apiKey
	}
}

// WithBaseURL sets the base URL for the client.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		c.baseURL = baseURL
	}
}

// WithHTTPClient sets the HTTP client for the client.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithTimeout sets the timeout for the default HTTP client.
// This option is ignored if a custom HTTP client is provided.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		if c.httpClient == http.DefaultClient {
			c.httpClient = &http.Client{
				Timeout: timeout,
			}
		}
	}
}

// Client represents a Firecrawl API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates a new Firecrawl client with the provided options.
func New(opts ...ClientOption) (*Client, error) {
	c := &Client{
		apiKey:  os.Getenv("FIRECRAWL_API_KEY"),
		baseURL: "https://api.firecrawl.dev/v2",
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	if envURL := os.Getenv("FIRECRAWL_API_URL"); envURL != "" {
		c.baseURL = envURL
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("no api key provided")
	}
	return c, nil
}

// Fetch a web page.
func (c *Client) Fetch(ctx context.Context, input *web.FetchInput) (*web.FetchOutput, error) {
	// Convert web.FetchFormat to Firecrawl v2 Format objects
	formats := make([]Format, len(input.Formats))
	for i, format := range input.Formats {
		switch format {
		case web.FetchFormatMarkdown:
			formats[i] = "markdown"
		case web.FetchFormatHTML:
			formats[i] = "html"
		case web.FetchFormatSummary:
			formats[i] = "summary"
		case web.FetchFormatLinks:
			formats[i] = "links"
		}
	}

	// Default to markdown if no formats specified
	if len(formats) == 0 {
		formats = []Format{"markdown"}
	}

	body := scrapeRequestBody{
		URL:         input.URL,
		Formats:     formats,
		Headers:     input.Headers,
		IncludeTags: input.IncludeTags,
		ExcludeTags: input.ExcludeTags,
	}

	// Set optional fields from input
	if input.OnlyMainContent != nil {
		body.OnlyMainContent = input.OnlyMainContent
	}
	if input.MaxAge != nil {
		body.MaxAge = input.MaxAge
	}
	if input.StoreInCache != nil {
		body.StoreInCache = input.StoreInCache
	}
	if input.WaitFor != nil {
		body.WaitFor = input.WaitFor
	}
	if input.Timeout > 0 {
		timeout := input.Timeout
		body.Timeout = &timeout
	}

	// Handle screenshot format if specified
	if input.Screenshot != nil {
		screenshotFormat := ScreenshotFormat{
			Type:     "screenshot",
			FullPage: &input.Screenshot.FullPage,
		}
		if input.Screenshot.Quality > 0 {
			screenshotFormat.Quality = &input.Screenshot.Quality
		}
		if input.Screenshot.Viewport.Width > 0 && input.Screenshot.Viewport.Height > 0 {
			screenshotFormat.Viewport = &Viewport{
				Width:  input.Screenshot.Viewport.Width,
				Height: input.Screenshot.Viewport.Height,
			}
		}
		body.Formats = append(body.Formats, screenshotFormat)
	}

	// Handle JSON schema extraction if specified
	if input.JSONSchema != nil {
		jsonFormat := JSONFormat{
			Type:   "json",
			Schema: input.JSONSchema.Schema,
			Prompt: input.JSONSchema.Prompt,
		}
		body.Formats = append(body.Formats, jsonFormat)
	}

	// Set v2 specific defaults
	if body.OnlyMainContent == nil {
		onlyMain := true
		body.OnlyMainContent = &onlyMain
	}
	if body.RemoveBase64Images == nil {
		removeImages := true
		body.RemoveBase64Images = &removeImages
	}
	if body.BlockAds == nil {
		blockAds := true
		body.BlockAds = &blockAds
	}
	if body.Proxy == nil {
		proxy := "auto"
		body.Proxy = &proxy
	}
	if body.StoreInCache == nil {
		storeCache := true
		body.StoreInCache = &storeCache
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/scrape", &body)
	if err != nil {
		// Check if it's a FetchError and return it directly for proper error handling
		if fetchErr, ok := err.(*web.FetchError); ok {
			return nil, fetchErr
		}
		return nil, fmt.Errorf("scrape request failed: %w", err)
	}
	var scrapeResp scrapeResponse
	if err := json.Unmarshal(resp, &scrapeResp); err != nil {
		return nil, fmt.Errorf("failed to parse scrape response: %w", err)
	}
	if !scrapeResp.Success {
		return nil, fmt.Errorf("scrape operation failed")
	}

	// Build the response
	output := &web.FetchOutput{
		Markdown: scrapeResp.Data.Markdown,
		Metadata: web.PageMetadata{
			SourceURL:   scrapeResp.Data.Metadata.SourceURL,
			Title:       scrapeResp.Data.Metadata.Title,
			Description: scrapeResp.Data.Metadata.Description,
			StatusCode:  scrapeResp.Data.Metadata.StatusCode,
		},
	}

	// Set optional response fields
	if scrapeResp.Data.Summary != nil {
		output.Summary = *scrapeResp.Data.Summary
	}
	if scrapeResp.Data.HTML != nil {
		output.HTML = *scrapeResp.Data.HTML
	}
	if scrapeResp.Data.Screenshot != nil {
		output.Screenshot = *scrapeResp.Data.Screenshot
	}
	if scrapeResp.Data.Links != nil {
		output.Links = scrapeResp.Data.Links
	}
	if scrapeResp.Data.Warning != nil {
		output.Warning = *scrapeResp.Data.Warning
	}
	if scrapeResp.Data.Metadata.Language != nil {
		output.Metadata.Language = *scrapeResp.Data.Metadata.Language
	}

	return output, nil
}

// doRequest performs an HTTP request to the Firecrawl API.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Handle specific v2 API error responses
		switch resp.StatusCode {
		case 402:
			return nil, &web.FetchError{StatusCode: 402, Err: fmt.Errorf("payment required to access this resource")}
		case 429:
			return nil, &web.FetchError{StatusCode: 429, Err: fmt.Errorf("request rate limit exceeded, please wait and try again later")}
		case 500:
			return nil, &web.FetchError{StatusCode: 500, Err: fmt.Errorf("server error occurred")}
		default:
			return nil, &web.FetchError{StatusCode: resp.StatusCode, Err: fmt.Errorf("request failed: %s", respBody)}
		}
	}
	return respBody, nil
}
