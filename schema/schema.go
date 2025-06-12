package schema

import "encoding/json"

// SchemaType represents the type of a JSON schema.
type SchemaType string

const (
	Array   SchemaType = "array"
	Boolean SchemaType = "boolean"
	Integer SchemaType = "integer"
	Null    SchemaType = "null"
	Number  SchemaType = "number"
	Object  SchemaType = "object"
	String  SchemaType = "string"
)

// Schema describes the structure of a JSON object.
type Schema struct {
	Type                 SchemaType           `json:"type"`
	Description          string               `json:"description,omitempty"`
	Title                string               `json:"title,omitempty"`
	Properties           map[string]*Property `json:"properties"`
	Required             []string             `json:"required,omitempty"`
	AdditionalProperties *bool                `json:"additionalProperties,omitempty"`
}

// AsMap converts the schema to a map[string]any.
func (s *Schema) AsMap() map[string]any {
	var result map[string]any
	data, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// Property of a schema.
type Property struct {
	Type                 SchemaType           `json:"type,omitempty"`
	Description          string               `json:"description,omitempty"`
	Enum                 []string             `json:"enum,omitempty"`
	Items                *Property            `json:"items,omitempty"`
	Required             []string             `json:"required,omitempty"`
	Properties           map[string]*Property `json:"properties,omitempty"`
	AdditionalProperties *bool                `json:"additionalProperties,omitempty"`
	Nullable             *bool                `json:"nullable,omitempty"`
	Pattern              *string              `json:"pattern,omitempty"`
	Format               *string              `json:"format,omitempty"`
	MinItems             *int                 `json:"minItems,omitempty"`
	MaxItems             *int                 `json:"maxItems,omitempty"`
	MinLength            *int                 `json:"minLength,omitempty"`
	MaxLength            *int                 `json:"maxLength,omitempty"`
	Minimum              *float64             `json:"minimum,omitempty"`
	Maximum              *float64             `json:"maximum,omitempty"`
}
