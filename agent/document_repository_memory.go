package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive"
)

var _ dive.DocumentRepository = &MemoryDocumentRepository{}

// MemoryDocumentRepository implements DocumentRepository interface using an in-memory map
type MemoryDocumentRepository struct {
	mu             sync.RWMutex
	documents      map[string]*dive.TextDocument
	namedDocuments map[string]string
}

// NewMemoryDocumentRepository creates a new MemoryDocumentRepository
func NewMemoryDocumentRepository() *MemoryDocumentRepository {
	return &MemoryDocumentRepository{
		documents:      make(map[string]*dive.TextDocument),
		namedDocuments: make(map[string]string),
	}
}

// RegisterDocument assigns a name to a document path
func (r *MemoryDocumentRepository) RegisterDocument(ctx context.Context, name, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.namedDocuments[name] = path
	return nil
}

// GetDocument returns a document by name
func (r *MemoryDocumentRepository) GetDocument(ctx context.Context, name string) (dive.Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Look up the path if this is the name of a registered document
	if registeredPath, ok := r.namedDocuments[name]; ok {
		name = registeredPath
	}

	doc, exists := r.documents[name]
	if !exists {
		return nil, fmt.Errorf("document not found: %s", name)
	}
	return doc, nil
}

// ListDocuments lists documents matching the given criteria
func (r *MemoryDocumentRepository) ListDocuments(ctx context.Context, input *dive.ListDocumentInput) (*dive.ListDocumentOutput, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []dive.Document

	for _, doc := range r.documents {
		// Check path prefix if specified
		if input.PathPrefix != "" {
			if !strings.HasPrefix(doc.Path(), input.PathPrefix) {
				continue
			}
			// If not recursive, ensure there are no additional path segments
			if !input.Recursive {
				remainingPath := strings.TrimPrefix(doc.Path(), input.PathPrefix)
				if strings.Contains(remainingPath, "/") {
					continue
				}
			}
		}
		items = append(items, doc)
	}

	return &dive.ListDocumentOutput{Items: items}, nil
}

// PutDocument stores a document
func (r *MemoryDocumentRepository) PutDocument(ctx context.Context, doc dive.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	textDoc, ok := doc.(*dive.TextDocument)
	if !ok {
		// If not already a TextDocument, create a new one with the same properties
		textDoc = dive.NewTextDocument(dive.TextDocumentOptions{
			ID:          doc.ID(),
			Name:        doc.Name(),
			Description: doc.Description(),
			Path:        doc.Path(),
			Version:     doc.Version(),
			Content:     doc.Content(),
			ContentType: doc.ContentType(),
		})
	}

	r.documents[doc.Name()] = textDoc
	return nil
}

// DeleteDocument removes a document
func (r *MemoryDocumentRepository) DeleteDocument(ctx context.Context, doc dive.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.documents[doc.Name()]; !exists {
		return fmt.Errorf("document not found: %s", doc.Name())
	}

	delete(r.documents, doc.Name())
	return nil
}

// Exists checks if a document exists by name
func (r *MemoryDocumentRepository) Exists(ctx context.Context, name string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Look up the path if this is the name of a registered document
	if registeredPath, ok := r.namedDocuments[name]; ok {
		name = registeredPath
	}

	_, exists := r.documents[name]
	return exists, nil
}
