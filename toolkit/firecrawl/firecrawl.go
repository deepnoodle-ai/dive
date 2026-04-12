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

	"github.com/deepnoodle-ai/wonton/fetch"
)

var _ fetch.Fetcher = &Client{}

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
func (c *Client) Fetch(ctx context.Context, req *fetch.Request) (*fetch.Response, error) {
	// Convert wonton format strings to the names Firecrawl v2 expects.
	formats := make([]Format, 0, len(req.Formats))
	for _, format := range req.Formats {
		switch format {
		case "markdown", "html", "links", "summary":
			formats = append(formats, format)
		case "raw_html", "rawHtml":
			formats = append(formats, "rawHtml")
		}
	}
	if len(formats) == 0 {
		formats = []Format{"markdown"}
	}

	body := scrapeRequestBody{
		URL:         req.URL,
		Formats:     formats,
		Headers:     req.Headers,
		IncludeTags: req.IncludeTags,
		ExcludeTags: req.ExcludeTags,
	}
	// fetch.Request.OnlyMainContent is a bool, so we can only forward "true";
	// when false (the zero value) we omit the field and let Firecrawl apply
	// its own default (also true as of v2).
	if req.OnlyMainContent {
		onlyMain := true
		body.OnlyMainContent = &onlyMain
	}
	if req.MaxAge > 0 {
		maxAge := int64(req.MaxAge)
		body.MaxAge = &maxAge
	}
	if req.WaitFor > 0 {
		body.WaitFor = &req.WaitFor
	}
	if req.Timeout > 0 {
		body.Timeout = &req.Timeout
	}
	if req.Mobile {
		body.Mobile = &req.Mobile
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/scrape", &body)
	if err != nil {
		return nil, err
	}
	var scrapeResp scrapeResponse
	if err := json.Unmarshal(resp, &scrapeResp); err != nil {
		return nil, fmt.Errorf("failed to parse scrape response: %w", err)
	}
	if !scrapeResp.Success || scrapeResp.Data == nil {
		return nil, fmt.Errorf("scrape operation failed")
	}

	output := &fetch.Response{
		URL:       req.URL,
		Markdown:  scrapeResp.Data.Markdown,
		Timestamp: time.Now().UTC(),
	}
	if md := scrapeResp.Data.Metadata; md != nil {
		output.StatusCode = md.StatusCode
		output.Metadata = fetch.Metadata{
			Title:       md.Title,
			Description: md.Description,
			Keywords:    []string(md.Keywords),
			Canonical:   md.SourceURL,
		}
	}

	// Set optional response fields
	if scrapeResp.Data.Summary != nil {
		output.Summary = *scrapeResp.Data.Summary
	}
	if scrapeResp.Data.HTML != nil {
		output.HTML = *scrapeResp.Data.HTML
	}
	if scrapeResp.Data.RawHTML != nil {
		output.RawHTML = *scrapeResp.Data.RawHTML
	}
	if scrapeResp.Data.Screenshot != nil {
		output.Screenshot = *scrapeResp.Data.Screenshot
	}
	// Convert links from []string to []fetch.Link
	if scrapeResp.Data.Links != nil {
		output.Links = make([]fetch.Link, len(scrapeResp.Data.Links))
		for i, link := range scrapeResp.Data.Links {
			output.Links[i] = fetch.Link{URL: link}
		}
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
		switch resp.StatusCode {
		case 400:
			return nil, fetch.NewRequestErrorf("bad request: %s", respBody).WithStatusCode(400)
		case 401:
			return nil, fetch.NewRequestErrorf("unauthorized: invalid or missing API key").WithStatusCode(401)
		case 402:
			return nil, fetch.NewRequestErrorf("payment required to access this resource").WithStatusCode(402)
		case 404:
			return nil, fetch.NewRequestErrorf("resource not found").WithStatusCode(404)
		case 429:
			return nil, fetch.NewRequestErrorf("request rate limit exceeded, please wait and try again later").WithStatusCode(429)
		case 500, 502, 503, 504:
			return nil, fetch.NewRequestErrorf("server error occurred").WithStatusCode(resp.StatusCode)
		default:
			return nil, fetch.NewRequestErrorf("request failed: %s", respBody).WithStatusCode(resp.StatusCode)
		}
	}
	return respBody, nil
}
