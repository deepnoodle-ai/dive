package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestBuildContextContent(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "This is test content for context"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Create a test image file path (just for path testing, not actual image)
	testImage := filepath.Join(tmpDir, "test.png")
	err = os.WriteFile(testImage, []byte("fake-png-data"), 0644)
	require.NoError(t, err)

	tests := []struct {
		name     string
		entries  []Content
		expected int
		wantErr  bool
	}{
		{
			name:     "empty entries",
			entries:  []Content{},
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "nil entries",
			entries:  nil,
			expected: 0,
			wantErr:  false,
		},
		{
			name: "inline text entry",
			entries: []Content{
				{Text: "This is inline context"},
			},
			expected: 1,
			wantErr:  false,
		},
		{
			name: "local text file",
			entries: []Content{
				{Path: testFile},
			},
			expected: 1,
			wantErr:  false,
		},
		{
			name: "local image file",
			entries: []Content{
				{Path: testImage},
			},
			expected: 1,
			wantErr:  false,
		},
		{
			name: "remote URL",
			entries: []Content{
				{URL: "https://example.com/test.pdf"},
			},
			expected: 1,
			wantErr:  false,
		},
		{
			name: "remote image URL",
			entries: []Content{
				{URL: "https://example.com/image.png"},
			},
			expected: 1,
			wantErr:  false,
		},
		{
			name: "mixed entries",
			entries: []Content{
				{Path: testFile},
				{Text: "Text text"},
				{URL: "https://example.com/doc.pdf"},
			},
			expected: 3,
			wantErr:  false,
		},
		{
			name: "empty context entry",
			entries: []Content{
				{}, // All fields empty
			},
			expected: 0,
			wantErr:  true,
		},
		{
			name: "multiple fields set",
			entries: []Content{
				{Text: "some text", Path: testFile}, // Both Text and Path set
			},
			expected: 0,
			wantErr:  true,
		},
		{
			name: "all fields set",
			entries: []Content{
				{
					Text: "some text",
					Path: testFile,
					URL:  "https://example.com/doc.pdf",
				},
			},
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := buildContextContent(context.Background(), nil, "", tt.entries)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, messages, tt.expected)
		})
	}
}

func TestBuildMessageFromString(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "This is test content"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	tests := []struct {
		name        string
		input       string
		expectType  string // "text", "image", "document"
		expectError bool
	}{
		{
			name:        "local text file",
			input:       testFile,
			expectType:  "text",
			expectError: false,
		},
		{
			name:        "remote URL",
			input:       "https://example.com/doc.pdf",
			expectType:  "document",
			expectError: false,
		},
		{
			name:        "remote image URL",
			input:       "https://example.com/image.png",
			expectType:  "image",
			expectError: false,
		},
		{
			name:        "non-existent file (treated as literal)",
			input:       "/non/existent/path.txt",
			expectType:  "text",
			expectError: false,
		},
		{
			name:        "plain text literal",
			input:       "Just some text",
			expectType:  "text",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := buildMessageFromLocalFile(tt.input)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			switch tt.expectType {
			case "text":
				_, ok := msg.(*llm.TextContent)
				require.True(t, ok, "Expected TextContent")
			case "image":
				_, ok := msg.(*llm.ImageContent)
				require.True(t, ok, "Expected ImageContent")
			case "document":
				_, ok := msg.(*llm.DocumentContent)
				require.True(t, ok, "Expected DocumentContent")
			}
		})
	}
}

func TestBuildMessageFromLocalFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Test text file
	textFile := filepath.Join(tmpDir, "test.txt")
	textContent := "Hello world"
	err := os.WriteFile(textFile, []byte(textContent), 0644)
	require.NoError(t, err)

	msg, err := buildMessageFromLocalFile(textFile)
	require.NoError(t, err)

	textContentBlock, ok := msg.(*llm.TextContent)
	require.True(t, ok)
	require.Equal(t, textContent, textContentBlock.Text)

	// Test image file (fake PNG)
	imageFile := filepath.Join(tmpDir, "test.png")
	err = os.WriteFile(imageFile, []byte("fake-png-data"), 0644)
	require.NoError(t, err)

	msg, err = buildMessageFromLocalFile(imageFile)
	require.NoError(t, err)

	imageContent, ok := msg.(*llm.ImageContent)
	require.True(t, ok)
	require.Equal(t, llm.ContentSourceTypeURL, imageContent.Source.Type)
	require.Contains(t, imageContent.Source.URL, "file://")

	// Test unknown extension (should become document)
	unknownFile := filepath.Join(tmpDir, "test.xyz")
	err = os.WriteFile(unknownFile, []byte("unknown content"), 0644)
	require.NoError(t, err)

	msg, err = buildMessageFromLocalFile(unknownFile)
	require.NoError(t, err)

	docContent, ok := msg.(*llm.DocumentContent)
	require.True(t, ok)
	require.Equal(t, llm.ContentSourceTypeURL, docContent.Source.Type)
	require.Contains(t, docContent.Source.URL, "file://")
}

func TestIsImageExt(t *testing.T) {
	imageExts := []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"}
	nonImageExts := []string{".txt", ".pdf", ".doc", ".md", ""}

	for _, ext := range imageExts {
		require.True(t, isImageExt(ext), "Expected %s to be recognized as image extension", ext)
	}

	for _, ext := range nonImageExts {
		require.False(t, isImageExt(ext), "Expected %s to NOT be recognized as image extension", ext)
	}
}

func TestFileURI(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "relative path",
			path:     "test.txt",
			expected: "file://",
		},
		{
			name:     "absolute path",
			path:     "/tmp/test.txt",
			expected: "file:///tmp/test.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fileURI(tt.path)
			require.Contains(t, result, tt.expected)
		})
	}
}
