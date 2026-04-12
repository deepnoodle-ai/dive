package subagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestRegistry(t *testing.T) {
	t.Run("NewRegistry without general-purpose", func(t *testing.T) {
		r := NewRegistry(false)
		assert.Equal(t, 0, r.Len())
		_, ok := r.Get("general-purpose")
		assert.False(t, ok)
	})

	t.Run("NewRegistry with general-purpose", func(t *testing.T) {
		r := NewRegistry(true)
		assert.Equal(t, 1, r.Len())
		def, ok := r.Get("general-purpose")
		assert.True(t, ok)
		assert.Equal(t, GeneralPurpose.Description, def.Description)
	})

	t.Run("Register and Get", func(t *testing.T) {
		r := NewRegistry(false)
		def := &Definition{
			Description: "Test agent",
			Prompt:      "You are a test agent",
		}

		r.Register("test-agent", def)

		retrieved, ok := r.Get("test-agent")
		assert.True(t, ok)
		assert.Equal(t, "Test agent", retrieved.Description)
		assert.Equal(t, "You are a test agent", retrieved.Prompt)
	})

	t.Run("RegisterAll", func(t *testing.T) {
		r := NewRegistry(false)
		defs := map[string]*Definition{
			"agent-a": {Description: "Agent A"},
			"agent-b": {Description: "Agent B"},
		}

		r.RegisterAll(defs)

		assert.Equal(t, 2, r.Len())
		a, _ := r.Get("agent-a")
		assert.Equal(t, "Agent A", a.Description)
		b, _ := r.Get("agent-b")
		assert.Equal(t, "Agent B", b.Description)
	})

	t.Run("List returns sorted names", func(t *testing.T) {
		r := NewRegistry(false)
		r.Register("zebra", &Definition{Description: "Z"})
		r.Register("alpha", &Definition{Description: "A"})
		r.Register("mango", &Definition{Description: "M"})

		names := r.List()
		assert.Equal(t, []string{"alpha", "mango", "zebra"}, names)
	})

	t.Run("GenerateToolDescription", func(t *testing.T) {
		r := NewRegistry(false)
		r.Register("code-review", &Definition{Description: "Reviews code"})
		r.Register("doc-writer", &Definition{Description: "Writes documentation"})

		desc := r.GenerateToolDescription()
		assert.Contains(t, desc, "Available subagent types:")
		assert.Contains(t, desc, "code-review: Reviews code")
		assert.Contains(t, desc, "doc-writer: Writes documentation")
	})

	t.Run("GenerateToolDescription empty registry", func(t *testing.T) {
		r := NewRegistry(false)
		desc := r.GenerateToolDescription()
		assert.Equal(t, "", desc)
	})
}

// mockTool implements dive.Tool for testing
type mockTool struct {
	name string
}

func (m *mockTool) Name() string                       { return m.name }
func (m *mockTool) Description() string                { return "Mock tool" }
func (m *mockTool) Schema() *dive.Schema               { return nil }
func (m *mockTool) Annotations() *dive.ToolAnnotations { return nil }
func (m *mockTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, nil
}

func TestFilterTools(t *testing.T) {
	allTools := []dive.Tool{
		&mockTool{name: "Read"},
		&mockTool{name: "Write"},
		&mockTool{name: "Bash"},
		&mockTool{name: "Task"},
	}

	t.Run("nil tools allows all except Task", func(t *testing.T) {
		def := &Definition{Tools: nil}
		filtered := FilterTools(def, allTools)
		assert.Equal(t, 3, len(filtered))
		names := make([]string, len(filtered))
		for i, t := range filtered {
			names[i] = t.Name()
		}
		assert.Contains(t, names, "Read")
		assert.Contains(t, names, "Write")
		assert.Contains(t, names, "Bash")
	})

	t.Run("empty tools allows all except Task", func(t *testing.T) {
		def := &Definition{Tools: []string{}}
		filtered := FilterTools(def, allTools)
		assert.Equal(t, 3, len(filtered))
	})

	t.Run("specified tools filters to only those", func(t *testing.T) {
		def := &Definition{Tools: []string{"Read", "Bash"}}
		filtered := FilterTools(def, allTools)
		assert.Equal(t, 2, len(filtered))
		names := make([]string, len(filtered))
		for i, t := range filtered {
			names[i] = t.Name()
		}
		assert.Contains(t, names, "Read")
		assert.Contains(t, names, "Bash")
	})

	t.Run("Task is never allowed even if specified", func(t *testing.T) {
		def := &Definition{Tools: []string{"Read", "Task"}}
		filtered := FilterTools(def, allTools)
		assert.Equal(t, 1, len(filtered))
		assert.Equal(t, "Read", filtered[0].Name())
	})
}

func TestMapLoader(t *testing.T) {
	t.Run("Load returns definitions", func(t *testing.T) {
		loader := &MapLoader{
			Definitions: map[string]*Definition{
				"test": {Description: "Test"},
			},
		}

		defs, err := loader.Load(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 1, len(defs))
		assert.Equal(t, "Test", defs["test"].Description)
	})
}

func TestCompositeLoader(t *testing.T) {
	t.Run("Load combines all loaders", func(t *testing.T) {
		loader1 := &MapLoader{
			Definitions: map[string]*Definition{
				"agent-a": {Description: "A"},
			},
		}
		loader2 := &MapLoader{
			Definitions: map[string]*Definition{
				"agent-b": {Description: "B"},
			},
		}

		composite := &CompositeLoader{
			Loaders: []Loader{loader1, loader2},
		}

		defs, err := composite.Load(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 2, len(defs))
		assert.Equal(t, "A", defs["agent-a"].Description)
		assert.Equal(t, "B", defs["agent-b"].Description)
	})

	t.Run("later loaders override earlier", func(t *testing.T) {
		loader1 := &MapLoader{
			Definitions: map[string]*Definition{
				"same": {Description: "Original"},
			},
		}
		loader2 := &MapLoader{
			Definitions: map[string]*Definition{
				"same": {Description: "Override"},
			},
		}

		composite := &CompositeLoader{
			Loaders: []Loader{loader1, loader2},
		}

		defs, err := composite.Load(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "Override", defs["same"].Description)
	})
}

func TestFileLoader(t *testing.T) {
	t.Run("Load from directory", func(t *testing.T) {
		// Create temp directory with test files
		dir := t.TempDir()

		// Create a valid agent file
		content := `---
description: Test code reviewer
model: sonnet
tools:
  - Read
  - Grep
---
You are a code reviewer. Analyze the code and provide feedback.`

		err := os.WriteFile(filepath.Join(dir, "code-reviewer.md"), []byte(content), 0644)
		assert.NoError(t, err)

		loader := &FileLoader{
			Directories: []string{dir},
		}

		defs, err := loader.Load(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 1, len(defs))

		def := defs["code-reviewer"]
		assert.Equal(t, "Test code reviewer", def.Description)
		assert.Equal(t, "sonnet", def.Model)
		assert.Equal(t, []string{"Read", "Grep"}, def.Tools)
		assert.Contains(t, def.Prompt, "code reviewer")
	})

	t.Run("Load skips non-markdown files", func(t *testing.T) {
		dir := t.TempDir()

		// Create a non-markdown file
		err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not an agent"), 0644)
		assert.NoError(t, err)

		loader := &FileLoader{
			Directories: []string{dir},
		}

		defs, err := loader.Load(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 0, len(defs))
	})

	t.Run("Load handles missing directory", func(t *testing.T) {
		loader := &FileLoader{
			Directories: []string{"/nonexistent/path"},
		}

		defs, err := loader.Load(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, 0, len(defs))
	})
}

func TestLoadFromDirectory(t *testing.T) {
	t.Run("returns error for non-directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file.txt")
		os.WriteFile(file, []byte("content"), 0644)

		_, err := LoadFromDirectory(file)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})

	t.Run("returns error for invalid frontmatter", func(t *testing.T) {
		dir := t.TempDir()
		content := `No frontmatter here`
		os.WriteFile(filepath.Join(dir, "invalid.md"), []byte(content), 0644)

		_, err := LoadFromDirectory(dir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "frontmatter")
	})

	t.Run("returns error for missing description", func(t *testing.T) {
		dir := t.TempDir()
		content := `---
model: sonnet
---
Body without description`
		os.WriteFile(filepath.Join(dir, "nodesc.md"), []byte(content), 0644)

		_, err := LoadFromDirectory(dir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "description is required")
	})
}

func TestLoadIntoRegistry(t *testing.T) {
	t.Run("loads definitions into registry", func(t *testing.T) {
		loader := &MapLoader{
			Definitions: map[string]*Definition{
				"loaded-agent": {Description: "Loaded"},
			},
		}

		registry := NewRegistry(false)
		err := LoadIntoRegistry(context.Background(), loader, registry)
		assert.NoError(t, err)

		def, ok := registry.Get("loaded-agent")
		assert.True(t, ok)
		assert.Equal(t, "Loaded", def.Description)
	})
}
