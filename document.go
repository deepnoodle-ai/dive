package dive

import "path/filepath"

var (
	_ Document = &TextDocument{}
)

// DocumentRef is used to point to one or more matching documents
type DocumentRef struct {
	Name string   `json:"name,omitempty"`
	Tags []string `json:"tags,omitempty"`
	Glob string   `json:"glob,omitempty"`
}

// Chunk of a document. I'm not yet sure if this should have a pointer to a
// Document or if it should be referenced by name/id/path. :thinking:
type Chunk struct {
	Index      int      `json:"index"`
	Content    string   `json:"content"`
	Heading    string   `json:"heading,omitempty"`
	DocumentID string   `json:"document_id,omitempty"`
	Document   Document `json:"-"`
}

// Document containing content that can be read or written to by an agent
type Document interface {
	ID() string
	Name() string
	Description() string
	Path() string
	Version() int
	Content() string
	ContentType() string
	Chunks() []*Chunk
	SetContent(content string) error
}

// DocumentMetadata contains an overview of a document
type DocumentMetadata struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
	Version     int    `json:"version,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// TextDocumentOptions are used to initialize a TextDocument
type TextDocumentOptions struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
	Version     int    `json:"version,omitempty"`
	Content     string `json:"content,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// NewTextDocument returns a new TextDocument with the given options and content
func NewTextDocument(opts TextDocumentOptions) *TextDocument {
	if opts.ContentType == "" {
		opts.ContentType = "text/plain"
	}

	// Validate name matches path basename if both are provided
	if opts.Name != "" && opts.Path != "" {
		basename := filepath.Base(opts.Path)
		if basename != opts.Name {
			// Auto-correct to match the path. Could panic instead...
			opts.Name = basename
		}
	} else if opts.Path != "" {
		// If only path is provided, derive name from it
		opts.Name = filepath.Base(opts.Path)
	}

	return &TextDocument{
		id:          opts.ID,
		name:        opts.Name,
		description: opts.Description,
		path:        opts.Path,
		version:     opts.Version,
		content:     opts.Content,
		contentType: opts.ContentType,
	}
}

// TextDocument implements the Document interface
type TextDocument struct {
	id          string
	name        string
	description string
	path        string
	version     int
	content     string
	contentType string
}

func (d *TextDocument) ID() string {
	return d.id
}

func (d *TextDocument) Name() string {
	return d.name
}

func (d *TextDocument) Path() string {
	return d.path
}

func (d *TextDocument) Version() int {
	return d.version
}

func (d *TextDocument) Description() string {
	return d.description
}

func (d *TextDocument) Content() string {
	return d.content
}

func (d *TextDocument) ContentType() string {
	return d.contentType
}

func (d *TextDocument) SetName(name string) {
	d.name = name
}

func (d *TextDocument) SetPath(path string) {
	d.path = path
}

func (d *TextDocument) SetContent(content string) error {
	d.content = content
	return nil
}

func (d *TextDocument) IncrementVersion() {
	d.version++
}

func (d *TextDocument) Chunks() []*Chunk {
	// TODO
	return []*Chunk{}
}
