package firecrawl

// Format represents the different output formats available in Firecrawl v2
// We use interface{} here because the v2 API accepts both strings and objects
type Format interface{}

// MarkdownFormat represents markdown output format
type MarkdownFormat struct {
	Type string `json:"type"` // "markdown"
}

// SummaryFormat represents summary output format
type SummaryFormat struct {
	Type string `json:"type"` // "summary"
}

// HTMLFormat represents HTML output format
type HTMLFormat struct {
	Type string `json:"type"` // "html"
}

// RawHTMLFormat represents raw HTML output format
type RawHTMLFormat struct {
	Type string `json:"type"` // "rawHtml"
}

// LinksFormat represents links output format
type LinksFormat struct {
	Type string `json:"type"` // "links"
}

// ScreenshotFormat represents screenshot output format
type ScreenshotFormat struct {
	Type     string    `json:"type"` // "screenshot"
	FullPage *bool     `json:"fullPage,omitempty"`
	Quality  *int      `json:"quality,omitempty"`
	Viewport *Viewport `json:"viewport,omitempty"`
}

// JSONFormat represents JSON extraction format
type JSONFormat struct {
	Type   string      `json:"type"` // "json"
	Schema interface{} `json:"schema"`
	Prompt string      `json:"prompt,omitempty"`
}

// ChangeTrackingFormat represents change tracking format
type ChangeTrackingFormat struct {
	Type   string      `json:"type"` // "changeTracking"
	Modes  []string    `json:"modes,omitempty"`
	Schema interface{} `json:"schema,omitempty"`
	Prompt string      `json:"prompt,omitempty"`
	Tag    *string     `json:"tag,omitempty"`
}

// Viewport represents viewport dimensions
type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Location represents location settings
type Location struct {
	Country   string   `json:"country,omitempty"`
	Languages []string `json:"languages,omitempty"`
}

// Action represents actions to perform on the page
type Action interface{}

// WaitAction represents a wait action
type WaitAction struct {
	Type         string `json:"type"` // "wait"
	Milliseconds int    `json:"milliseconds"`
	Selector     string `json:"selector,omitempty"`
}

// ScreenshotAction represents a screenshot action
type ScreenshotAction struct {
	Type     string    `json:"type"` // "screenshot"
	FullPage *bool     `json:"fullPage,omitempty"`
	Quality  *int      `json:"quality,omitempty"`
	Viewport *Viewport `json:"viewport,omitempty"`
}

// ClickAction represents a click action
type ClickAction struct {
	Type     string `json:"type"` // "click"
	Selector string `json:"selector"`
	All      *bool  `json:"all,omitempty"`
}

// WriteAction represents a write text action
type WriteAction struct {
	Type string `json:"type"` // "write"
	Text string `json:"text"`
}

// PressAction represents a key press action
type PressAction struct {
	Type string `json:"type"` // "press"
	Key  string `json:"key"`
}

// ScrollAction represents a scroll action
type ScrollAction struct {
	Type      string `json:"type"`                // "scroll"
	Direction string `json:"direction,omitempty"` // "up" or "down"
	Selector  string `json:"selector,omitempty"`
}

// ScrapeAction represents a scrape action
type ScrapeAction struct {
	Type string `json:"type"` // "scrape"
}

// ExecuteJavaScriptAction represents a JavaScript execution action
type ExecuteJavaScriptAction struct {
	Type   string `json:"type"` // "executeJavascript"
	Script string `json:"script"`
}

// PDFAction represents a PDF generation action
type PDFAction struct {
	Type      string   `json:"type"` // "pdf"
	Format    string   `json:"format,omitempty"`
	Landscape *bool    `json:"landscape,omitempty"`
	Scale     *float64 `json:"scale,omitempty"`
}

// Parser represents parsers configuration
type Parser interface{}

// PDFParser represents PDF parsing configuration
type PDFParser struct {
	Type     string `json:"type"` // "pdf"
	MaxPages *int   `json:"maxPages,omitempty"`
}

// document represents a scraped document from Firecrawl v2
type document struct {
	Markdown       string                 `json:"markdown,omitempty"`
	Summary        *string                `json:"summary,omitempty"`
	HTML           *string                `json:"html,omitempty"`
	RawHTML        *string                `json:"rawHtml,omitempty"`
	Screenshot     *string                `json:"screenshot,omitempty"`
	Links          []string               `json:"links,omitempty"`
	Actions        *actionResults         `json:"actions,omitempty"`
	Metadata       *documentMetadata      `json:"metadata,omitempty"`
	Warning        *string                `json:"warning,omitempty"`
	ChangeTracking *changeTrackingResults `json:"changeTracking,omitempty"`
}

// actionResults contains results from actions
type actionResults struct {
	Screenshots       []string           `json:"screenshots,omitempty"`
	Scrapes           []scrapeResult     `json:"scrapes,omitempty"`
	JavascriptReturns []javascriptReturn `json:"javascriptReturns,omitempty"`
	PDFs              []string           `json:"pdfs,omitempty"`
}

// scrapeResult contains result from a scrape action
type scrapeResult struct {
	URL  string `json:"url"`
	HTML string `json:"html"`
}

// javascriptReturn contains result from JavaScript execution
type javascriptReturn struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// changeTrackingResults contains change tracking information
type changeTrackingResults struct {
	PreviousScrapeAt *string     `json:"previousScrapeAt,omitempty"`
	ChangeStatus     string      `json:"changeStatus,omitempty"`
	Visibility       string      `json:"visibility,omitempty"`
	Diff             *string     `json:"diff,omitempty"`
	JSON             interface{} `json:"json,omitempty"`
}

// documentMetadata contains metadata about a scraped document
type documentMetadata struct {
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Language    *string                `json:"language,omitempty"`
	SourceURL   string                 `json:"sourceURL,omitempty"`
	StatusCode  int                    `json:"statusCode,omitempty"`
	Error       *string                `json:"error,omitempty"`
	Other       map[string]interface{} `json:"-"` // For any other metadata fields
}

// scrapeResponse represents the response from Firecrawl v2 scrape API
type scrapeResponse struct {
	Success bool      `json:"success"`
	Data    *document `json:"data,omitempty"`
}

// scrapeRequestBody represents the request body for Firecrawl v2 scrape API
type scrapeRequestBody struct {
	URL                 string            `json:"url"`
	Formats             []Format          `json:"formats,omitempty"`
	OnlyMainContent     *bool             `json:"onlyMainContent,omitempty"`
	IncludeTags         []string          `json:"includeTags,omitempty"`
	ExcludeTags         []string          `json:"excludeTags,omitempty"`
	MaxAge              *int64            `json:"maxAge,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	WaitFor             *int              `json:"waitFor,omitempty"`
	Mobile              *bool             `json:"mobile,omitempty"`
	SkipTlsVerification *bool             `json:"skipTlsVerification,omitempty"`
	Timeout             *int              `json:"timeout,omitempty"`
	Parsers             []Parser          `json:"parsers,omitempty"`
	Actions             []Action          `json:"actions,omitempty"`
	Location            *Location         `json:"location,omitempty"`
	RemoveBase64Images  *bool             `json:"removeBase64Images,omitempty"`
	BlockAds            *bool             `json:"blockAds,omitempty"`
	Proxy               *string           `json:"proxy,omitempty"`
	StoreInCache        *bool             `json:"storeInCache,omitempty"`
	ZeroDataRetention   *bool             `json:"zeroDataRetention,omitempty"`
}
