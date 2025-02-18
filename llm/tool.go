package llm

import (
	"context"
)

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  Schema `json:"parameters"`
}

func (t *Tool) ParametersCount() int {
	return len(t.Parameters.Properties)
}

type ToolFunc[T any] func(ctx context.Context, name string, input T) (string, error)

type ToolChoice struct {
	Type                   string `json:"type"`
	Name                   string `json:"name,omitempty"`
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use,omitempty"`
}

// type Tool interface {
// 	Definition() *ToolDefinition
// 	Invoke(ctx context.Context, input json.RawMessage) (string, error)
// }

// type ToolInvocation struct {
// 	Name   string          `json:"name"`
// 	Input  json.RawMessage `json:"input"`
// 	Result string          `json:"result"`
// 	Error  error           `json:"error"`
// }
