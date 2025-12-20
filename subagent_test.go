package dive

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestSubagentDefinition(t *testing.T) {
	t.Run("GeneralPurposeSubagent is defined", func(t *testing.T) {
		assert.NotNil(t, GeneralPurposeSubagent)
		assert.NotEmpty(t, GeneralPurposeSubagent.Description)
		assert.NotEmpty(t, GeneralPurposeSubagent.Prompt)
		assert.Nil(t, GeneralPurposeSubagent.Tools) // inherits all
		assert.Empty(t, GeneralPurposeSubagent.Model)
	})
}

func TestSubagentRegistry(t *testing.T) {
	t.Run("new registry with general-purpose", func(t *testing.T) {
		registry := NewSubagentRegistry(true)
		assert.NotNil(t, registry)
		assert.Equal(t, 1, registry.Len())

		def, ok := registry.Get("general-purpose")
		assert.True(t, ok)
		assert.Equal(t, GeneralPurposeSubagent.Description, def.Description)
	})

	t.Run("new registry without general-purpose", func(t *testing.T) {
		registry := NewSubagentRegistry(false)
		assert.NotNil(t, registry)
		assert.Equal(t, 0, registry.Len())
	})

	t.Run("register and get", func(t *testing.T) {
		registry := NewSubagentRegistry(false)
		def := &SubagentDefinition{
			Description: "Test subagent",
			Prompt:      "You are a test agent",
			Tools:       []string{"Read", "Grep"},
			Model:       "sonnet",
		}
		registry.Register("test-agent", def)

		got, ok := registry.Get("test-agent")
		assert.True(t, ok)
		assert.Equal(t, "Test subagent", got.Description)
		assert.Equal(t, "You are a test agent", got.Prompt)
		assert.Equal(t, []string{"Read", "Grep"}, got.Tools)
		assert.Equal(t, "sonnet", got.Model)
	})

	t.Run("register all", func(t *testing.T) {
		registry := NewSubagentRegistry(false)
		defs := map[string]*SubagentDefinition{
			"agent1": {Description: "First agent"},
			"agent2": {Description: "Second agent"},
		}
		registry.RegisterAll(defs)

		assert.Equal(t, 2, registry.Len())

		def1, ok := registry.Get("agent1")
		assert.True(t, ok)
		assert.Equal(t, "First agent", def1.Description)

		def2, ok := registry.Get("agent2")
		assert.True(t, ok)
		assert.Equal(t, "Second agent", def2.Description)
	})

	t.Run("list returns sorted names", func(t *testing.T) {
		registry := NewSubagentRegistry(false)
		registry.Register("zebra", &SubagentDefinition{Description: "Z"})
		registry.Register("alpha", &SubagentDefinition{Description: "A"})
		registry.Register("middle", &SubagentDefinition{Description: "M"})

		names := registry.List()
		assert.Equal(t, []string{"alpha", "middle", "zebra"}, names)
	})

	t.Run("get non-existent returns false", func(t *testing.T) {
		registry := NewSubagentRegistry(false)
		_, ok := registry.Get("nonexistent")
		assert.False(t, ok)
	})

	t.Run("generate tool description", func(t *testing.T) {
		registry := NewSubagentRegistry(false)
		registry.Register("code-reviewer", &SubagentDefinition{
			Description: "Reviews code for quality",
		})
		registry.Register("test-runner", &SubagentDefinition{
			Description: "Runs and analyzes tests",
		})

		desc := registry.GenerateToolDescription()
		assert.Contains(t, desc, "Available subagent types:")
		assert.Contains(t, desc, "code-reviewer: Reviews code for quality")
		assert.Contains(t, desc, "test-runner: Runs and analyzes tests")
	})

	t.Run("empty registry generates empty description", func(t *testing.T) {
		registry := NewSubagentRegistry(false)
		desc := registry.GenerateToolDescription()
		assert.Empty(t, desc)
	})
}

func TestFilterTools(t *testing.T) {
	// Create mock tools
	mockTools := []Tool{
		&subagentMockTool{name: "Read"},
		&subagentMockTool{name: "Grep"},
		&subagentMockTool{name: "Glob"},
		&subagentMockTool{name: "Edit"},
		&subagentMockTool{name: "Task"},
		&subagentMockTool{name: "Bash"},
	}

	t.Run("nil tools inherits all except Task", func(t *testing.T) {
		def := &SubagentDefinition{
			Tools: nil,
		}
		filtered := FilterTools(def, mockTools)

		assert.Equal(t, 5, len(filtered))
		for _, tool := range filtered {
			assert.NotEqual(t, "Task", tool.Name())
		}
	})

	t.Run("empty tools inherits all except Task", func(t *testing.T) {
		def := &SubagentDefinition{
			Tools: []string{},
		}
		filtered := FilterTools(def, mockTools)

		assert.Equal(t, 5, len(filtered))
		for _, tool := range filtered {
			assert.NotEqual(t, "Task", tool.Name())
		}
	})

	t.Run("specific tools filters correctly", func(t *testing.T) {
		def := &SubagentDefinition{
			Tools: []string{"Read", "Grep"},
		}
		filtered := FilterTools(def, mockTools)

		assert.Equal(t, 2, len(filtered))
		names := make([]string, len(filtered))
		for i, tool := range filtered {
			names[i] = tool.Name()
		}
		assert.Contains(t, names, "Read")
		assert.Contains(t, names, "Grep")
	})

	t.Run("Task tool is never included even if requested", func(t *testing.T) {
		def := &SubagentDefinition{
			Tools: []string{"Read", "Task", "Grep"},
		}
		filtered := FilterTools(def, mockTools)

		assert.Equal(t, 2, len(filtered))
		for _, tool := range filtered {
			assert.NotEqual(t, "Task", tool.Name())
		}
	})
}

// subagentMockTool implements the Tool interface for testing
type subagentMockTool struct {
	name string
}

func (m *subagentMockTool) Name() string        { return m.name }
func (m *subagentMockTool) Description() string { return "Mock tool: " + m.name }
func (m *subagentMockTool) Schema() *Schema     { return nil }
func (m *subagentMockTool) Annotations() *ToolAnnotations {
	return &ToolAnnotations{Title: m.name}
}
func (m *subagentMockTool) Call(ctx context.Context, input any) (*ToolResult, error) {
	return NewToolResultText("ok"), nil
}
