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

	"github.com/diveagents/dive/web"
)

var kagiBaseURL = "https://kagi.com/api/v0/search"

func SetKagiBaseURL(url string) {
	kagiBaseURL = url
}

type KagiClientOption func(*KagiClient)

func WithKagiAPIKey(apiKey string) KagiClientOption {
	return func(c *KagiClient) {
		c.apiKey = apiKey
	}
}

func WithKagiBaseURL(url string) KagiClientOption {
	return func(c *KagiClient) {
		kagiBaseURL = url
	}
}

func WithKagiHTTPClient(httpClient *http.Client) KagiClientOption {
	return func(c *KagiClient) {
		c.httpClient = httpClient
	}
}

type KagiClient struct {
	apiKey     string
	httpClient *http.Client
}

func New(opts ...KagiClientOption) (*KagiClient, error) {
	c := &KagiClient{
		apiKey: os.Getenv("KAGI_API_KEY"),
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

func (s *KagiClient) Search(ctx context.Context, q *web.SearchInput) (*web.SearchOutput, error) {
	if q.Limit < 0 {
		return nil, fmt.Errorf("invalid limit: %d", q.Limit)
	}

	params := url.Values{}
	params.Set("q", q.Query)
	if q.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", q.Limit))
	}

	rawURL := kagiBaseURL + "?" + params.Encode()
	results, err := s.fetchResultsWithRetries(ctx, rawURL, 3)
	if err != nil {
		return nil, err
	}

	var items []*web.SearchItem
	rank := 0

	for _, item := range results.Data {
		if item.Type == 0 {
			rank++
			items = append(items, &web.SearchItem{
				Rank:        rank,
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

func (s *KagiClient) getThumbnailURL(item *kagiSearchResultItem) string {
	if item.Thumbnail != nil && item.Thumbnail.URL != "" {
		return item.Thumbnail.URL
	}
	return ""
}

func (s *KagiClient) fetchResultsWithRetries(ctx context.Context, url string, retries int) (*kagiResults, error) {
	var err error
	for range retries {
		var results *kagiResults
		results, err = s.fetchResults(ctx, url)
		if err == nil {
			return results, nil
		}
		time.Sleep(time.Second * 2)
	}
	return nil, fmt.Errorf("failed to fetch %q after %d retries: %w", url, retries, err)
}

func (s *KagiClient) fetchResults(ctx context.Context, url string) (*kagiResults, error) {
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

	var result kagiResults
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	if len(result.Error) > 0 {
		return nil, fmt.Errorf("kagi api error: %s", result.Error[0].Message)
	}

	return &result, nil
}

type kagiResults struct {
	Meta struct {
		ID         string  `json:"id"`
		Node       string  `json:"node"`
		MS         int     `json:"ms"`
		APIBalance float64 `json:"api_balance"`
	} `json:"meta"`
	Data  []*kagiSearchResultItem `json:"data"`
	Error []struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Ref     string `json:"ref"`
	} `json:"error"`
}

type kagiSearchResultItem struct {
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
