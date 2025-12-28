package dive

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

// mockTypedTool is a simple typed tool for testing
type mockTypedTool struct {
	name        string
	description string
	schema      *Schema
}

type mockInput struct {
	Name  string `json:"name,omitempty"`
	Value int    `json:"value,omitempty"`
}

func (m *mockTypedTool) Name() string {
	return m.name
}

func (m *mockTypedTool) Description() string {
	return m.description
}

func (m *mockTypedTool) Schema() *Schema {
	return m.schema
}

func (m *mockTypedTool) Annotations() *ToolAnnotations {
	return nil
}

func (m *mockTypedTool) Call(ctx context.Context, input mockInput) (*ToolResult, error) {
	return NewToolResultText("ok"), nil
}

func TestTypedToolAdapter_ConvertInput_NilInput(t *testing.T) {
	tool := &mockTypedTool{
		name:        "test",
		description: "test tool",
	}
	adapter := ToolAdapter(tool)

	// Call with nil input - should not error
	result, err := adapter.Call(context.Background(), nil)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestTypedToolAdapter_ConvertInput_EmptyBytes(t *testing.T) {
	tool := &mockTypedTool{
		name:        "test",
		description: "test tool",
	}
	adapter := ToolAdapter(tool)

	// Call with empty byte slice - should not error
	result, err := adapter.Call(context.Background(), []byte{})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestTypedToolAdapter_ConvertInput_EmptyRawMessage(t *testing.T) {
	tool := &mockTypedTool{
		name:        "test",
		description: "test tool",
	}
	adapter := ToolAdapter(tool)

	// Call with empty json.RawMessage - should not error
	result, err := adapter.Call(context.Background(), json.RawMessage{})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestTypedToolAdapter_ConvertInput_ValidJSON(t *testing.T) {
	tool := &mockTypedTool{
		name:        "test",
		description: "test tool",
	}
	adapter := ToolAdapter(tool)

	// Call with valid JSON - should work
	result, err := adapter.Call(context.Background(), json.RawMessage(`{"name":"test","value":42}`))
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}

func TestTypedToolAdapter_ConvertInput_EmptyObject(t *testing.T) {
	tool := &mockTypedTool{
		name:        "test",
		description: "test tool",
	}
	adapter := ToolAdapter(tool)

	// Call with empty object - should work
	result, err := adapter.Call(context.Background(), json.RawMessage(`{}`))
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsError)
}
