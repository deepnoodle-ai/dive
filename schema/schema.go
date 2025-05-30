package schema

// Schema describes the structure of a JSON object.
type Schema struct {
	Type                 string               `json:"type"`
	Properties           map[string]*Property `json:"properties"`
	Required             []string             `json:"required,omitempty"`
	AdditionalProperties *bool                `json:"additionalProperties,omitempty"`
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
