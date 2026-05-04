// Package firecrawl provides a client for the Firecrawl v2 API.
package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"time"

	"github.com/deepnoodle-ai/wonton/fetch"
	"github.com/deepnoodle-ai/wonton/web"
)

const DefaultBaseURL = "https://api.firecrawl.dev/v2"

const maxResponseBodySize = 25 << 20 // 25 MiB
const maxParseFileSize = 50 << 20    // 50 MiB

// ErrMissingCredentials is returned when required credentials are absent.
var ErrMissingCredentials = errors.New("firecrawl: missing required credentials")

// ErrNilRequest is returned when a nil request is passed to Fetch.
var ErrNilRequest = errors.New("firecrawl: nil request")

var _ fetch.Fetcher = &Client{}

// Client is a Firecrawl API client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL sets the base URL. Use only for testing or internal routing —
// accepting a caller-supplied URL in production would enable SSRF attacks.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// New creates a client from the FIRECRAWL_API_KEY environment variable.
// FIRECRAWL_API_URL overrides the base URL when set.
func New(opts ...Option) (*Client, error) {
	apiKey := os.Getenv("FIRECRAWL_API_KEY")
	c, err := NewClient(apiKey, opts...)
	if err != nil {
		return nil, err
	}
	if envURL := os.Getenv("FIRECRAWL_API_URL"); envURL != "" {
		c.baseURL = strings.TrimRight(envURL, "/")
	}
	return c, nil
}

// NewClient creates a client with an explicit API key.
func NewClient(apiKey string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("%w: api_key is required", ErrMissingCredentials)
	}
	c := &Client{
		apiKey:     apiKey,
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Scrape fetches a single URL and returns the scraped document.
func (c *Client) Scrape(ctx context.Context, req ScrapeRequest) (*ScrapeResponse, error) {
	if req.URL == "" {
		return nil, errors.New("firecrawl: scrape requires a URL")
	}
	if len(req.Formats) == 0 {
		req.Formats = []string{"markdown"}
	}
	raw, err := c.post(ctx, "/scrape", req.toWire())
	if err != nil {
		return nil, err
	}
	var resp ScrapeResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("firecrawl: decode scrape response: %w", err)
	}
	return &resp, nil
}

// Search performs a web search and returns matching pages with their scraped content.
func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	if req.Query == "" {
		return nil, errors.New("firecrawl: search requires a query")
	}
	body := searchBody{
		Query: req.Query,
		Limit: req.Limit,
	}
	if len(req.Formats) > 0 {
		body.ScrapeOptions = &scrapeOpts{
			Formats: toFormatObjects(req.Formats),
		}
	}
	raw, err := c.post(ctx, "/search", body)
	if err != nil {
		return nil, err
	}
	var wire searchResponseWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("firecrawl: decode search response: %w", err)
	}
	return &SearchResponse{Success: wire.Success, Data: wire.Data.Web}, nil
}

// Parse converts an uploaded document (e.g. a PDF) to markdown.
func (c *Client) Parse(ctx context.Context, req ParseRequest) (*ParseResponse, error) {
	if len(req.File) == 0 {
		return nil, errors.New("firecrawl parse: file is required")
	}
	if len(req.File) > maxParseFileSize {
		return nil, fmt.Errorf("firecrawl parse: file exceeds %d bytes", maxParseFileSize)
	}
	fileName := req.FileName
	if fileName == "" {
		fileName = "document"
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	filePart, err := writer.CreatePart(filePartHeader(fileName, req.ContentType))
	if err != nil {
		return nil, fmt.Errorf("firecrawl parse: create file part: %w", err)
	}
	if _, err := filePart.Write(req.File); err != nil {
		return nil, fmt.Errorf("firecrawl parse: write file part: %w", err)
	}

	opts := map[string]any{}
	if len(req.Formats) > 0 {
		opts["formats"] = toFormatObjects(req.Formats)
	}
	if req.ZeroDataRetention {
		opts["zeroDataRetention"] = true
	}
	if len(opts) > 0 {
		optsPart, err := writer.CreatePart(formPartHeader("options", "application/json"))
		if err != nil {
			return nil, fmt.Errorf("firecrawl parse: create options part: %w", err)
		}
		if err := json.NewEncoder(optsPart).Encode(opts); err != nil {
			return nil, fmt.Errorf("firecrawl parse: encode options: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("firecrawl parse: close multipart body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/parse", &buf)
	if err != nil {
		return nil, fmt.Errorf("firecrawl: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	raw, err := c.do(httpReq)
	if err != nil {
		return nil, err
	}
	var resp ParseResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("firecrawl: decode parse response: %w", err)
	}
	return &resp, nil
}

// Fetch implements the wonton fetch.Fetcher interface, allowing this client to
// be passed directly to toolkit.FetchTool.
func (c *Client) Fetch(ctx context.Context, req *fetch.Request) (*fetch.Response, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	formats := req.Formats
	if len(formats) == 0 {
		formats = []string{"markdown"}
	}
	scrapeReq := ScrapeRequest{
		URL:                req.URL,
		Formats:            formats,
		OnlyMainContent:    true,
		RemoveBase64Images: true,
		BlockAds:           true,
		Proxy:              "auto", // use Firecrawl's auto proxy for better compatibility
		StoreInCache:       true,   // cache responses to reduce API costs
		Headers:            req.Headers,
		IncludeTags:        req.IncludeTags,
		ExcludeTags:        req.ExcludeTags,
		MaxAge:             time.Duration(req.MaxAge) * time.Millisecond,
		WaitFor:            time.Duration(req.WaitFor) * time.Millisecond,
		Timeout:            time.Duration(req.Timeout) * time.Millisecond,
		Mobile:             req.Mobile,
	}

	scrapeResp, err := c.Scrape(ctx, scrapeReq)
	if err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			return nil, fetch.NewRequestErrorf("%s", apiErr.message).WithStatusCode(apiErr.statusCode)
		}
		return nil, err
	}

	resp := &fetch.Response{
		URL:       req.URL,
		Timestamp: time.Now().UTC(),
	}
	doc := scrapeResp.Data
	if doc == nil {
		return resp, nil
	}
	resp.Markdown = doc.Markdown
	resp.HTML = doc.HTML
	resp.RawHTML = doc.RawHTML
	resp.Summary = doc.Summary
	resp.Screenshot = doc.Screenshot
	for _, link := range doc.Links {
		resp.Links = append(resp.Links, fetch.Link{URL: link})
	}
	if doc.Metadata != nil {
		resp.StatusCode = doc.Metadata.StatusCode
		resp.Metadata = fetch.Metadata{
			Title:       doc.Metadata.Title,
			Description: doc.Metadata.Description,
			Canonical:   doc.Metadata.SourceURL,
		}
	}
	return resp, nil
}

// Searcher returns an adapter that implements web.Searcher using the metadata-only
// (no-scrape) search path. Use this to back toolkit.WebSearchTool with Firecrawl.
func (c *Client) Searcher() web.Searcher {
	return &searcherAdapter{c}
}

type searcherAdapter struct{ c *Client }

func (a *searcherAdapter) Search(ctx context.Context, input *web.SearchInput) (*web.SearchOutput, error) {
	resp, err := a.c.Search(ctx, SearchRequest{Query: input.Query, Limit: input.Limit})
	if err != nil {
		return nil, err
	}
	items := make([]*web.SearchItem, 0, len(resp.Data))
	for _, r := range resp.Data {
		items = append(items, &web.SearchItem{
			URL:         r.URL,
			Title:       r.Title,
			Description: r.Description,
		})
	}
	return &web.SearchOutput{Items: items}, nil
}

// apiError carries an HTTP status code through the error chain so Fetch can
// convert it to a fetch.RequestError without exposing wonton in the core API.
type apiError struct {
	statusCode int
	message    string
}

func (e *apiError) Error() string { return e.message }

func (c *Client) post(ctx context.Context, path string, body any) ([]byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("firecrawl: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("firecrawl: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firecrawl: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("firecrawl: read response: %w", err)
	}
	if int64(len(body)) > maxResponseBodySize {
		return nil, fmt.Errorf("firecrawl: response body exceeds %d bytes", maxResponseBodySize)
	}
	if resp.StatusCode >= 400 {
		return nil, &apiError{
			statusCode: resp.StatusCode,
			message:    fmt.Sprintf("firecrawl: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
		}
	}
	return body, nil
}

func filePartHeader(fileName, contentType string) textproto.MIMEHeader {
	header := formPartHeader("file", contentType)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeMultipartValue(fileName)))
	if contentType == "" {
		header.Set("Content-Type", "application/octet-stream")
	}
	return header
}

func formPartHeader(name, contentType string) textproto.MIMEHeader {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"`, escapeMultipartValue(name)))
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	return header
}

func escapeMultipartValue(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}
