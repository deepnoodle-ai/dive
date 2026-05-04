package firecrawl

import "time"

// Document is a scraped or parsed web document returned by Scrape and Parse.
type Document struct {
	Markdown   string    `json:"markdown,omitempty"`
	HTML       string    `json:"html,omitempty"`
	RawHTML    string    `json:"rawHtml,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Screenshot string    `json:"screenshot,omitempty"`
	Links      []string  `json:"links,omitempty"`
	Metadata   *Metadata `json:"metadata,omitempty"`
}

// Metadata holds page-level metadata from a scraped document.
type Metadata struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Language    string `json:"language,omitempty"`
	SourceURL   string `json:"sourceURL,omitempty"`
	StatusCode  int    `json:"statusCode,omitempty"`
}

// ScrapeRequest is the input for a Scrape call.
type ScrapeRequest struct {
	URL     string   // required
	Formats []string // e.g. "markdown", "html", "rawHtml", "links", "summary"; defaults to ["markdown"]

	OnlyMainContent    bool
	IncludeTags        []string
	ExcludeTags        []string
	Headers            map[string]string
	WaitFor            time.Duration
	Timeout            time.Duration
	MaxAge             time.Duration
	Mobile             bool
	RemoveBase64Images bool
	BlockAds           bool
	Proxy              string // e.g. "auto", "stealth"
	StoreInCache       bool
	ZeroDataRetention  bool
}

func (req ScrapeRequest) toWire() scrapeBody {
	return scrapeBody{
		URL:                req.URL,
		Formats:            toFormatObjects(req.Formats),
		OnlyMainContent:    req.OnlyMainContent,
		IncludeTags:        req.IncludeTags,
		ExcludeTags:        req.ExcludeTags,
		Headers:            req.Headers,
		WaitFor:            int(req.WaitFor.Milliseconds()),
		Timeout:            int(req.Timeout.Milliseconds()),
		MaxAge:             int(req.MaxAge.Milliseconds()),
		Mobile:             req.Mobile,
		RemoveBase64Images: req.RemoveBase64Images,
		BlockAds:           req.BlockAds,
		Proxy:              req.Proxy,
		StoreInCache:       req.StoreInCache,
		ZeroDataRetention:  req.ZeroDataRetention,
	}
}

// ScrapeResponse is the response from a Scrape call.
type ScrapeResponse struct {
	Success bool      `json:"success"`
	Data    *Document `json:"data,omitempty"`
}

// SearchRequest is the input for a Search call.
type SearchRequest struct {
	Query   string   // required
	Limit   int      // max results; 0 = server default
	Formats []string // formats for scraped result content; defaults to ["markdown"]
}

// SearchResult is a single result from a Search call.
type SearchResult struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
	Markdown    string `json:"markdown,omitempty"`
}

// SearchResponse is the response from a Search call.
type SearchResponse struct {
	Success bool           `json:"success"`
	Data    []SearchResult `json:"data,omitempty"`
}

// ParseRequest is the input for a Parse call.
type ParseRequest struct {
	FileName          string // defaults to "document" if empty
	ContentType       string // e.g. "application/pdf"; defaults to "application/octet-stream"
	File              []byte // required
	Formats           []string
	ZeroDataRetention bool
}

// ParseResponse is the response from a Parse call.
type ParseResponse struct {
	Success bool      `json:"success"`
	Data    *Document `json:"data,omitempty"`
}

// formatObject is the wire representation of a format in Firecrawl v2 requests.
type formatObject struct {
	Type string `json:"type"`
}

func toFormatObjects(formats []string) []formatObject {
	out := make([]formatObject, 0, len(formats))
	for _, f := range formats {
		out = append(out, formatObject{Type: f})
	}
	return out
}

// scrapeBody is the JSON wire type for scrape and Fetch requests.
type scrapeBody struct {
	URL                string            `json:"url"`
	Formats            []formatObject    `json:"formats,omitempty"`
	OnlyMainContent    bool              `json:"onlyMainContent,omitempty"`
	IncludeTags        []string          `json:"includeTags,omitempty"`
	ExcludeTags        []string          `json:"excludeTags,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	WaitFor            int               `json:"waitFor,omitempty"`
	Timeout            int               `json:"timeout,omitempty"`
	MaxAge             int               `json:"maxAge,omitempty"`
	Mobile             bool              `json:"mobile,omitempty"`
	RemoveBase64Images bool              `json:"removeBase64Images,omitempty"`
	BlockAds           bool              `json:"blockAds,omitempty"`
	Proxy              string            `json:"proxy,omitempty"`
	StoreInCache       bool              `json:"storeInCache,omitempty"`
	ZeroDataRetention  bool              `json:"zeroDataRetention,omitempty"`
}

// searchBody is the JSON wire type for search requests.
type searchBody struct {
	Query         string       `json:"query"`
	Limit         int          `json:"limit,omitempty"`
	ScrapeOptions *scrapeOpts  `json:"scrapeOptions,omitempty"`
}

type scrapeOpts struct {
	Formats []formatObject `json:"formats,omitempty"`
}
