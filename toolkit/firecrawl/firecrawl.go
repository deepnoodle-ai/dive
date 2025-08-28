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
		baseURL: "https://api.firecrawl.dev",
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
func (c *Client) Fetch(ctx context.Context, input *web.FetchInput) (*web.Document, error) {
	body := scrapeRequestBody{
		URL:             input.URL,
		Formats:         input.Formats,
		Headers:         input.Headers,
		IncludeTags:     input.IncludeTags,
		ExcludeTags:     input.ExcludeTags,
		OnlyMainContent: input.OnlyMainContent,
		Timeout:         30000,
	}
	resp, err := c.doRequest(ctx, http.MethodPost, "/v1/scrape", &body)
	if err != nil {
		return nil, fmt.Errorf("scrape request failed: %w", err)
	}
	var scrapeResp scrapeResponse
	if err := json.Unmarshal(resp, &scrapeResp); err != nil {
		return nil, fmt.Errorf("failed to parse scrape response: %w", err)
	}
	if !scrapeResp.Success {
		return nil, fmt.Errorf("scrape operation failed")
	}
	return &web.Document{
		Content:    scrapeResp.Data.HTML,
		Markdown:   scrapeResp.Data.Markdown,
		Screenshot: scrapeResp.Data.Screenshot,
		Links:      scrapeResp.Data.Links,
		Metadata: &web.DocumentMetadata{
			URL:         scrapeResp.Data.Metadata.SourceURL,
			Title:       scrapeResp.Data.Metadata.Title,
			Description: scrapeResp.Data.Metadata.Description,
			Language:    scrapeResp.Data.Metadata.Language,
			Keywords:    scrapeResp.Data.Metadata.Keywords,
		},
	}, nil
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
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, respBody)
	}
	return respBody, nil
}
