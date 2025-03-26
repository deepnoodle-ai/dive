package document

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

var _ Repository = &MemoryRepository{}

// MemoryRepository implements Repository interface using an in-memory map
type MemoryRepository struct {
	mu             sync.RWMutex
	documents      map[string]*TextDocument
	namedDocuments map[string]string
}

// NewMemoryRepository creates a new MemoryRepository
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		documents:      make(map[string]*TextDocument),
		namedDocuments: make(map[string]string),
	}
}

// RegisterDocument assigns a name to a document path
func (r *MemoryRepository) RegisterDocument(ctx context.Context, name, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.namedDocuments[name] = path
	return nil
}

// GetDocument returns a document by name
func (r *MemoryRepository) GetDocument(ctx context.Context, name string) (Document, error) {
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
func (r *MemoryRepository) ListDocuments(ctx context.Context, input *ListDocumentInput) (*ListDocumentOutput, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var items []Document

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

	return &ListDocumentOutput{Items: items}, nil
}

// PutDocument stores a document
func (r *MemoryRepository) PutDocument(ctx context.Context, doc Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	textDoc, ok := doc.(*TextDocument)
	if !ok {
		// If not already a TextDocument, create a new one with the same properties
		textDoc = New(Options{
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
func (r *MemoryRepository) DeleteDocument(ctx context.Context, doc Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.documents[doc.Name()]; !exists {
		return fmt.Errorf("document not found: %s", doc.Name())
	}

	delete(r.documents, doc.Name())
	return nil
}

// Exists checks if a document exists by name
func (r *MemoryRepository) Exists(ctx context.Context, name string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Look up the path if this is the name of a registered document
	if registeredPath, ok := r.namedDocuments[name]; ok {
		name = registeredPath
	}

	_, exists := r.documents[name]
	return exists, nil
}
