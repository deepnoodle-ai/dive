package google

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/deepnoodle-ai/wonton/web"
)

const defaultBaseURL = "https://www.googleapis.com/customsearch/v1"

// ClientOption is a function that modifies the client configuration.
type ClientOption func(*Client)

// WithCredentials sets both the CX and API key for the client.
func WithCredentials(cx, apiKey string) ClientOption {
	return func(c *Client) {
		c.cx = cx
		c.key = apiKey
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

type Client struct {
	cx         string
	key        string
	baseURL    string
	httpClient *http.Client
}

func New(opts ...ClientOption) (*Client, error) {
	c := &Client{
		cx:      os.Getenv("GOOGLE_SEARCH_CX"),
		key:     os.Getenv("GOOGLE_SEARCH_API_KEY"),
		baseURL: defaultBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.cx == "" {
		return nil, fmt.Errorf("missing google search cx")
	}
	if c.key == "" {
		return nil, fmt.Errorf("missing google search api key")
	}
	return c, nil
}

func (s *Client) Search(ctx context.Context, q *web.SearchInput) (*web.SearchOutput, error) {
	limit := q.Limit
	if limit < 0 || limit > 100 {
		return nil, fmt.Errorf("invalid limit: %d", limit)
	}
	if limit == 0 {
		limit = 10
	}
	pageCount := (limit + 9) / 10 // Google always returns 10 results per page
	var items []*web.SearchItem
	var curPage int
	for curPage < pageCount {
		params := url.Values{}
		params.Set("key", s.key)
		params.Set("cx", s.cx)
		params.Set("q", q.Query)
		params.Set("start", fmt.Sprintf("%d", curPage*10+1))
		rawURL := s.baseURL + "?" + params.Encode()
		crs, err := s.fetchResultsWithRetries(ctx, rawURL, 3)
		if err != nil {
			return nil, err
		}
		for _, item := range crs.Items {
			items = append(items, &web.SearchItem{
				URL:         item.Link,
				Title:       item.Title,
				Description: item.Snippet,
				Icon:        item.Thumbnail(),
				Image:       item.Image(),
			})
		}
		curPage++
	}
	return &web.SearchOutput{Items: items}, nil
}

func (s *Client) fetchResultsWithRetries(ctx context.Context, url string, retries int) (*results, error) {
	var err error
	for i := 0; i < retries; i++ {
		var results *results
		results, err = s.fetchResults(ctx, url)
		if err == nil {
			return results, nil
		}
		if strings.Contains(err.Error(), "429") {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second * 5):
			}
			continue
		}
	}
	return nil, fmt.Errorf("failed to fetch %q after %d retries: %w", url, retries, err)
}

func (s *Client) fetchResults(ctx context.Context, url string) (*results, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
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
	var result results
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
