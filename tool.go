package agent

import (
	"context"
	"encoding/json"
)

type ToolFunction func(ctx context.Context, name string, input json.RawMessage) (string, error)

type Tool interface {
	Definition() *ToolDefinition
	Invoke(ctx context.Context, input json.RawMessage) (string, error)
}

type SchemaProperty struct {
	Type        string                    `json:"type"`
	Description string                    `json:"description"`
	Enum        []string                  `json:"enum,omitempty"`
	Items       *SchemaProperty           `json:"items,omitempty"`
	Required    []string                  `json:"required,omitempty"`
	Properties  map[string]SchemaProperty `json:"properties,omitempty"`
}

type Schema struct {
	Type       string                    `json:"type"`
	Properties map[string]SchemaProperty `json:"properties"`
	Required   []string                  `json:"required,omitempty"`
}

type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  Schema `json:"parameters"`
}

func (t *ToolDefinition) ParametersCount() int {
	return len(t.Parameters.Properties)
}

type ToolInvocation struct {
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input"`
	Result string          `json:"result"`
	Error  error           `json:"error"`
}
