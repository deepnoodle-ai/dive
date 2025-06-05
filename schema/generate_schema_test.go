package schema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test types for schema generation
type TestUser struct {
	Name     string   `json:"name" description:"User's full name"`
	Age      int      `json:"age,omitempty" description:"User's age in years" minimum:"0" maximum:"150"`
	Email    string   `json:"email" description:"User's email address" format:"email" required:"true"`
	Tags     []string `json:"tags,omitempty" description:"User tags" maxItems:"10"`
	Active   bool     `json:"active" description:"Whether the user is active"`
	Metadata *string  `json:"metadata,omitempty" description:"Optional metadata"`
}

type TestProduct struct {
	ID          string  `json:"id" pattern:"^[A-Z0-9]+$"`
	Name        string  `json:"name" minLength:"1" maxLength:"100"`
	Price       float64 `json:"price" minimum:"0"`
	Category    string  `json:"category" enum:"electronics,books,clothing"`
	InStock     bool    `json:"in_stock"`
	Description *string `json:"description,omitempty" nullable:"true"`
}

func TestGenerate_SimpleTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected SchemaType
	}{
		{"string", "", String},
		{"int", 0, Integer},
		{"int64", int64(0), Integer},
		{"float64", 0.0, Number},
		{"bool", false, Boolean},
		{"slice", []string{}, Array},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := Generate(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.expected, schema.Type)
		})
	}
}

func TestGenerate_Struct(t *testing.T) {
	schema, err := Generate(TestUser{})
	require.NoError(t, err)

	// Check basic schema properties
	require.Equal(t, Object, schema.Type)
	require.NotNil(t, schema.AdditionalProperties)
	require.False(t, *schema.AdditionalProperties)

	// Check properties exist
	expectedProps := []string{"name", "age", "email", "tags", "active", "metadata"}
	require.Len(t, schema.Properties, len(expectedProps))

	for _, prop := range expectedProps {
		_, exists := schema.Properties[prop]
		require.True(t, exists, "Property %s not found", prop)
	}

	// Check required fields
	expectedRequired := []string{"name", "email", "active"}
	require.Len(t, schema.Required, len(expectedRequired))

	for _, req := range expectedRequired {
		found := false
		for _, r := range schema.Required {
			if r == req {
				found = true
				break
			}
		}
		require.True(t, found, "Required field %s not found", req)
	}
}

func TestGenerate_PropertyTypes(t *testing.T) {
	schema, err := Generate(TestUser{})
	require.NoError(t, err)

	tests := []struct {
		field    string
		expected SchemaType
	}{
		{"name", String},
		{"age", Integer},
		{"email", String},
		{"tags", Array},
		{"active", Boolean},
		{"metadata", String},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			prop, exists := schema.Properties[tt.field]
			require.True(t, exists, "Property %s not found", tt.field)
			require.Equal(t, tt.expected, prop.Type)
		})
	}
}

func TestGenerate_PropertyDescriptions(t *testing.T) {
	schema, err := Generate(TestUser{})
	require.NoError(t, err)

	tests := []struct {
		field       string
		description string
	}{
		{"name", "User's full name"},
		{"age", "User's age in years"},
		{"email", "User's email address"},
		{"tags", "User tags"},
		{"active", "Whether the user is active"},
		{"metadata", "Optional metadata"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			prop, exists := schema.Properties[tt.field]
			require.True(t, exists, "Property %s not found", tt.field)
			require.Equal(t, tt.description, prop.Description)
		})
	}
}

func TestGenerate_PropertyConstraints(t *testing.T) {
	schema, err := Generate(TestProduct{})
	require.NoError(t, err)

	// Test pattern constraint
	idProp := schema.Properties["id"]
	require.NotNil(t, idProp.Pattern)
	require.Equal(t, "^[A-Z0-9]+$", *idProp.Pattern)

	// Test length constraints
	nameProp := schema.Properties["name"]
	require.NotNil(t, nameProp.MinLength)
	require.Equal(t, 1, *nameProp.MinLength)
	require.NotNil(t, nameProp.MaxLength)
	require.Equal(t, 100, *nameProp.MaxLength)

	// Test numeric constraints
	priceProp := schema.Properties["price"]
	require.NotNil(t, priceProp.Minimum)
	require.Equal(t, 0.0, *priceProp.Minimum)

	// Test enum constraint
	categoryProp := schema.Properties["category"]
	expected := []string{"electronics", "books", "clothing"}
	require.Equal(t, expected, categoryProp.Enum)

	// Test nullable constraint
	descProp := schema.Properties["description"]
	require.NotNil(t, descProp.Nullable)
	require.True(t, *descProp.Nullable)
}

func TestGenerate_ArrayType(t *testing.T) {
	schema, err := Generate(TestUser{})
	require.NoError(t, err)

	tagsProp := schema.Properties["tags"]
	require.Equal(t, Array, tagsProp.Type)
	require.NotNil(t, tagsProp.Items)
	require.Equal(t, String, tagsProp.Items.Type)
	require.NotNil(t, tagsProp.MaxItems)
	require.Equal(t, 10, *tagsProp.MaxItems)
}

func TestGenerate_PointerType(t *testing.T) {
	schema, err := Generate(TestUser{})
	require.NoError(t, err)

	metadataProp := schema.Properties["metadata"]
	require.Equal(t, String, metadataProp.Type)
	require.NotNil(t, metadataProp.Nullable)
	require.True(t, *metadataProp.Nullable)
}

func TestGenerate_JSONSerialization(t *testing.T) {
	schema, err := Generate(TestUser{})
	require.NoError(t, err)

	// Test that the schema can be serialized to JSON
	jsonData, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	// Test that it can be deserialized back
	var deserializedSchema Schema
	err = json.Unmarshal(jsonData, &deserializedSchema)
	require.NoError(t, err)

	// Basic validation that the deserialized schema matches
	require.Equal(t, schema.Type, deserializedSchema.Type)
	require.Len(t, deserializedSchema.Properties, len(schema.Properties))
}

func TestGenerate_NilInput(t *testing.T) {
	_, err := Generate(nil)
	require.Error(t, err)
}

func TestGenerate_UnsupportedType(t *testing.T) {
	unsupported := make(chan int)
	_, err := Generate(unsupported)
	require.Error(t, err)
}

type NestedStruct struct {
	Inner struct {
		Value string `json:"value" description:"Inner value"`
		Count int    `json:"count,omitempty"`
	} `json:"inner" description:"Nested inner struct"`
}

func TestGenerate_NestedStruct(t *testing.T) {
	schema, err := Generate(NestedStruct{})
	require.NoError(t, err)

	innerProp := schema.Properties["inner"]
	require.Equal(t, Object, innerProp.Type)
	require.NotNil(t, innerProp.Properties)

	valueProp := innerProp.Properties["value"]
	require.Equal(t, String, valueProp.Type)

	// Check that "value" is required but "count" is not
	valueRequired := false
	countRequired := false
	for _, req := range innerProp.Required {
		if req == "value" {
			valueRequired = true
		}
		if req == "count" {
			countRequired = true
		}
	}

	require.True(t, valueRequired, "Inner value should be required")
	require.False(t, countRequired, "Inner count should not be required")
}

type SimpleTestStruct struct {
	Name   string `json:"name" description:"A name field"`
	Age    int    `json:"age,omitempty" description:"Age in years"`
	Active bool   `json:"active" description:"Whether active"`
}

func TestGenerate_SimpleJSONSerialization(t *testing.T) {
	schema, err := Generate(SimpleTestStruct{})
	require.NoError(t, err)

	jsonData, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	expectedJSON := `{
  "type": "object",
  "properties": {
    "active": {
      "type": "boolean",
      "description": "Whether active"
    },
    "age": {
      "type": "integer",
      "description": "Age in years"
    },
    "name": {
      "type": "string",
      "description": "A name field"
    }
  },
  "required": [
    "name",
    "active"
  ],
  "additionalProperties": false
}`

	require.JSONEq(t, expectedJSON, string(jsonData))
}

type ComplexTestStruct struct {
	ID     string `json:"id" description:"Unique identifier"`
	Config struct {
		MaxRetries int    `json:"max_retries" description:"Maximum retry attempts"`
		Timeout    string `json:"timeout,omitempty" description:"Timeout duration"`
	} `json:"config" description:"Configuration settings"`
	Tags []string `json:"tags,omitempty" description:"List of tags" maxItems:"5"`
}

func TestGenerate_ComplexJSONSerialization(t *testing.T) {
	schema, err := Generate(ComplexTestStruct{})
	require.NoError(t, err)

	jsonData, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	expectedJSON := `{
  "type": "object",
  "properties": {
    "config": {
      "type": "object",
      "properties": {
        "max_retries": {
          "type": "integer",
          "description": "Maximum retry attempts"
        },
        "timeout": {
          "type": "string",
          "description": "Timeout duration"
        }
      },
      "required": [
        "max_retries"
      ],
      "additionalProperties": false,
      "description": "Configuration settings"
    },
    "id": {
      "type": "string",
      "description": "Unique identifier"
    },
    "tags": {
      "type": "array",
      "items": {
        "type": "string"
      },
      "description": "List of tags",
      "maxItems": 5
    }
  },
  "required": [
    "id",
    "config"
  ],
  "additionalProperties": false
}`

	require.JSONEq(t, expectedJSON, string(jsonData))
}
