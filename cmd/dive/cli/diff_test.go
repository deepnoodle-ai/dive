package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/stretchr/testify/require"
)

func TestReadFileContent(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "dive_diff_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test reading regular file
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, world!\nThis is a test file."
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	content, err := readFileContent(testFile)
	require.NoError(t, err)
	require.Equal(t, testContent, content)

	// Test reading non-existent file
	_, err = readFileContent(filepath.Join(tempDir, "nonexistent.txt"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "file does not exist")
}

func TestBuildDiffPrompt(t *testing.T) {
	// Create a sample unified diff
	unifiedDiff := `--- old.txt	original
+++ new.txt	modified
@@ -1 +1 @@
-Hello world
+Hello beautiful world`
	oldFile := "old.txt"
	newFile := "new.txt"

	tests := []struct {
		name         string
		outputFormat string
		expectedText string
	}{
		{
			name:         "text format",
			outputFormat: "text",
			expectedText: "Format your response as clear, readable text with appropriate sections.",
		},
		{
			name:         "markdown format",
			outputFormat: "markdown",
			expectedText: "Format your response using Markdown with clear headings, bullet points, and code blocks where appropriate.",
		},
		{
			name:         "json format",
			outputFormat: "json",
			expectedText: "Format your response as a JSON object with fields: summary (string), changes (array of objects with 'type', 'description', 'impact'), patterns (array of strings), recommendations (array of strings).",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := buildDiffPrompt(unifiedDiff, oldFile, newFile, tt.outputFormat)

			// Check that the prompt contains the expected format instruction
			require.Contains(t, prompt, tt.expectedText)

			// Check that the prompt contains the diff and file names
			require.Contains(t, prompt, unifiedDiff)
			require.Contains(t, prompt, oldFile)
			require.Contains(t, prompt, newFile)

			// Check that it contains the analysis instruction
			require.Contains(t, prompt, "Analyze this unified diff")
			require.Contains(t, prompt, "What changed and why it matters")
		})
	}
}

func TestBuildDiffSystemPrompt(t *testing.T) {
	prompt := buildDiffSystemPrompt()

	// Check that the system prompt contains key elements for diff analysis
	require.Contains(t, prompt, "expert diff analyzer")
	require.Contains(t, prompt, "unified diff")
	require.Contains(t, prompt, "'+' are additions")
	require.Contains(t, prompt, "'-' are deletions")
	require.Contains(t, prompt, "Content Analysis")
	require.Contains(t, prompt, "Semantic Impact")
	require.Contains(t, prompt, "Pattern Recognition")
	require.NotEmpty(t, prompt)
}

func TestHelperFunctions(t *testing.T) {
	// Test floatPtr
	f := 0.5
	ptr := floatPtr(f)
	require.NotNil(t, ptr)
	require.Equal(t, f, *ptr)

	// Test intPtr
	i := 42
	iPtr := intPtr(i)
	require.NotNil(t, iPtr)
	require.Equal(t, i, *iPtr)
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"single line", "hello", 1},
		{"two lines", "hello\nworld", 2},
		{"three lines with empty", "hello\n\nworld", 3},
		{"trailing newline", "hello\nworld\n", 3},
		{"multiple newlines", "\n\n\n", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countLines(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateUnifiedDiff(t *testing.T) {
	tests := []struct {
		name         string
		oldContent   string
		newContent   string
		contextLines int
		expectEmpty  bool
	}{
		{
			name:         "simple change",
			oldContent:   "Hello world",
			newContent:   "Hello beautiful world",
			contextLines: 3,
			expectEmpty:  false,
		},
		{
			name:         "identical files",
			oldContent:   "Same content",
			newContent:   "Same content",
			contextLines: 3,
			expectEmpty:  true,
		},
		{
			name:         "multiline diff",
			oldContent:   "Line 1\nLine 2\nLine 3",
			newContent:   "Line 1\nModified Line 2\nLine 3\nLine 4",
			contextLines: 1,
			expectEmpty:  false,
		},
		{
			name:         "empty to content",
			oldContent:   "",
			newContent:   "New content",
			contextLines: 3,
			expectEmpty:  false,
		},
		{
			name:         "content to empty",
			oldContent:   "Old content",
			newContent:   "",
			contextLines: 3,
			expectEmpty:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := generateUnifiedDiff(tt.oldContent, tt.newContent, "old.txt", "new.txt", tt.contextLines)

			if tt.expectEmpty {
				require.Empty(t, diff)
			} else {
				require.NotEmpty(t, diff)
				// Check for unified diff format markers
				require.Contains(t, diff, "---")
				require.Contains(t, diff, "+++")
				require.Contains(t, diff, "@@")
			}
		})
	}
}

func TestReadFileContentEdgeCases(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "dive_diff_edge_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test empty file
	emptyFile := filepath.Join(tempDir, "empty.txt")
	err = os.WriteFile(emptyFile, []byte(""), 0644)
	require.NoError(t, err)

	content, err := readFileContent(emptyFile)
	require.NoError(t, err)
	require.Equal(t, "", content)

	// Test file with special characters
	specialFile := filepath.Join(tempDir, "special.txt")
	specialContent := "Hello 🌍\nLine with\ttabs\nUnicode: αβγ"
	err = os.WriteFile(specialFile, []byte(specialContent), 0644)
	require.NoError(t, err)

	content, err = readFileContent(specialFile)
	require.NoError(t, err)
	require.Equal(t, specialContent, content)

	// Test binary file (should still work as we read as text)
	binaryFile := filepath.Join(tempDir, "binary.bin")
	binaryContent := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	err = os.WriteFile(binaryFile, binaryContent, 0644)
	require.NoError(t, err)

	content, err = readFileContent(binaryFile)
	require.NoError(t, err)
	require.Equal(t, string(binaryContent), content)
}

func TestRunDiffBasic(t *testing.T) {
	// Create temporary test files
	tempDir, err := os.MkdirTemp("", "dive_diff_integration_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	oldFile := filepath.Join(tempDir, "old.txt")
	newFile := filepath.Join(tempDir, "new.txt")

	err = os.WriteFile(oldFile, []byte("Hello world"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(newFile, []byte("Hello beautiful world"), 0644)
	require.NoError(t, err)

	// Note: Testing with actual AI analysis would require API keys
	// Here we just verify file reading works
	// In a real test environment with API keys, this would work:
	// err = runDiff(context.Background(), oldFile, newFile, "text", 3, "anthropic", "")

	// Test with identical files (should detect no changes early)
	err = runDiff(context.TODO(), oldFile, oldFile, "text", 3, "anthropic", "")
	require.NoError(t, err)
}

func TestBuildDiffPromptEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		unifiedDiff  string
		outputFormat string
	}{
		{
			name:         "empty diff",
			unifiedDiff:  "",
			outputFormat: "text",
		},
		{
			name: "simple addition",
			unifiedDiff: `--- old.txt
+++ new.txt
@@ -1,0 +1 @@
+New content`,
			outputFormat: "text",
		},
		{
			name:         "large diff",
			unifiedDiff:  strings.Repeat("@@ -1,1 +1,1 @@\n-old line\n+new line\n", 100),
			outputFormat: "markdown",
		},
		{
			name: "complex multiline diff",
			unifiedDiff: `--- old.txt
+++ new.txt
@@ -1,3 +1,4 @@
 Line 1
-Line 2
+Modified Line 2
 Line 3
+Line 4`,
			outputFormat: "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := buildDiffPrompt(tt.unifiedDiff, "old.txt", "new.txt", tt.outputFormat)

			// Basic checks
			require.NotEmpty(t, prompt)
			require.Contains(t, prompt, "Analyze this unified diff")
			if tt.unifiedDiff != "" {
				require.Contains(t, prompt, tt.unifiedDiff)
			}
		})
	}
}

func TestUnifiedDiffLibIntegration(t *testing.T) {
	// Test that we can properly use the difflib library
	oldLines := []string{"Hello", "world", "!"}
	newLines := []string{"Hello", "beautiful", "world", "!"}

	diff := difflib.UnifiedDiff{
		A:        oldLines,
		B:        newLines,
		FromFile: "test_old.txt",
		ToFile:   "test_new.txt",
		FromDate: "original",
		ToDate:   "modified",
		Context:  2,
	}

	result, err := difflib.GetUnifiedDiffString(diff)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	// Verify the diff contains expected markers
	require.Contains(t, result, "--- test_old.txt")
	require.Contains(t, result, "+++ test_new.txt")
	require.Contains(t, result, "@@")
	require.Contains(t, result, "+beautiful")
}
