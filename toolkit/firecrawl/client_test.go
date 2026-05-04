package firecrawl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/fetch"
)

func TestNewClientMissingAPIKey(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "")
	_, err := New()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_key is required")
}

func TestNewClientFromCredentials(t *testing.T) {
	_, err := NewClientFromCredentials(map[string]any{"api_key": "fc_test"})
	assert.NoError(t, err)

	_, err = NewClientFromCredentials(nil)
	assert.Error(t, err)

	_, err = NewClientFromCredentials(map[string]any{})
	assert.Error(t, err)
}

func TestClientScrape(t *testing.T) {
	var path, auth string
	var body scrapeBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"hello","metadata":{"title":"Test","statusCode":200}}}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient("fc_test", WithBaseURL(srv.URL))
	assert.NoError(t, err)

	resp, err := client.Scrape(context.Background(), ScrapeRequest{
		URL:     "https://example.com",
		Formats: []string{"markdown"},
	})
	assert.NoError(t, err)
	assert.Equal(t, "/scrape", path)
	assert.Equal(t, "Bearer fc_test", auth)
	assert.Equal(t, "https://example.com", body.URL)
	assert.Equal(t, 1, len(body.Formats))
	assert.Equal(t, "markdown", body.Formats[0].Type)
	assert.Equal(t, true, resp.Success)
	assert.Equal(t, "hello", resp.Data.Markdown)
	assert.Equal(t, "Test", resp.Data.Metadata.Title)
	assert.Equal(t, 200, resp.Data.Metadata.StatusCode)
}

func TestClientSearchInjectsDefaultScrapeOptions(t *testing.T) {
	var path, auth string
	var body searchBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"success":true,"data":[{"title":"Example","url":"https://example.com","markdown":"content"}]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient("fc_test", WithBaseURL(srv.URL))
	assert.NoError(t, err)

	resp, err := client.Search(context.Background(), SearchRequest{Query: "mobius", Limit: 5})
	assert.NoError(t, err)
	assert.Equal(t, "/search", path)
	assert.Equal(t, "Bearer fc_test", auth)
	assert.Equal(t, "mobius", body.Query)
	assert.Equal(t, 5, body.Limit)
	assert.NotNil(t, body.ScrapeOptions)
	assert.Equal(t, 1, len(body.ScrapeOptions.Formats))
	assert.Equal(t, "markdown", body.ScrapeOptions.Formats[0].Type)
	assert.Equal(t, true, resp.Success)
	assert.Equal(t, 1, len(resp.Data))
	assert.Equal(t, "Example", resp.Data[0].Title)
	assert.Equal(t, "https://example.com", resp.Data[0].URL)
}

func TestClientSearchPreservesExplicitFormats(t *testing.T) {
	var body searchBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient("fc_test", WithBaseURL(srv.URL))
	assert.NoError(t, err)

	_, err = client.Search(context.Background(), SearchRequest{
		Query:   "mobius",
		Formats: []string{"summary"},
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(body.ScrapeOptions.Formats))
	assert.Equal(t, "summary", body.ScrapeOptions.Formats[0].Type)
}

func TestClientParse(t *testing.T) {
	var path, auth, contentType, fileName, fileBody string
	var options map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		assert.NoError(t, r.ParseMultipartForm(1<<20))
		file, header, err := r.FormFile("file")
		assert.NoError(t, err)
		defer func() { _ = file.Close() }()
		fileName = header.Filename
		raw, err := io.ReadAll(file)
		assert.NoError(t, err)
		fileBody = string(raw)
		values := r.MultipartForm.Value["options"]
		assert.Equal(t, 1, len(values))
		assert.NoError(t, json.Unmarshal([]byte(values[0]), &options))
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"# Parsed"}}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient("fc_test", WithBaseURL(srv.URL))
	assert.NoError(t, err)

	resp, err := client.Parse(context.Background(), ParseRequest{
		FileName:          "report.pdf",
		ContentType:       "application/pdf",
		File:              []byte("%PDF-test"),
		ZeroDataRetention: true,
	})
	assert.NoError(t, err)
	assert.Equal(t, "/parse", path)
	assert.Equal(t, "Bearer fc_test", auth)
	assert.Equal(t, true, strings.HasPrefix(contentType, "multipart/form-data; boundary="))
	assert.Equal(t, "report.pdf", fileName)
	assert.Equal(t, "%PDF-test", fileBody)
	assert.Equal(t, true, options["zeroDataRetention"])
	assert.Equal(t, true, resp.Success)
	assert.Equal(t, "# Parsed", resp.Data.Markdown)
}

func TestClientFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/scrape", r.URL.Path)
		assert.Equal(t, "Bearer fc_test", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"# Example","html":"<h1>Example</h1>","summary":"A test page.","links":["https://example.com/link1","https://example.com/link2"],"metadata":{"title":"Example Page","description":"An example","sourceURL":"https://example.com","statusCode":200}}}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient("fc_test", WithBaseURL(srv.URL))
	assert.NoError(t, err)

	resp, err := client.Fetch(context.Background(), &fetch.Request{
		URL:     "https://example.com",
		Formats: []string{"markdown", "html", "summary", "links"},
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "# Example", resp.Markdown)
	assert.Equal(t, "<h1>Example</h1>", resp.HTML)
	assert.Equal(t, "A test page.", resp.Summary)
	assert.Len(t, resp.Links, 2)
	assert.Equal(t, "https://example.com/link1", resp.Links[0].URL)
	assert.Equal(t, "Example Page", resp.Metadata.Title)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestClientFetchDefaultsToMarkdown(t *testing.T) {
	var body scrapeBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"# Default","metadata":{"sourceURL":"https://example.com"}}}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient("fc_test", WithBaseURL(srv.URL))
	assert.NoError(t, err)

	resp, err := client.Fetch(context.Background(), &fetch.Request{URL: "https://example.com"})
	assert.NoError(t, err)
	assert.Equal(t, "# Default", resp.Markdown)
	assert.Equal(t, 1, len(body.Formats))
	assert.Equal(t, "markdown", body.Formats[0].Type)
}

func TestClientFetchErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"payment required", 402},
		{"rate limit", 429},
		{"server error", 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"error":"test error"}`))
			}))
			t.Cleanup(srv.Close)

			client, err := NewClient("fc_test", WithBaseURL(srv.URL))
			assert.NoError(t, err)

			_, err = client.Fetch(context.Background(), &fetch.Request{URL: "https://example.com"})
			assert.Error(t, err)
			assert.True(t, fetch.IsRequestError(err))
			var reqErr *fetch.RequestError
			assert.True(t, errors.As(err, &reqErr))
			assert.Equal(t, tt.statusCode, reqErr.StatusCode())
		})
	}
}
