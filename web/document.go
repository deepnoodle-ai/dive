package web

// Document contains information about a web page.
type Document struct {
	Markdown   string            `json:"markdown,omitempty"`
	Content    string            `json:"content,omitempty"`
	Screenshot string            `json:"screenshot,omitempty"`
	Links      []string          `json:"links,omitempty"`
	Metadata   *DocumentMetadata `json:"metadata,omitempty"`
}

// DocumentMetadata contains metadata about a web page.
type DocumentMetadata struct {
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Language    string `json:"language,omitempty"`
	Keywords    string `json:"keywords,omitempty"`
}
