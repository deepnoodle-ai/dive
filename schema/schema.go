package schema

import "encoding/json"

// Schema describes the structure of a JSON object.
type Schema struct {
	Type                 string               `json:"type"`
	Properties           map[string]*Property `json:"properties"`
	Required             []string             `json:"required,omitempty"`
	AdditionalProperties *bool                `json:"additionalProperties,omitempty"`
}

// AsMap converts the schema to a map[string]any.
func (s Schema) AsMap() map[string]any {
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
	Type        string               `json:"type"`
	Description string               `json:"description"`
	Enum        []string             `json:"enum,omitempty"`
	Items       *Property            `json:"items,omitempty"`
	Required    []string             `json:"required,omitempty"`
	Properties  map[string]*Property `json:"properties,omitempty"`
}
