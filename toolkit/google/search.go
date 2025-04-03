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

	"github.com/diveagents/dive/web"
)

var baseURL = "https://www.googleapis.com/customsearch/v1"

func SetBaseURL(url string) {
	baseURL = url
}

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
		baseURL = url
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
	httpClient *http.Client
}

func New(opts ...ClientOption) (*Client, error) {
	c := &Client{
		cx:  os.Getenv("GOOGLE_SEARCH_CX"),
		key: os.Getenv("GOOGLE_SEARCH_API_KEY"),
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
	if q.Limit < 0 || q.Limit > 100 {
		return nil, fmt.Errorf("invalid limit: %d", q.Limit)
	}
	if q.Limit == 0 {
		q.Limit = 10
	}
	pageCount := (q.Limit + 9) / 10 // Google always returns 10 results per page
	var items []*web.SearchItem
	var curPage, curRank int
	for curPage < pageCount {
		params := url.Values{}
		params.Set("key", s.key)
		params.Set("cx", s.cx)
		params.Set("q", q.Query)
		params.Set("start", fmt.Sprintf("%d", curPage*10+1))
		rawURL := baseURL + "?" + params.Encode()
		crs, err := s.fetchResultsWithRetries(ctx, rawURL, 3)
		if err != nil {
			return nil, err
		}
		for _, item := range crs.Items {
			curRank++
			items = append(items, &web.SearchItem{
				Rank:        curRank,
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
			time.Sleep(time.Second * 5)
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

// udm=14 indicates "web" search (no AI results)
// udm=18 indicates "forums" search
// tbm=nws indicates "news" search
// udm=2 indicates "images" search
// tbm=vid indicates "videos" search
// tbm=bks indicates "books" search
// tbs=qdr:w indicates "past week" search
// tbs=qdr:m indicates "past month" search
// tbs=qdr:y indicates "past year" search

// type TimeRange string

// const (
// 	PastWeek  TimeRange = "tbs=qdr:w"
// 	PastMonth TimeRange = "tbs=qdr:m"
// 	PastYear  TimeRange = "tbs=qdr:y"
// )

// type Category string

// const (
// 	Web    Category = "udm=14"
// 	Forums Category = "udm=18"
// 	News   Category = "tbm=nws"
// 	Images Category = "udm=2"
// 	Videos Category = "tbm=vid"
// 	Books  Category = "tbm=bks"
// )
