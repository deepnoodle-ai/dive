package subagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestDescribeTypes(t *testing.T) {
	t.Run("lists types in sorted order", func(t *testing.T) {
		desc := DescribeTypes(map[string]*Definition{
			"doc-writer":  {Description: "Writes documentation"},
			"code-review": {Description: "Reviews code"},
		})
		assert.Contains(t, desc, "Available subagent types:")
		assert.Contains(t, desc, "code-review: Reviews code")
		assert.Contains(t, desc, "doc-writer: Writes documentation")
		assert.True(t, strings.Index(desc, "code-review") < strings.Index(desc, "doc-writer"))
	})

	t.Run("empty catalog yields empty string", func(t *testing.T) {
		assert.Equal(t, "", DescribeTypes(nil))
		assert.Equal(t, "", DescribeTypes(map[string]*Definition{}))
	})
}

func TestBuiltinDefinitions(t *testing.T) {
	t.Run("GeneralPurpose prompt is embedded", func(t *testing.T) {
		assert.NotEqual(t, "", GeneralPurpose.Description)
		assert.NotEqual(t, "", GeneralPurpose.Prompt)
		// Distinctive content from prompts/subagent.md guards the embed mapping.
		assert.Contains(t, GeneralPurpose.Prompt, "general-purpose agent")
	})

	t.Run("Explore is read-only", func(t *testing.T) {
		assert.NotEqual(t, "", Explore.Description)
		assert.NotEqual(t, "", Explore.Prompt)
		assert.True(t, len(Explore.DisallowedTools) > 0)
		// Distinctive content from prompts/explore.md guards the embed mapping.
		assert.Contains(t, Explore.Prompt, "read-only code exploration agent")
	})

	t.Run("Plan is read-only", func(t *testing.T) {
		assert.NotEqual(t, "", Plan.Description)
		assert.NotEqual(t, "", Plan.Prompt)
		assert.True(t, len(Plan.DisallowedTools) > 0)
		// The required final section is distinctive to prompts/plan.md.
		assert.Contains(t, Plan.Prompt, "Critical Files for Implementation")
	})

	t.Run("clone and modify does not affect original", func(t *testing.T) {
		clone := *Explore

		// Scalar field: a shallow copy already isolates it.
		clone.Model = "haiku"
		assert.Equal(t, "", Explore.Model)
		assert.Equal(t, "haiku", clone.Model)

		// Slice fields alias after a shallow copy, so copy them before
		// mutating to keep the original independent.
		clone.DisallowedTools = append([]string{}, Explore.DisallowedTools...)
		clone.DisallowedTools = append(clone.DisallowedTools, "WebSearch")
		clone.Tools = append([]string{}, Explore.Tools...)
		clone.Tools = append(clone.Tools, "Glob")

		assert.Equal(t, []string{"Edit", "Write", "Bash"}, Explore.DisallowedTools)
		assert.Equal(t, 0, len(Explore.Tools))
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
		&mockTool{name: "Agent"},
	}

	t.Run("nil tools allows all except Agent", func(t *testing.T) {
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

	t.Run("empty tools allows all except Agent", func(t *testing.T) {
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

	t.Run("Agent is never allowed even if specified", func(t *testing.T) {
		def := &Definition{Tools: []string{"Read", "Agent"}}
		filtered := FilterTools(def, allTools)
		assert.Equal(t, 1, len(filtered))
		assert.Equal(t, "Read", filtered[0].Name())
	})

	t.Run("DisallowedTools removes tools from the inherited set", func(t *testing.T) {
		def := &Definition{DisallowedTools: []string{"Write", "Bash"}}
		filtered := FilterTools(def, allTools)
		names := make([]string, len(filtered))
		for i, t := range filtered {
			names[i] = t.Name()
		}
		assert.Equal(t, []string{"Read"}, names)
	})

	t.Run("DisallowedTools matches case-insensitively", func(t *testing.T) {
		def := &Definition{DisallowedTools: []string{"write", "BASH"}}
		filtered := FilterTools(def, allTools)
		names := make([]string, len(filtered))
		for i, t := range filtered {
			names[i] = t.Name()
		}
		assert.Equal(t, []string{"Read"}, names)
	})

	t.Run("DisallowedTools applies on top of the allowlist", func(t *testing.T) {
		def := &Definition{Tools: []string{"Read", "Write"}, DisallowedTools: []string{"Write"}}
		filtered := FilterTools(def, allTools)
		assert.Equal(t, 1, len(filtered))
		assert.Equal(t, "Read", filtered[0].Name())
	})

	t.Run("read-only builtins keep only read tools", func(t *testing.T) {
		// Explore and Plan rely on DisallowedTools to enforce read-only.
		for _, def := range []*Definition{Explore, Plan} {
			filtered := FilterTools(def, allTools)
			names := make([]string, len(filtered))
			for i, t := range filtered {
				names[i] = t.Name()
			}
			assert.Equal(t, []string{"Read"}, names)
		}
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
