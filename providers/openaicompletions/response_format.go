package openaicompletions

import (
	"fmt"

	"github.com/deepnoodle-ai/dive/llm"
)

func chatResponseFormat(format *llm.ResponseFormat) (*ChatResponseFormat, error) {
	switch format.Type {
	case llm.ResponseFormatTypeText:
		return nil, nil // Chat Completions uses text by default.
	case llm.ResponseFormatTypeJSON:
		return &ChatResponseFormat{Type: llm.ResponseFormatTypeJSON}, nil
	case llm.ResponseFormatTypeJSONSchema:
		if format.Schema == nil {
			return nil, fmt.Errorf("schema is required for json_schema response format")
		}
		if format.Name == "" {
			return nil, fmt.Errorf("name is required for json_schema response format")
		}
		schema := format.Schema.AsMap()
		schema["additionalProperties"] = false
		return &ChatResponseFormat{
			Type: llm.ResponseFormatTypeJSONSchema,
			JSONSchema: &ChatJSONSchema{
				Name:        format.Name,
				Description: format.Description,
				Strict:      true,
				Schema:      schema,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported response format type: %s", format.Type)
	}
}
