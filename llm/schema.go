package llm

type Schema struct {
	Type       string                     `json:"type"`
	Properties map[string]*SchemaProperty `json:"properties"`
	Required   []string                   `json:"required,omitempty"`
}

type SchemaProperty struct {
	Type        string                     `json:"type"`
	Description string                     `json:"description"`
	Enum        []string                   `json:"enum,omitempty"`
	Items       *SchemaProperty            `json:"items,omitempty"`
	Required    []string                   `json:"required,omitempty"`
	Properties  map[string]*SchemaProperty `json:"properties,omitempty"`
}
