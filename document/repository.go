package document

import "context"

// ListDocumentInput specifies search criteria for documents in a document store
type ListDocumentInput struct {
	PathPrefix string
	Recursive  bool
}

// ListDocumentOutput is the output for listing documents
type ListDocumentOutput struct {
	Items []Document
}

// Repository provides read/write access to a set of documents. This could be
// backed by a local file system, a remote API, or a database.
type Repository interface {

	// GetDocument returns a document by name
	GetDocument(ctx context.Context, name string) (Document, error)

	// ListDocuments lists documents
	ListDocuments(ctx context.Context, input *ListDocumentInput) (*ListDocumentOutput, error)

	// PutDocument puts a document
	PutDocument(ctx context.Context, doc Document) error

	// DeleteDocument deletes a document
	DeleteDocument(ctx context.Context, doc Document) error

	// Exists checks if a document exists by name
	Exists(ctx context.Context, name string) (bool, error)
}
