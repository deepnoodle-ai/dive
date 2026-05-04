package firecrawl

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/fetch"
)

func integrationClient(t *testing.T) *Client {
	t.Helper()
	apiKey := os.Getenv("FIRECRAWL_API_KEY")
	if apiKey == "" {
		t.Skip("skipping integration test: FIRECRAWL_API_KEY not set")
	}
	c, err := NewClient(apiKey)
	assert.NoError(t, err)
	return c
}

func TestIntegrationScrape(t *testing.T) {
	c := integrationClient(t)
	resp, err := c.Scrape(context.Background(), ScrapeRequest{
		URL:             "https://example.com",
		OnlyMainContent: true,
	})
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Data)
	assert.True(t, len(resp.Data.Markdown) > 0)
}

func TestIntegrationSearch(t *testing.T) {
	c := integrationClient(t)
	resp, err := c.Search(context.Background(), SearchRequest{
		Query: "golang context package",
		Limit: 3,
	})
	assert.NoError(t, err)
	assert.True(t, resp.Success)
	assert.True(t, len(resp.Data) > 0)
	assert.True(t, len(resp.Data[0].URL) > 0)
}

func TestIntegrationFetch(t *testing.T) {
	c := integrationClient(t)
	resp, err := c.Fetch(context.Background(), &fetch.Request{URL: "https://example.com"})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, len(resp.Markdown) > 0)
	assert.True(t, resp.Timestamp.Before(time.Now()))
}
