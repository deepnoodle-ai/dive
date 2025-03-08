package document

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileSysRepository(t *testing.T) {
	// Create temp directory for tests
	tmpDir, err := os.MkdirTemp("", "docrepo-test-*")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tmpDir)

	repo, err := NewFileSysRepository(tmpDir)
	require.NoError(t, err, "Failed to create document repo")

	ctx := context.Background()

	// Test PutDocument
	t.Run("PutDocument", func(t *testing.T) {
		doc := New(Options{
			Name:    "test.txt",
			Path:    "docs/test.txt",
			Content: "test content",
		})

		require.NoError(t, repo.PutDocument(ctx, doc))

		// Verify file exists
		content, err := os.ReadFile(filepath.Join(tmpDir, "docs/test.txt"))
		require.NoError(t, err, "Failed to read written file")
		require.Equal(t, "test content", string(content), "File content mismatch")
	})

	// Test GetDocument
	t.Run("GetDocument", func(t *testing.T) {
		doc, err := repo.GetDocument(ctx, "docs/test.txt")
		require.NoError(t, err, "GetDocument failed")

		require.Equal(t, "test content", doc.Content(), "Document content mismatch")
		require.Equal(t, "test.txt", doc.Name(), "Document name mismatch")
	})

	// Test ListDocuments
	t.Run("ListDocuments", func(t *testing.T) {
		// Clean up any existing test files first
		err := os.RemoveAll(filepath.Join(tmpDir, "docs"))
		require.NoError(t, err, "Failed to clean up test directory")

		// Create base docs directory
		err = os.MkdirAll(filepath.Join(tmpDir, "docs"), 0755)
		require.NoError(t, err, "Failed to create docs directory")

		// Add test document that should already exist
		doc := New(Options{
			Name:    "test.txt",
			Path:    "docs/test.txt",
			Content: "test content",
		})
		require.NoError(t, repo.PutDocument(ctx, doc), "Failed to put initial test document")

		// Add more test documents
		docs := []struct {
			uri     string
			content string
		}{
			{"docs/sub1/a.txt", "content a"},
			{"docs/sub1/b.txt", "content b"},
			{"docs/sub2/c.txt", "content c"},
		}

		for _, d := range docs {
			doc := New(Options{
				Name:    filepath.Base(d.uri),
				Path:    d.uri,
				Content: d.content,
			})
			require.NoError(t, repo.PutDocument(ctx, doc), "Failed to put test document")
		}

		tests := []struct {
			name         string
			input        *ListDocumentInput
			wantCount    int
			wantContains string
		}{
			{
				name:         "List all recursively",
				input:        &ListDocumentInput{PathPrefix: "docs", Recursive: true},
				wantCount:    4, // including the original test.txt
				wantContains: "docs/sub1/a.txt",
			},
			{
				name:         "List without recursion",
				input:        &ListDocumentInput{PathPrefix: "docs", Recursive: false},
				wantCount:    1, // only test.txt in root docs/
				wantContains: "test.txt",
			},
			{
				name:         "List specific subdirectory",
				input:        &ListDocumentInput{PathPrefix: "docs/sub1", Recursive: true},
				wantCount:    2,
				wantContains: "a.txt",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := repo.ListDocuments(ctx, tt.input)
				require.NoError(t, err, "ListDocuments failed")
				require.Len(t, result.Items, tt.wantCount, "Unexpected number of documents")

				found := false
				for _, doc := range result.Items {
					if doc.Path() == tt.wantContains || doc.Name() == tt.wantContains {
						found = true
						break
					}
				}
				require.True(t, found, "Expected to find document containing %q", tt.wantContains)
			})
		}
	})

	// Test DeleteDocument
	t.Run("DeleteDocument", func(t *testing.T) {
		doc := New(Options{
			Name:    "delete-test.txt",
			Path:    "docs/delete-test.txt",
			Content: "to be deleted",
		})

		// First put the document
		require.NoError(t, repo.PutDocument(ctx, doc), "Failed to put test document")

		// Then delete it
		require.NoError(t, repo.DeleteDocument(ctx, doc), "DeleteDocument failed")

		// Verify it's gone
		_, err = repo.GetDocument(ctx, "docs/delete-test.txt")
		require.Error(t, err, "Expected error getting deleted document")
	})

	// Test error cases
	t.Run("ErrorCases", func(t *testing.T) {
		// Test getting non-existent document
		_, err := repo.GetDocument(ctx, "nonexistent.txt")
		require.Error(t, err, "Expected error getting non-existent document")

		// Test putting document without URI
		doc := New(Options{
			Name:    "no-uri.txt",
			Content: "test",
		})
		err = repo.PutDocument(ctx, doc)
		require.Error(t, err, "Expected error putting document without URI")
	})
}
