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
)

var baseURL = "https://www.googleapis.com/customsearch/v1"

func SetBaseURL(url string) {
	baseURL = url
}

type Query struct {
	Text  string `json:"text"`
	Limit int    `json:"limit,omitempty"`
	// TimeRange string `json:"time_range,omitempty"`
	// Category  string `json:"category,omitempty"`
	// Offset    int    `json:"offset,omitempty"`
}

type Item struct {
	Rank        int    `json:"rank"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Image       string `json:"image,omitempty"`
}

type Results struct {
	Items []*Item `json:"items"`
}

type Client struct {
	cx  string
	key string
}

func New() (*Client, error) {
	cx := os.Getenv("GOOGLE_SEARCH_CX")
	if cx == "" {
		return nil, fmt.Errorf("missing GOOGLE_SEARCH_CX")
	}
	key := os.Getenv("GOOGLE_SEARCH_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("missing GOOGLE_SEARCH_API_KEY")
	}
	return &Client{cx: cx, key: key}, nil
}

func (s *Client) Search(ctx context.Context, q *Query) (*Results, error) {
	if q.Limit < 0 || q.Limit > 100 {
		return nil, fmt.Errorf("invalid limit: %d", q.Limit)
	}
	if q.Limit == 0 {
		q.Limit = 10
	}
	pageCount := (q.Limit + 9) / 10 // Google always returns 10 results per page
	var items []*Item
	var curPage, curRank int
	for curPage < pageCount {
		params := url.Values{}
		params.Set("key", s.key)
		params.Set("cx", s.cx)
		params.Set("q", q.Text)
		params.Set("start", fmt.Sprintf("%d", curPage*10+1))
		rawURL := baseURL + "?" + params.Encode()
		crs, err := fetchResultsWithRetries(ctx, rawURL, 3)
		if err != nil {
			return nil, err
		}
		for _, item := range crs.Items {
			curRank++
			items = append(items, &Item{
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
	return &Results{Items: items}, nil
}

func fetchResultsWithRetries(ctx context.Context, url string, retries int) (*results, error) {
	var err error
	for i := 0; i < retries; i++ {
		var results *results
		results, err = fetchResults(ctx, url)
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

func fetchResults(ctx context.Context, url string) (*results, error) {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
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
