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

func TestToolAnnotations_MarshalSequentialOnlyHint(t *testing.T) {
	// The key is omitted when false so existing serialized annotations are
	// unchanged by the new hint.
	data, err := json.Marshal(&ToolAnnotations{Title: "t"})
	assert.NoError(t, err)
	var m map[string]any
	assert.NoError(t, json.Unmarshal(data, &m))
	_, present := m["sequentialOnlyHint"]
	assert.False(t, present)

	data, err = json.Marshal(&ToolAnnotations{Title: "t", SequentialOnlyHint: true})
	assert.NoError(t, err)
	m = nil
	assert.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, true, m["sequentialOnlyHint"])
}

func TestToolAnnotations_UnmarshalInvalidBoolHint(t *testing.T) {
	// Non-boolean hint values must fail fast instead of being silently
	// ignored (which would leave the zero-value default in place).
	var ann ToolAnnotations
	err := json.Unmarshal([]byte(`{"sequentialOnlyHint":"true"}`), &ann)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sequentialOnlyHint")

	err = json.Unmarshal([]byte(`{"readOnlyHint":1}`), &ann)
	assert.Error(t, err)

	// Valid booleans still round-trip, with unknown keys going to Extra.
	ann = ToolAnnotations{}
	err = json.Unmarshal([]byte(`{"sequentialOnlyHint":true,"editHint":false,"custom":"x"}`), &ann)
	assert.NoError(t, err)
	assert.True(t, ann.SequentialOnlyHint)
	assert.False(t, ann.EditHint)
	assert.Equal(t, "x", ann.Extra["custom"])
}
