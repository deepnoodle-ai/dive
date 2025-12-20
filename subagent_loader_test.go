package dive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestMapSubagentLoader(t *testing.T) {
	ctx := context.Background()

	t.Run("loads from map", func(t *testing.T) {
		loader := &MapSubagentLoader{
			Subagents: map[string]*SubagentDefinition{
				"test-agent": {
					Description: "Test agent",
					Prompt:      "You are a test agent",
				},
			},
		}

		result, err := loader.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, "Test agent", result["test-agent"].Description)
	})

	t.Run("empty map returns empty result", func(t *testing.T) {
		loader := &MapSubagentLoader{
			Subagents: map[string]*SubagentDefinition{},
		}

		result, err := loader.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(result))
	})

	t.Run("nil map returns nil", func(t *testing.T) {
		loader := &MapSubagentLoader{}

		result, err := loader.Load(ctx)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}

func TestCompositeSubagentLoader(t *testing.T) {
	ctx := context.Background()

	t.Run("combines multiple loaders", func(t *testing.T) {
		loader1 := &MapSubagentLoader{
			Subagents: map[string]*SubagentDefinition{
				"agent1": {Description: "First agent"},
			},
		}
		loader2 := &MapSubagentLoader{
			Subagents: map[string]*SubagentDefinition{
				"agent2": {Description: "Second agent"},
			},
		}

		composite := &CompositeSubagentLoader{
			Loaders: []SubagentLoader{loader1, loader2},
		}

		result, err := composite.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, "First agent", result["agent1"].Description)
		assert.Equal(t, "Second agent", result["agent2"].Description)
	})

	t.Run("later loaders override earlier ones", func(t *testing.T) {
		loader1 := &MapSubagentLoader{
			Subagents: map[string]*SubagentDefinition{
				"shared": {Description: "Original"},
			},
		}
		loader2 := &MapSubagentLoader{
			Subagents: map[string]*SubagentDefinition{
				"shared": {Description: "Override"},
			},
		}

		composite := &CompositeSubagentLoader{
			Loaders: []SubagentLoader{loader1, loader2},
		}

		result, err := composite.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, "Override", result["shared"].Description)
	})

	t.Run("empty loaders returns empty result", func(t *testing.T) {
		composite := &CompositeSubagentLoader{
			Loaders: []SubagentLoader{},
		}

		result, err := composite.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(result))
	})
}

func TestFileSubagentLoader(t *testing.T) {
	ctx := context.Background()

	t.Run("loads from directory", func(t *testing.T) {
		// Create temp directory with test files
		tmpDir := t.TempDir()
		agentsDir := filepath.Join(tmpDir, ".dive", "agents")
		err := os.MkdirAll(agentsDir, 0755)
		assert.NoError(t, err)

		// Create a test agent file
		content := `---
description: Code reviewer agent
model: sonnet
tools:
  - Read
  - Grep
---

You are a code review specialist.
Review code for quality and security issues.`

		err = os.WriteFile(filepath.Join(agentsDir, "code-reviewer.md"), []byte(content), 0644)
		assert.NoError(t, err)

		loader := &FileSubagentLoader{
			Directories: []string{agentsDir},
		}

		result, err := loader.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(result))

		agent := result["code-reviewer"]
		assert.NotNil(t, agent)
		assert.Equal(t, "Code reviewer agent", agent.Description)
		assert.Equal(t, "sonnet", agent.Model)
		assert.Equal(t, []string{"Read", "Grep"}, agent.Tools)
		assert.Contains(t, agent.Prompt, "code review specialist")
	})

	t.Run("skips non-existent directories", func(t *testing.T) {
		loader := &FileSubagentLoader{
			Directories: []string{"/nonexistent/path"},
		}

		result, err := loader.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(result))
	})

	t.Run("loads from multiple directories", func(t *testing.T) {
		// Create two temp directories
		tmpDir1 := t.TempDir()
		tmpDir2 := t.TempDir()

		// Create agent in first directory
		content1 := `---
description: Agent one
---
First agent prompt`
		err := os.WriteFile(filepath.Join(tmpDir1, "agent1.md"), []byte(content1), 0644)
		assert.NoError(t, err)

		// Create agent in second directory
		content2 := `---
description: Agent two
---
Second agent prompt`
		err = os.WriteFile(filepath.Join(tmpDir2, "agent2.md"), []byte(content2), 0644)
		assert.NoError(t, err)

		loader := &FileSubagentLoader{
			Directories: []string{tmpDir1, tmpDir2},
		}

		result, err := loader.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, "Agent one", result["agent1"].Description)
		assert.Equal(t, "Agent two", result["agent2"].Description)
	})
}

func TestParseFrontmatter(t *testing.T) {
	t.Run("parses valid frontmatter", func(t *testing.T) {
		content := `---
description: Test
model: opus
---

Body content here`

		fm, body, err := parseFrontmatter(content)
		assert.NoError(t, err)
		assert.Contains(t, fm, "description: Test")
		assert.Contains(t, fm, "model: opus")
		assert.Contains(t, body, "Body content here")
	})

	t.Run("fails without opening delimiter", func(t *testing.T) {
		content := `description: Test
---

Body content`

		_, _, err := parseFrontmatter(content)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must start with ---")
	})

	t.Run("fails without closing delimiter", func(t *testing.T) {
		content := `---
description: Test

Body without closing`

		_, _, err := parseFrontmatter(content)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing closing ---")
	})
}
