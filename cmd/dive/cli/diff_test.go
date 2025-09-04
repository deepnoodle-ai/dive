package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	oldContent := "Hello world"
	newContent := "Hello beautiful world"
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
			expectedText: "Format your response as clear, readable text.",
		},
		{
			name:         "markdown format",
			outputFormat: "markdown",
			expectedText: "Format your response using Markdown with clear headings and structure.",
		},
		{
			name:         "json format",
			outputFormat: "json",
			expectedText: "Format your response as a JSON object with fields: summary, changes (array), impact_assessment.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := buildDiffPrompt(oldContent, newContent, oldFile, newFile, tt.outputFormat)
			
			// Check that the prompt contains the expected format instruction
			require.Contains(t, prompt, tt.expectedText)
			
			// Check that the prompt contains the file contents
			require.Contains(t, prompt, oldContent)
			require.Contains(t, prompt, newContent)
			require.Contains(t, prompt, oldFile)
			require.Contains(t, prompt, newFile)
			
			// Check that it contains the analysis instruction
			require.Contains(t, prompt, "semantic differences")
		})
	}
}

func TestBuildDiffSystemPrompt(t *testing.T) {
	prompt := buildDiffSystemPrompt()
	
	// Check that the system prompt contains key elements
	require.Contains(t, prompt, "semantic diff analyzer")
	require.Contains(t, prompt, "Content Changes")
	require.Contains(t, prompt, "Structural Changes")
	require.Contains(t, prompt, "Semantic Meaning")
	require.Contains(t, prompt, "Impact Assessment")
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

func TestDiffCmdFlags(t *testing.T) {
	// Test that the diff command has the expected flags
	require.NotNil(t, diffCmd)
	require.Equal(t, "diff [old_file] [new_file]", diffCmd.Use)
	require.Contains(t, diffCmd.Short, "Semantic diff")
	
	// Check flags exist
	explainFlag := diffCmd.Flags().Lookup("explain-changes")
	require.NotNil(t, explainFlag)
	require.Equal(t, "false", explainFlag.DefValue)
	
	formatFlag := diffCmd.Flags().Lookup("format")
	require.NotNil(t, formatFlag)
	require.Equal(t, "text", formatFlag.DefValue)
}

func TestDiffCmdRegistration(t *testing.T) {
	// Test that the diff command is properly registered with the root command
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "diff" {
			found = true
			break
		}
	}
	require.True(t, found, "diff command should be registered with root command")
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

	// Test basic diff without AI (should not require API keys)
	err = runDiff(nil, oldFile, newFile, false, "text", "anthropic", "")
	require.NoError(t, err)

	// Test with identical files
	err = runDiff(nil, oldFile, oldFile, false, "text", "anthropic", "")
	require.NoError(t, err)
}

func TestBuildDiffPromptEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		oldContent   string
		newContent   string
		outputFormat string
	}{
		{
			name:         "empty files",
			oldContent:   "",
			newContent:   "",
			outputFormat: "text",
		},
		{
			name:         "one empty file",
			oldContent:   "",
			newContent:   "New content",
			outputFormat: "text",
		},
		{
			name:         "large content",
			oldContent:   strings.Repeat("A", 1000),
			newContent:   strings.Repeat("B", 1000),
			outputFormat: "markdown",
		},
		{
			name:         "multiline content",
			oldContent:   "Line 1\nLine 2\nLine 3",
			newContent:   "Line 1\nModified Line 2\nLine 3\nLine 4",
			outputFormat: "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := buildDiffPrompt(tt.oldContent, tt.newContent, "old.txt", "new.txt", tt.outputFormat)
			
			// Basic checks
			require.NotEmpty(t, prompt)
			require.Contains(t, prompt, "semantic differences")
			require.Contains(t, prompt, tt.oldContent)
			require.Contains(t, prompt, tt.newContent)
		})
	}
}