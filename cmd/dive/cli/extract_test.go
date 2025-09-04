package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadSchemaFromFile(t *testing.T) {
	tempDir := t.TempDir()
	schemaFile := filepath.Join(tempDir, "test_schema.json")
	
	testSchema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Person's name",
			},
			"age": map[string]interface{}{
				"type":        "integer",
				"description": "Person's age",
			},
		},
		"required": []string{"name"},
	}
	
	schemaJSON, err := json.MarshalIndent(testSchema, "", "  ")
	require.NoError(t, err)
	
	err = os.WriteFile(schemaFile, schemaJSON, 0644)
	require.NoError(t, err)
	
	// Test loading the schema
	loadedSchema, err := loadSchemaFromFile(schemaFile)
	require.NoError(t, err)
	
	// Check schema structure instead of exact equality due to JSON unmarshaling type differences
	require.Equal(t, testSchema["type"], loadedSchema["type"])
	require.NotNil(t, loadedSchema["properties"])
	require.NotNil(t, loadedSchema["required"])
}

func TestLoadSchemaFromFile_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	schemaFile := filepath.Join(tempDir, "invalid_schema.json")
	
	err := os.WriteFile(schemaFile, []byte("invalid json"), 0644)
	require.NoError(t, err)
	
	_, err = loadSchemaFromFile(schemaFile)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid JSON schema")
}

func TestLoadSchemaFromFile_NonexistentFile(t *testing.T) {
	_, err := loadSchemaFromFile("nonexistent.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read schema file")
}

func TestBuildExtractionInstructions(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{"type": "string"},
		},
	}
	
	instructions := buildExtractionInstructions(schema, "", "")
	require.Contains(t, instructions, "data extraction specialist")
	require.Contains(t, instructions, "valid JSON")
	require.Contains(t, instructions, "extract tool")
}

func TestBuildExtractionInstructions_WithBiasFilter(t *testing.T) {
	schema := map[string]interface{}{"type": "object"}
	biasFilter := "avoid gender assumptions"
	
	instructions := buildExtractionInstructions(schema, biasFilter, "")
	require.Contains(t, instructions, "Bias Filtering Requirements: avoid gender assumptions")
}

func TestBuildExtractionInstructions_WithCustomInstructions(t *testing.T) {
	schema := map[string]interface{}{"type": "object"}
	customInstructions := "focus on financial data"
	
	instructions := buildExtractionInstructions(schema, "", customInstructions)
	require.Contains(t, instructions, "Additional Extraction Instructions: focus on financial data")
}

func TestExtractJSONFromResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		expected string
	}{
		{
			"Simple JSON object",
			`Here is the extracted data: {"name": "John", "age": 30}`,
			`{"name": "John", "age": 30}`,
		},
		{
			"JSON array",
			`The results are: [{"name": "John"}, {"name": "Jane"}]`,
			`[{"name": "John"}, {"name": "Jane"}]`,
		},
		{
			"Nested JSON",
			`Response: {"person": {"name": "John", "details": {"age": 30}}}`,
			`{"person": {"name": "John", "details": {"age": 30}}}`,
		},
		{
			"No JSON",
			"This is just text without JSON",
			"This is just text without JSON",
		},
		{
			"Incomplete JSON",
			`Here is data: {"name": "John"`,
			`Here is data: {"name": "John"`,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSONFromResponse(tt.response)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSaveExtractedData(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "subdir", "output.json")
	
	testData := []byte(`{"name": "John", "age": 30}`)
	
	err := saveExtractedData(outputPath, testData)
	require.NoError(t, err)
	
	// Verify file was created and content is correct
	savedData, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Equal(t, testData, savedData)
}