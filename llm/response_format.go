package llm

import "github.com/deepnoodle-ai/wonton/schema"

// ResponseFormatType specifies the expected format of the LLM's response.
type ResponseFormatType string

const (
	ResponseFormatTypeText       ResponseFormatType = "text"
	ResponseFormatTypeJSON       ResponseFormatType = "json_object"
	ResponseFormatTypeJSONSchema ResponseFormatType = "json_schema"
)

// ResponseFormat guides an LLM's response format.
type ResponseFormat struct {
	// Type indicates the format type ("text", "json_object", or "json_schema")
	Type ResponseFormatType `json:"type"`

	// Schema provides a JSON schema to guide the model's output
	Schema *schema.Schema `json:"schema,omitempty"`

	// Name provides a name for the output to guide the model
	Name string `json:"name,omitempty"`

	// Description provides additional context to guide the model
	Description string `json:"description,omitempty"`
}
