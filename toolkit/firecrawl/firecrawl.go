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
)

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

// Document represents a scraped document from Firecrawl.
type Document struct {
	Markdown   string            `json:"markdown,omitempty"`
	HTML       string            `json:"html,omitempty"`
	RawHTML    string            `json:"rawHtml,omitempty"`
	Screenshot string            `json:"screenshot,omitempty"`
	Links      []string          `json:"links,omitempty"`
	Metadata   *DocumentMetadata `json:"metadata,omitempty"`
}

// DocumentMetadata contains metadata about a scraped document.
type DocumentMetadata struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Language    string `json:"language,omitempty"`
	Keywords    string `json:"keywords,omitempty"`
	SourceURL   string `json:"sourceURL,omitempty"`
	StatusCode  int    `json:"statusCode,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ScrapeOptions configures how a URL should be scraped.
type ScrapeOptions struct {
	Formats         []string          `json:"formats,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	IncludeTags     []string          `json:"includeTags,omitempty"`
	ExcludeTags     []string          `json:"excludeTags,omitempty"`
	OnlyMainContent bool              `json:"onlyMainContent,omitempty"`
	WaitFor         int               `json:"waitFor,omitempty"`
	ParsePDF        bool              `json:"parsePDF,omitempty"`
	Timeout         int               `json:"timeout,omitempty"`
}

// BatchScrapeResponse represents the response from a batch scrape request.
type BatchScrapeResponse struct {
	Success     bool     `json:"success"`
	ID          string   `json:"id,omitempty"`
	URL         string   `json:"url,omitempty"`
	InvalidURLs []string `json:"invalidURLs,omitempty"`
}

// BatchScrapeStatus represents the status of a batch scrape job.
type BatchScrapeStatus struct {
	Status      string      `json:"status"`
	Total       int         `json:"total,omitempty"`
	Completed   int         `json:"completed,omitempty"`
	CreditsUsed int         `json:"creditsUsed,omitempty"`
	ExpiresAt   string      `json:"expiresAt,omitempty"`
	Next        *string     `json:"next,omitempty"`
	Data        []*Document `json:"data,omitempty"`
}

type scrapeRequestBody struct {
	URL string `json:"url"`
	*ScrapeOptions
}

type batchScrapeRequestBody struct {
	URLs []string `json:"urls"`
	*ScrapeOptions
}

type scrapeResponse struct {
	Success bool      `json:"success"`
	Data    *Document `json:"data,omitempty"`
}

// NewClient creates a new Firecrawl client with the provided options.
func NewClient(opts ...ClientOption) (*Client, error) {
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

// Scrape performs a single URL scrape.
func (c *Client) Scrape(ctx context.Context, url string, opts *ScrapeOptions) (*Document, error) {
	body := scrapeRequestBody{
		URL:           url,
		ScrapeOptions: opts,
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
	return scrapeResp.Data, nil
}

// BatchScrape performs a synchronous batch scrape of multiple URLs.
func (c *Client) BatchScrape(ctx context.Context, urls []string, opts *ScrapeOptions) (*BatchScrapeStatus, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no URLs provided")
	}
	body := batchScrapeRequestBody{
		URLs:          urls,
		ScrapeOptions: opts,
	}
	resp, err := c.doRequest(ctx, http.MethodPost, "/v1/batch/scrape", &body)
	if err != nil {
		return nil, fmt.Errorf("batch scrape request failed: %w", err)
	}
	var batchResp BatchScrapeResponse
	if err := json.Unmarshal(resp, &batchResp); err != nil {
		return nil, fmt.Errorf("failed to parse batch scrape response: %w", err)
	}
	if !batchResp.Success {
		return nil, fmt.Errorf("batch scrape operation failed")
	}
	// Poll for the job to complete
	status, err := c.WaitForBatchScrape(ctx, batchResp.ID, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for batch scrape job to complete: %w", err)
	}
	return status, nil
}

// CheckBatchScrapeStatus checks the status of an asynchronous batch scrape job.
func (c *Client) CheckBatchScrapeStatus(ctx context.Context, jobID string) (*BatchScrapeStatus, error) {
	if jobID == "" {
		return nil, fmt.Errorf("no job id provided")
	}
	resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/v1/batch/scrape/%s", jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("check batch scrape status failed: %w", err)
	}
	var status BatchScrapeStatus
	if err := json.Unmarshal(resp, &status); err != nil {
		return nil, fmt.Errorf("failed to parse batch scrape status: %w", err)
	}
	return &status, nil
}

// WaitForBatchScrape waits for an asynchronous batch scrape job to complete.
func (c *Client) WaitForBatchScrape(ctx context.Context, jobID string, pollInterval time.Duration) (*BatchScrapeStatus, error) {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			status, err := c.CheckBatchScrapeStatus(ctx, jobID)
			if err != nil {
				fmt.Println("error checking batch scrape status", err)
				continue
			}
			switch status.Status {
			case "completed":
				return status, nil
			case "failed":
				return nil, fmt.Errorf("batch scrape job failed")
			case "active", "paused", "pending", "queued", "waiting", "scraping":
				continue
			default:
				return nil, fmt.Errorf("unknown batch scrape status: %s", status.Status)
			}
		}
	}
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
