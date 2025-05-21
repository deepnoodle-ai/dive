package schema

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSchemaMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		input Schema
	}{
		{
			name: "simple schema",
			input: Schema{
				Type: "object",
				Properties: map[string]*Property{
					"name": {
						Type:        "string",
						Description: "The name of the object",
					},
					"age": {
						Type:        "integer",
						Description: "The age of the object",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			name: "nested properties",
			input: Schema{
				Type: "object",
				Properties: map[string]*Property{
					"user": {
						Type:        "object",
						Description: "User information",
						Properties: map[string]*Property{
							"id": {
								Type:        "string",
								Description: "User ID",
							},
							"settings": {
								Type:        "object",
								Description: "User settings",
								Properties: map[string]*Property{
									"theme": {
										Type:        "string",
										Description: "UI theme",
									},
								},
							},
						},
						Required: []string{"id"},
					},
				},
			},
		},
		{
			name: "array property",
			input: Schema{
				Type: "object",
				Properties: map[string]*Property{
					"tags": {
						Type:        "array",
						Description: "List of tags",
						Items: &Property{
							Type:        "string",
							Description: "Tag value",
						},
					},
				},
			},
		},
		{
			name: "enum property",
			input: Schema{
				Type: "object",
				Properties: map[string]*Property{
					"status": {
						Type:        "string",
						Description: "Status of the item",
						Enum:        []string{"pending", "active", "completed"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Failed to marshal schema: %v", err)
			}

			// Unmarshal back to Schema
			var result Schema
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("Failed to unmarshal schema: %v", err)
			}

			// Compare input and result
			if !reflect.DeepEqual(tt.input, result) {
				t.Errorf("Schema after marshal/unmarshal doesn't match original:\nOriginal: %+v\nResult:   %+v", tt.input, result)
			}
		})
	}
}

func TestPropertyMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name  string
		input Property
	}{
		{
			name: "simple property",
			input: Property{
				Type:        "string",
				Description: "A simple string property",
			},
		},
		{
			name: "object property with nested fields",
			input: Property{
				Type:        "object",
				Description: "A complex object",
				Properties: map[string]*Property{
					"field1": {
						Type:        "string",
						Description: "First field",
					},
					"field2": {
						Type:        "number",
						Description: "Second field",
					},
				},
				Required: []string{"field1"},
			},
		},
		{
			name: "array property",
			input: Property{
				Type:        "array",
				Description: "An array of objects",
				Items: &Property{
					Type:        "object",
					Description: "Array item",
					Properties: map[string]*Property{
						"id": {
							Type:        "string",
							Description: "Item ID",
						},
					},
				},
			},
		},
		{
			name: "property with enum",
			input: Property{
				Type:        "string",
				Description: "Property with enum values",
				Enum:        []string{"option1", "option2", "option3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Failed to marshal property: %v", err)
			}

			// Unmarshal back to Property
			var result Property
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("Failed to unmarshal property: %v", err)
			}

			// Compare input and result
			if !reflect.DeepEqual(tt.input, result) {
				t.Errorf("Property after marshal/unmarshal doesn't match original:\nOriginal: %+v\nResult:   %+v", tt.input, result)
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	// Create a schema with various property types
	original := Schema{
		Type: "object",
		Properties: map[string]*Property{
			"id": {
				Type:        "string",
				Description: "Unique identifier",
			},
			"details": {
				Type:        "object",
				Description: "Detailed information",
				Properties: map[string]*Property{
					"name": {
						Type:        "string",
						Description: "Full name",
					},
					"age": {
						Type:        "integer",
						Description: "Age in years",
					},
					"preferences": {
						Type:        "object",
						Description: "User preferences",
						Properties: map[string]*Property{
							"theme": {
								Type:        "string",
								Description: "UI theme",
								Enum:        []string{"light", "dark", "system"},
							},
						},
					},
				},
			},
			"tags": {
				Type:        "array",
				Description: "Associated tags",
				Items: &Property{
					Type:        "string",
					Description: "Tag value",
				},
			},
			"status": {
				Type:        "string",
				Description: "Current status",
				Enum:        []string{"active", "inactive", "pending"},
			},
		},
		Required: []string{"id", "details"},
	}

	// Convert to JSON string representation
	jsonData, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal schema: %v", err)
	}

	// Output JSON for debugging if needed
	// t.Logf("JSON:\n%s", string(jsonData))

	// Parse JSON back to Schema
	var parsed Schema
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal schema: %v", err)
	}

	// Ensure the round-trip preserves all data
	if !reflect.DeepEqual(original, parsed) {
		t.Errorf("Schema after JSON round-trip doesn't match original")
	}
}
