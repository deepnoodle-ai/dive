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
	assert.Contains(t, resp.Data.Markdown, "Example Domain")
	assert.NotNil(t, resp.Data.Metadata)
	assert.Equal(t, "Example Domain", resp.Data.Metadata.Title)
	assert.Equal(t, 200, resp.Data.Metadata.StatusCode)
	assert.Equal(t, "https://example.com", resp.Data.Metadata.SourceURL)
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
	first := resp.Data[0]
	assert.True(t, len(first.URL) > 0)
	assert.True(t, len(first.Title) > 0)
	assert.True(t, len(first.Description) > 0)
	assert.True(t, len(first.Markdown) > 0)
	assert.True(t, first.Position >= 1)
}

func TestIntegrationFetch(t *testing.T) {
	c := integrationClient(t)
	resp, err := c.Fetch(context.Background(), &fetch.Request{
		URL:     "https://example.com",
		Formats: []string{"markdown", "links"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Markdown, "Example Domain")
	assert.Equal(t, "Example Domain", resp.Metadata.Title)
	assert.Equal(t, "https://example.com", resp.URL)
	assert.True(t, len(resp.Links) > 0)
	assert.True(t, resp.Timestamp.Before(time.Now()))
}
