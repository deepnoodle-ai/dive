package llm

import (
	"context"
	"encoding/json"
)

type ToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  Schema `json:"parameters"`
}

func (t *ToolDefinition) ParametersCount() int {
	return len(t.Parameters.Properties)
}

type ToolFunc func(ctx context.Context, input json.RawMessage) (string, error)

type ToolChoice struct {
	Type                   string `json:"type"`
	Name                   string `json:"name,omitempty"`
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use,omitempty"`
}

type Tool interface {
	Definition() *ToolDefinition
	Call(ctx context.Context, input json.RawMessage) (string, error)
}

type StandardTool struct {
	def *ToolDefinition
	fn  ToolFunc
}

func NewTool(def *ToolDefinition, fn ToolFunc) Tool {
	return &StandardTool{def: def, fn: fn}
}

func (t *StandardTool) Definition() *ToolDefinition {
	return t.def
}

func (t *StandardTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	return t.fn(ctx, input)
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
