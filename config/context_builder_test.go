package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
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
			messages, err := buildContextContent("", tt.entries)
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
			name:        "non-existent file",
			input:       "/non/existent/path.txt",
			expectType:  "",
			expectError: true, // Non-existent files now cause errors
		},
		{
			name:        "unsupported extension",
			input:       filepath.Join(tmpDir, "file.xyz"),
			expectType:  "",
			expectError: true, // Unsupported extensions cause errors
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the file for unsupported extension test
			if tt.name == "unsupported extension" {
				err := os.WriteFile(tt.input, []byte("content"), 0644)
				require.NoError(t, err)
			}

			msg, err := getContentFromLocalFile(tt.input)
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

	msg, err := getContentFromLocalFile(textFile)
	require.NoError(t, err)

	textContentBlock, ok := msg.(*llm.TextContent)
	require.True(t, ok)
	expectedWrapped := "<file name=\"test.txt\">\n" + textContent + "\n</file>"
	require.Equal(t, expectedWrapped, textContentBlock.Text)

	// Test image file (fake PNG)
	imageFile := filepath.Join(tmpDir, "test.png")
	err = os.WriteFile(imageFile, []byte("fake-png-data"), 0644)
	require.NoError(t, err)

	msg, err = getContentFromLocalFile(imageFile)
	require.NoError(t, err)

	imageContent, ok := msg.(*llm.ImageContent)
	require.True(t, ok)
	require.Equal(t, llm.ContentSourceTypeBase64, imageContent.Source.Type)
	require.Equal(t, "image/png", imageContent.Source.MediaType)
	require.NotEmpty(t, imageContent.Source.Data)

	// Test unknown extension (should return error)
	unknownFile := filepath.Join(tmpDir, "test.xyz")
	err = os.WriteFile(unknownFile, []byte("unknown content"), 0644)
	require.NoError(t, err)

	_, err = getContentFromLocalFile(unknownFile)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported local file extension")
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

func TestBuildContextContentWithWildcards(t *testing.T) {
	// Create a temporary directory with test files
	tempDir, err := os.MkdirTemp("", "context_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test files
	testFiles := []string{
		"file1.txt",
		"file2.txt",
		"file3.md",
		"subdir/nested.txt",
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tempDir, file)
		dir := filepath.Dir(fullPath)
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)

		content := "Content of " + file
		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name            string
		entries         []Content
		expectedCount   int
		expectedPattern string
	}{
		{
			name: "wildcard matches multiple txt files",
			entries: []Content{
				{Path: "*.txt"},
			},
			expectedCount:   2, // file1.txt, file2.txt
			expectedPattern: "Content of file",
		},
		{
			name: "doublestar matches nested files",
			entries: []Content{
				{Path: "**/*.txt"},
			},
			expectedCount:   3, // file1.txt, file2.txt, subdir/nested.txt
			expectedPattern: "Content of",
		},
		{
			name: "specific pattern matches single file",
			entries: []Content{
				{Path: "file1.txt"},
			},
			expectedCount:   1,
			expectedPattern: "Content of file1.txt",
		},
		{
			name: "mixed wildcard and non-wildcard",
			entries: []Content{
				{Path: "*.md"},
				{Path: "file1.txt"},
			},
			expectedCount:   2, // file3.md + file1.txt
			expectedPattern: "Content of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contents, err := buildContextContent(tempDir, tt.entries)
			require.NoError(t, err)
			require.Len(t, contents, tt.expectedCount)

			// Verify all contents are TextContent and contain expected pattern
			for _, content := range contents {
				textContent, ok := content.(*llm.TextContent)
				require.True(t, ok, "Expected TextContent")
				require.Contains(t, textContent.Text, tt.expectedPattern)
			}
		})
	}
}

func TestBuildContextContentWildcardErrors(t *testing.T) {
	// Test with non-existent directory pattern
	entries := []Content{
		{Path: "nonexistent/*.txt"},
	}

	contents, err := buildContextContent("", entries)
	require.NoError(t, err) // FilepathGlob returns empty result for non-matching patterns, not error
	require.Empty(t, contents)
}

func TestContainsWildcards(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"simple.txt", false},
		{"*.txt", true},
		{"file?.txt", true},
		{"[abc].txt", true},
		{"{a,b}.txt", true},
		{"dir/**/*.txt", true},
		{"normal/path/file.txt", false},
		{"path/with*wildcard.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := containsWildcards(tt.path)
			require.Equal(t, tt.expected, result)
		})
	}
}
