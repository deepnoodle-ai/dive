package firecrawl

// document represents a scraped document from Firecrawl.
type document struct {
	Markdown   string            `json:"markdown,omitempty"`
	HTML       string            `json:"html,omitempty"`
	RawHTML    string            `json:"rawHtml,omitempty"`
	Screenshot string            `json:"screenshot,omitempty"`
	Links      []string          `json:"links,omitempty"`
	Metadata   *documentMetadata `json:"metadata,omitempty"`
}

// documentMetadata contains metadata about a scraped document.
type documentMetadata struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Language    string `json:"language,omitempty"`
	Keywords    string `json:"keywords,omitempty"`
	SourceURL   string `json:"sourceURL,omitempty"`
	StatusCode  int    `json:"statusCode,omitempty"`
	Error       string `json:"error,omitempty"`
}

type scrapeResponse struct {
	Success bool      `json:"success"`
	Data    *document `json:"data,omitempty"`
}

type scrapeRequestBody struct {
	URL             string            `json:"url"`
	Formats         []string          `json:"formats,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	IncludeTags     []string          `json:"includeTags,omitempty"`
	ExcludeTags     []string          `json:"excludeTags,omitempty"`
	OnlyMainContent bool              `json:"onlyMainContent,omitempty"`
	WaitFor         int               `json:"waitFor,omitempty"`
	ParsePDF        bool              `json:"parsePDF,omitempty"`
	Timeout         int               `json:"timeout,omitempty"`
}
