package kagi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/deepnoodle-ai/wonton/web"
)

const defaultBaseURL = "https://kagi.com/api/v0/search"

// ClientOption is a function that modifies the client configuration.
type ClientOption func(*Client)

// WithAPIKey sets the API key for the client.
func WithAPIKey(apiKey string) ClientOption {
	return func(c *Client) {
		c.apiKey = apiKey
	}
}

// WithBaseURL sets the base URL for the client.
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = url
	}
}

// WithHTTPClient sets the HTTP client for the client.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// Client is a Kagi search API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates a new Kagi search client.
func New(opts ...ClientOption) (*Client, error) {
	c := &Client{
		apiKey:  os.Getenv("KAGI_API_KEY"),
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("missing kagi api key")
	}
	return c, nil
}

func (s *Client) Search(ctx context.Context, q *web.SearchInput) (*web.SearchOutput, error) {
	if q.Limit < 0 {
		return nil, fmt.Errorf("invalid limit: %d", q.Limit)
	}

	params := url.Values{}
	params.Set("q", q.Query)
	if q.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", q.Limit))
	}

	rawURL := s.baseURL + "?" + params.Encode()
	results, err := s.fetchResultsWithRetries(ctx, rawURL, 3)
	if err != nil {
		return nil, err
	}

	var items []*web.SearchItem

	for _, item := range results.Data {
		if item.Type == 0 {
			items = append(items, &web.SearchItem{
				URL:         item.URL,
				Title:       item.Title,
				Description: item.Snippet,
				Icon:        s.getThumbnailURL(item),
				Image:       s.getThumbnailURL(item),
			})
		}
	}

	return &web.SearchOutput{Items: items}, nil
}

func (s *Client) getThumbnailURL(item *searchResultItem) string {
	if item.Thumbnail != nil && item.Thumbnail.URL != "" {
		return item.Thumbnail.URL
	}
	return ""
}

func (s *Client) fetchResultsWithRetries(ctx context.Context, url string, retries int) (*searchResults, error) {
	var err error
	for range retries {
		var results *searchResults
		results, err = s.fetchResults(ctx, url)
		if err == nil {
			return results, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second * 2):
		}
	}
	return nil, fmt.Errorf("failed to fetch %q after %d retries: %w", url, retries, err)
}

func (s *Client) fetchResults(ctx context.Context, url string) (*searchResults, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bot "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result searchResults
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	if len(result.Error) > 0 {
		return nil, fmt.Errorf("kagi api error: %s", result.Error[0].Message)
	}

	return &result, nil
}

type searchResults struct {
	Meta struct {
		ID         string  `json:"id"`
		Node       string  `json:"node"`
		MS         int     `json:"ms"`
		APIBalance float64 `json:"api_balance"`
	} `json:"meta"`
	Data  []*searchResultItem `json:"data"`
	Error []struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Ref     string `json:"ref"`
	} `json:"error"`
}

type searchResultItem struct {
	Type      int    `json:"t"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Snippet   string `json:"snippet"`
	Published string `json:"published,omitempty"`
	Thumbnail *struct {
		URL    string `json:"url"`
		Width  int    `json:"width,omitempty"`
		Height int    `json:"height,omitempty"`
	} `json:"thumbnail,omitempty"`
	List []string `json:"list,omitempty"` // for related searches (t=1)
}
