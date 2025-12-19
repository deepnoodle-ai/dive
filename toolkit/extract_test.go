package toolkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractTool_Name(t *testing.T) {
	tool := NewExtractTool(ExtractToolOptions{})
	require.Equal(t, "extract", tool.Name())
}

func TestExtractTool_Description(t *testing.T) {
	tool := NewExtractTool(ExtractToolOptions{})
	desc := tool.Description()
	require.Contains(t, desc, "Extract structured data")
	require.Contains(t, desc, "JSON schema")
}

func TestExtractTool_Schema(t *testing.T) {
	tool := NewExtractTool(ExtractToolOptions{})
	schema := tool.Schema()
	require.Equal(t, "object", string(schema.Type))
	require.Contains(t, schema.Required, "input_path")
	require.Contains(t, schema.Required, "schema")
	require.NotEmpty(t, schema.Properties)
}

func TestExtractTool_CallWithMissingInputPath(t *testing.T) {
	tool := &ExtractTool{}
	input := &ExtractInput{
		Schema: map[string]interface{}{"type": "object"},
	}

	result, err := tool.Call(context.Background(), input)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].Text, "No input path provided")
}

func TestExtractTool_CallWithMissingSchema(t *testing.T) {
	tool := &ExtractTool{}
	input := &ExtractInput{
		InputPath: "test.txt",
	}

	result, err := tool.Call(context.Background(), input)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].Text, "No schema provided")
}

func TestExtractTool_CallWithNonexistentFile(t *testing.T) {
	tool := &ExtractTool{maxFileSize: 1024 * 1024}
	input := &ExtractInput{
		InputPath: "nonexistent.txt",
		Schema:    map[string]interface{}{"type": "object"},
	}

	result, err := tool.Call(context.Background(), input)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].Text, "File not found")
}

func TestExtractTool_CallWithTextFile(t *testing.T) {
	// Create a temporary text file
	tempDir := t.TempDir()
	textFile := filepath.Join(tempDir, "test.txt")
	testContent := "Hello, World! This is a test document with some data."
	err := os.WriteFile(textFile, []byte(testContent), 0644)
	require.NoError(t, err)

	tool := &ExtractTool{maxFileSize: 1024 * 1024}
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"message": map[string]interface{}{
				"type":        "string",
				"description": "The main message in the text",
			},
		},
		"required": []string{"message"},
	}

	input := &ExtractInput{
		InputPath: textFile,
		Schema:    schema,
	}

	result, err := tool.Call(context.Background(), input)
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Parse the result
	var analysisResult map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &analysisResult)
	require.NoError(t, err)

	require.Equal(t, "text", analysisResult["file_type"])
	require.Equal(t, textFile, analysisResult["file_path"])
	require.Equal(t, testContent, analysisResult["content"])

	// Check schema structure instead of exact equality due to JSON unmarshaling type differences
	extractedSchema, ok := analysisResult["schema"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "object", extractedSchema["type"])
	require.NotNil(t, extractedSchema["properties"])
	require.NotNil(t, extractedSchema["required"])
}

func TestExtractTool_CallWithBiasFilter(t *testing.T) {
	// Create a temporary text file
	tempDir := t.TempDir()
	textFile := filepath.Join(tempDir, "test.txt")
	testContent := "John is a doctor and Mary is a nurse."
	err := os.WriteFile(textFile, []byte(testContent), 0644)
	require.NoError(t, err)

	tool := &ExtractTool{maxFileSize: 1024 * 1024}
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"people": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":       map[string]interface{}{"type": "string"},
						"profession": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}

	input := &ExtractInput{
		InputPath:  textFile,
		Schema:     schema,
		BiasFilter: "avoid gender-based assumptions about professions",
	}

	result, err := tool.Call(context.Background(), input)
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Parse the result
	var analysisResult map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &analysisResult)
	require.NoError(t, err)

	require.Equal(t, "avoid gender-based assumptions about professions", analysisResult["bias_filter"])
	require.Contains(t, analysisResult["extraction_guidelines"], "Apply bias filtering: avoid gender-based assumptions about professions")
}

func TestExtractTool_CallWithCustomInstructions(t *testing.T) {
	// Create a temporary text file
	tempDir := t.TempDir()
	textFile := filepath.Join(tempDir, "test.txt")
	testContent := "Revenue: $100,000\nExpenses: $75,000\nProfit: $25,000"
	err := os.WriteFile(textFile, []byte(testContent), 0644)
	require.NoError(t, err)

	tool := &ExtractTool{maxFileSize: 1024 * 1024}
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"financial_data": map[string]interface{}{
				"type": "object",
			},
		},
	}

	input := &ExtractInput{
		InputPath:    textFile,
		Schema:       schema,
		Instructions: "focus only on monetary values and convert to numbers",
	}

	result, err := tool.Call(context.Background(), input)
	require.NoError(t, err)
	require.False(t, result.IsError)

	// Parse the result
	var analysisResult map[string]interface{}
	err = json.Unmarshal([]byte(result.Content[0].Text), &analysisResult)
	require.NoError(t, err)

	require.Equal(t, "focus only on monetary values and convert to numbers", analysisResult["custom_instructions"])
	require.Contains(t, analysisResult["extraction_guidelines"], "Additional instructions: focus only on monetary values and convert to numbers")
}

func TestExtractTool_DetectFileType(t *testing.T) {
	tool := &ExtractTool{}

	tests := []struct {
		name     string
		path     string
		content  []byte
		expected string
	}{
		{"PDF file", "document.pdf", []byte("%PDF-1.4"), "pdf"},
		{"Text file", "document.txt", []byte("Hello world"), "text"},
		{"JSON file", "data.json", []byte(`{"key": "value"}`), "text"},
		{"Image file", "image.jpg", []byte("\xFF\xD8\xFF"), "image"},
		{"Markdown file", "readme.md", []byte("# Title"), "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tool.detectFileType(tt.path, tt.content)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractTool_GetContentPreview(t *testing.T) {
	tool := &ExtractTool{}

	tests := []struct {
		name     string
		content  []byte
		fileType string
		expected string
	}{
		{
			"Short text",
			[]byte("Hello world"),
			"text",
			"Hello world",
		},
		{
			"Long text",
			[]byte(strings.Repeat("a", 600)),
			"text",
			strings.Repeat("a", 500) + "...",
		},
		{
			"Image",
			[]byte("image data"),
			"image",
			"Image file (10 bytes)",
		},
		{
			"PDF",
			[]byte("pdf data"),
			"pdf",
			"PDF file (8 bytes)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tool.getContentPreview(tt.content, tt.fileType)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractTool_CallWithLargeFile(t *testing.T) {
	// Create a temporary file that exceeds the size limit
	tempDir := t.TempDir()
	largeFile := filepath.Join(tempDir, "large.txt")

	// Create content larger than the limit
	largeContent := make([]byte, 1024) // 1KB
	for i := range largeContent {
		largeContent[i] = 'a'
	}
	err := os.WriteFile(largeFile, largeContent, 0644)
	require.NoError(t, err)

	tool := &ExtractTool{maxFileSize: 512} // 512 byte limit
	schema := map[string]interface{}{"type": "object"}

	input := &ExtractInput{
		InputPath: largeFile,
		Schema:    schema,
	}

	result, err := tool.Call(context.Background(), input)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].Text, "too large")
}

func TestExtractTool_Annotations(t *testing.T) {
	tool := &ExtractTool{}
	annotations := tool.Annotations()
	require.Equal(t, "extract", annotations.Title)
	require.True(t, annotations.ReadOnlyHint)
	require.False(t, annotations.DestructiveHint)
	require.True(t, annotations.IdempotentHint)
	require.False(t, annotations.OpenWorldHint)
}
