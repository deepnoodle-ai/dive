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

	// Check schema structure
	require.Equal(t, "object", string(loadedSchema.Type))
	require.NotNil(t, loadedSchema.Properties)
	require.NotEmpty(t, loadedSchema.Required)
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

func TestParseFieldSpec(t *testing.T) {
	tests := []struct {
		name          string
		fieldSpec     string
		expectedName  string
		expectedType  string
		expectedError bool
	}{
		{"Simple field name", "name", "name", "string", false},
		{"Field with string type", "name:string", "name", "string", false},
		{"Field with int type", "age:int", "age", "int", false},
		{"Field with boolean type", "active:bool", "active", "bool", false},
		{"Field with whitespace", " name : string ", "name", "string", false},
		{"Empty field name", ":string", "", "", true},
		{"Empty type", "name:", "", "", true},
		{"Empty spec", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, fieldType, err := parseFieldSpec(tt.fieldSpec)
			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedName, name)
				require.Equal(t, tt.expectedType, fieldType)
			}
		})
	}
}

func TestCreatePropertyFromType(t *testing.T) {
	tests := []struct {
		name         string
		typeStr      string
		expectedType string
		expectError  bool
	}{
		{"String type", "string", "string", false},
		{"Boolean type", "bool", "boolean", false},
		{"Boolean type alt", "boolean", "boolean", false},
		{"Integer type", "int", "integer", false},
		{"Integer type alt", "integer", "integer", false},
		{"Float type", "float", "number", false},
		{"Number type", "number", "number", false},
		{"Object type", "object", "object", false},
		{"Array of string", "array of string", "array", false},
		{"Array with brackets", "[int]", "array", false},
		{"Array with angle brackets", "array<bool>", "array", false},
		{"Invalid type", "invalid", "", true},
		{"Invalid array type", "array of invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prop, err := createPropertyFromType(tt.typeStr)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedType, string(prop.Type))

				// Special checks for array types
				if tt.expectedType == "array" {
					require.NotNil(t, prop.Items)
				}
			}
		})
	}
}

func TestParseArrayType(t *testing.T) {
	tests := []struct {
		name     string
		typeStr  string
		expected string
	}{
		{"Array of string", "array of string", "string"},
		{"Array of int", "array of int", "int"},
		{"Array with brackets", "[string]", "string"},
		{"Array with angle brackets", "array<int>", "int"},
		{"Not an array", "string", ""},
		{"Empty array spec", "array of ", ""},
		{"Malformed bracket", "[string", ""},
		{"Malformed angle", "array<string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseArrayType(tt.typeStr)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateSchemaFromFields(t *testing.T) {
	tests := []struct {
		name           string
		fieldsStr      string
		expectedFields map[string]string // field name -> type
		expectError    bool
	}{
		{
			"Simple fields without types",
			"name,age,color",
			map[string]string{"name": "string", "age": "string", "color": "string"},
			false,
		},
		{
			"Fields with types",
			"name:string,age:int,active:bool",
			map[string]string{"name": "string", "age": "integer", "active": "boolean"},
			false,
		},
		{
			"Mixed typed and untyped",
			"name,age:int,active:bool",
			map[string]string{"name": "string", "age": "integer", "active": "boolean"},
			false,
		},
		{
			"Array types",
			"tags:[string],scores:array of int",
			map[string]string{"tags": "array", "scores": "array"},
			false,
		},
		{
			"Empty fields string",
			"",
			nil,
			true,
		},
		{
			"Invalid type",
			"name:invalid",
			nil,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := createSchemaFromFields(tt.fieldsStr)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, "object", string(schema.Type))
				require.Len(t, schema.Properties, len(tt.expectedFields))

				for fieldName, expectedType := range tt.expectedFields {
					prop, exists := schema.Properties[fieldName]
					require.True(t, exists, "Field %s should exist", fieldName)
					require.Equal(t, expectedType, string(prop.Type), "Field %s should have type %s", fieldName, expectedType)
				}

				// All fields should be required
				require.Len(t, schema.Required, len(tt.expectedFields))
			}
		})
	}
}
