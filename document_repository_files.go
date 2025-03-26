package dive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var _ DocumentRepository = &FileDocumentRepository{}

// FileDocumentRepository implements DocumentRepository using the local file system
type FileDocumentRepository struct {
	rootDir        string
	namedDocuments map[string]string
	mutex          sync.RWMutex
}

// NewFileDocumentRepository creates a new document repository backed by the file system
func NewFileDocumentRepository(rootDir string) (*FileDocumentRepository, error) {
	if rootDir == "" {
		rootDir = "."
	}
	// Clean and get absolute path for root directory
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path for root directory: %w", err)
	}
	if err := os.MkdirAll(absRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}
	return &FileDocumentRepository{
		rootDir:        absRoot,
		namedDocuments: make(map[string]string),
	}, nil
}

// RegisterDocument assigns a name to a document path
func (r *FileDocumentRepository) RegisterDocument(ctx context.Context, name, path string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	sanitizedPath, err := r.sanitizePath(path)
	if err != nil {
		return fmt.Errorf("failed to sanitize path: %w", err)
	}
	// Convert the sanitized absolute path back to a relative path from root
	relPath, err := filepath.Rel(r.rootDir, sanitizedPath)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %w", err)
	}
	r.namedDocuments[name] = relPath
	return nil
}

// sanitizePath ensures a path is safe and within the root directory
func (r *FileDocumentRepository) sanitizePath(path string) (string, error) {
	// Clean the path to remove any . or .. components
	path = filepath.Clean(path)

	// If path is absolute, make it relative to root
	if filepath.IsAbs(path) {
		// Convert absolute path to be relative to root
		path = strings.TrimPrefix(path, "/")
	}

	// Join with root and clean again
	fullPath := filepath.Join(r.rootDir, path)

	// Verify the path is within root directory
	if !strings.HasPrefix(filepath.Clean(fullPath), r.rootDir) {
		return "", fmt.Errorf("path %q attempts to escape root directory", path)
	}
	return fullPath, nil
}

// GetDocument returns a document by name (which is treated as a path)
func (r *FileDocumentRepository) GetDocument(ctx context.Context, name string) (Document, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Look up the path if this is the name of a registered document
	if registeredPath, ok := r.namedDocuments[name]; ok {
		name = registeredPath
	}

	fullPath, err := r.sanitizePath(name)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("document %q does not exist", name)
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read document %q: %w", name, err)
	}
	return NewTextDocument(TextDocumentOptions{
		Name:        name,
		Path:        name, // Keep original path for reference
		Content:     string(content),
		ContentType: detectContentType(name),
	}), nil
}

// ListDocuments lists documents matching the input criteria
func (r *FileDocumentRepository) ListDocuments(ctx context.Context, input *ListDocumentInput) (*ListDocumentOutput, error) {
	var docs []Document

	startPath, err := r.sanitizePath(input.PathPrefix)
	if err != nil {
		return nil, err
	}

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Get path relative to root dir for storage
		relPath, err := filepath.Rel(r.rootDir, path)
		if err != nil {
			return err
		}
		if input.PathPrefix != "" && !strings.HasPrefix(relPath, input.PathPrefix) {
			return nil
		}
		// Skip directories unless this is the start dir
		if info.IsDir() {
			// If not recursive and this isn't the start dir, skip this directory
			if !input.Recursive && path != startPath {
				return filepath.SkipDir
			}
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil // Skip files we can't read
		}
		doc := NewTextDocument(TextDocumentOptions{
			Name:        filepath.Base(relPath),
			Path:        relPath,
			Content:     string(content),
			ContentType: detectContentType(relPath),
		})
		docs = append(docs, doc)
		return nil
	}

	err = filepath.Walk(startPath, walkFn)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to walk directory %q: %w", input.PathPrefix, err)
	}
	return &ListDocumentOutput{Items: docs}, nil
}

// PutDocument puts a document into the store
func (r *FileDocumentRepository) PutDocument(ctx context.Context, doc Document) error {
	if doc.Path() == "" {
		return fmt.Errorf("document path is required")
	}
	fullPath, err := r.sanitizePath(doc.Path())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for document: %w", err)
	}
	if err := os.WriteFile(fullPath, []byte(doc.Content()), 0644); err != nil {
		return fmt.Errorf("failed to write document to file: %w", err)
	}
	return nil
}

// DeleteDocument deletes a document from the store
func (r *FileDocumentRepository) DeleteDocument(ctx context.Context, doc Document) error {
	if doc.Path() == "" {
		return fmt.Errorf("document path is required")
	}
	fullPath, err := r.sanitizePath(doc.Path())
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete document file: %w", err)
	}
	return nil
}

// Exists checks if a document exists by name
func (r *FileDocumentRepository) Exists(ctx context.Context, name string) (bool, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Look up the path if this is the name of a registered document
	if registeredPath, ok := r.namedDocuments[name]; ok {
		name = registeredPath
	}
	fullPath, err := r.sanitizePath(name)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(fullPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check if document exists: %w", err)
	}
	return true, nil
}

func detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown":
		return "text/markdown"
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".html", ".htm":
		return "text/html"
	default:
		return "text/plain"
	}
}
