package skill

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func setupTestLoader(t *testing.T) *Loader {
	tmpDir := t.TempDir()

	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	// Agent-invocable skill
	skillDir := filepath.Join(skillsDir, "code-reviewer")
	assert.NoError(t, os.MkdirAll(skillDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: code-reviewer
description: Review code for best practices and issues.
allowed-tools:
  - Read
  - Grep
---

# Code Reviewer

## Instructions
1. Read the target files using the Read tool
2. Search for patterns using Grep
3. Provide detailed feedback`), 0644))

	// Agent-invocable skill
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "helper.md"), []byte(`---
name: helper
description: A general helper skill.
---

# Helper

Just follow the user's instructions.`), 0644))

	// Skill with variable placeholders
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "deploy.md"), []byte(`---
name: deploy
description: Deploy to an environment.
argument-hint: "<environment>"
---

Deploy to $1 environment.
Full args: $ARGUMENTS`), 0644))

	// Command (no description, user-invocable only)
	commandsDir := filepath.Join(tmpDir, ".dive", "commands")
	assert.NoError(t, os.MkdirAll(commandsDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(commandsDir, "commit.md"), []byte(`Create a good git commit.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.Load(context.Background()))
	return loader
}

func TestTool_Name(t *testing.T) {
	loader := setupTestLoader(t)
	tool := NewTool(loader)
	assert.Equal(t, "Skill", tool.Name())
}

func TestTool_Description(t *testing.T) {
	loader := setupTestLoader(t)
	tool := NewTool(loader)
	desc := tool.Description()

	// Description should be static — no skill listing
	assert.Contains(t, desc, "Execute a skill by name")
	// Should NOT contain any specific skill names (catalog is injected separately)
	assert.NotContains(t, desc, "code-reviewer")
	assert.NotContains(t, desc, "deploy")
}

func TestTool_Schema(t *testing.T) {
	loader := setupTestLoader(t)
	tool := NewTool(loader)
	s := tool.Schema()

	assert.Equal(t, schema.Object, s.Type)
	assert.Contains(t, s.Required, "skill")
	assert.Contains(t, s.Properties, "skill")
	assert.Contains(t, s.Properties, "args")
}

func TestTool_Annotations(t *testing.T) {
	loader := setupTestLoader(t)
	tool := NewTool(loader)
	ann := tool.Annotations()

	assert.Equal(t, "Skill", ann.Title)
	assert.True(t, ann.ReadOnlyHint)
	assert.False(t, ann.DestructiveHint)
	assert.True(t, ann.IdempotentHint)
}

func TestTool_Call(t *testing.T) {
	ctx := context.Background()

	t.Run("activate skill successfully", func(t *testing.T) {
		loader := setupTestLoader(t)
		tool := NewTool(loader)

		result, err := tool.Call(ctx, &ToolInput{Skill: "code-reviewer"})
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		// Tool result is a brief acknowledgment
		text := result.Content[0].Text
		assert.Contains(t, text, "Launching skill: code-reviewer")

		// Full instructions stored as pending for PostToolUse hook
		loader.mu.RLock()
		assert.Equal(t, 1, len(loader.pendingInstructions))
		pending := loader.pendingInstructions[0]
		loader.mu.RUnlock()
		assert.Contains(t, pending, "Read the target files")
		assert.Contains(t, pending, "Base directory for this skill:")
	})

	t.Run("activate skill with args expansion", func(t *testing.T) {
		loader := setupTestLoader(t)
		tool := NewTool(loader)

		result, err := tool.Call(ctx, &ToolInput{
			Skill: "deploy",
			Args:  "staging",
		})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "Launching skill: deploy")

		// Check expanded instructions in pending
		loader.mu.RLock()
		assert.Equal(t, 1, len(loader.pendingInstructions))
		pending := loader.pendingInstructions[0]
		loader.mu.RUnlock()
		assert.Contains(t, pending, "Deploy to staging environment")
		assert.Contains(t, pending, "Full args: staging")
		assert.Contains(t, pending, "**Arguments:** staging")
	})

	t.Run("skill not found", func(t *testing.T) {
		loader := setupTestLoader(t)
		tool := NewTool(loader)

		result, err := tool.Call(ctx, &ToolInput{Skill: "nonexistent"})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "skill \"nonexistent\" not found")
		assert.Contains(t, result.Content[0].Text, "Available skills:")
	})

	t.Run("empty skill name", func(t *testing.T) {
		loader := setupTestLoader(t)
		tool := NewTool(loader)

		result, err := tool.Call(ctx, &ToolInput{Skill: ""})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "skill name is required")
	})
}

func TestTool_EmptyLoader(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.Load(context.Background()))

	tool := NewTool(loader)

	// Description is static regardless of loaded skills
	desc := tool.Description()
	assert.Contains(t, desc, "Execute a skill by name")

	// Calling with any skill name should fail
	ctx := context.Background()
	result, err := tool.Call(ctx, &ToolInput{Skill: "anything"})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "No skills are currently available")
}

func TestTool_ShellExpansionBlockedForHTTPSkills(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"remote": {
				Name:         "remote",
				Description:  "A remote skill.",
				Instructions: "Result: !{echo SHOULD_NOT_EXECUTE}",
				SourceURI:    "https://example.com/skills/remote/SKILL.md",
				Config:       SkillConfig{Description: "A remote skill."},
			},
			"local": {
				Name:         "local",
				Description:  "A local skill.",
				Instructions: "Result: !{echo hello}",
				SourceURI:    "file:///path/to/skill.md",
				Config:       SkillConfig{Description: "A local skill."},
			},
		},
	}

	// Shell expansion enabled globally
	tool := NewTool(loader, WithToolShellExpansion(true))

	// Remote skill: shell expansion should NOT run
	_, err := tool.Call(context.Background(), &ToolInput{Skill: "remote"})
	assert.NoError(t, err)

	loader.mu.RLock()
	assert.Equal(t, 1, len(loader.pendingInstructions))
	remotePending := loader.pendingInstructions[0]
	loader.mu.RUnlock()
	// The !{...} placeholder should be left unexpanded
	assert.Contains(t, remotePending, "!{echo SHOULD_NOT_EXECUTE}")

	// Drain the queue
	loader.mu.Lock()
	loader.pendingInstructions = nil
	loader.mu.Unlock()

	// Local skill: shell expansion should run
	_, err = tool.Call(context.Background(), &ToolInput{Skill: "local"})
	assert.NoError(t, err)

	loader.mu.RLock()
	assert.Equal(t, 1, len(loader.pendingInstructions))
	localPending := loader.pendingInstructions[0]
	loader.mu.RUnlock()
	// The !{...} placeholder should have been expanded
	assert.NotContains(t, localPending, "!{echo hello}")
	assert.Contains(t, localPending, "hello")
}

func TestTool_ParallelSkillCalls(t *testing.T) {
	loader := &Loader{
		skills: map[string]*Skill{
			"skill-a": {
				Name:         "skill-a",
				Description:  "Skill A.",
				Instructions: "Instructions for A.",
				SourceURI:    "file:///a.md",
				Config:       SkillConfig{Description: "Skill A."},
			},
			"skill-b": {
				Name:         "skill-b",
				Description:  "Skill B.",
				Instructions: "Instructions for B.",
				SourceURI:    "file:///b.md",
				Config:       SkillConfig{Description: "Skill B."},
			},
		},
	}

	tool := NewTool(loader)
	ctx := context.Background()

	// Simulate two parallel Skill tool calls (both push before either hook pops)
	_, err := tool.Call(ctx, &ToolInput{Skill: "skill-a"})
	assert.NoError(t, err)
	_, err = tool.Call(ctx, &ToolInput{Skill: "skill-b"})
	assert.NoError(t, err)

	// Both should be queued
	loader.mu.RLock()
	assert.Equal(t, 2, len(loader.pendingInstructions))
	loader.mu.RUnlock()

	// First hook pop gets skill-a
	hook := skillContentHook(loader)
	hctx1 := &dive.HookContext{Tool: &mockTool{name: "Skill"}}
	assert.NoError(t, hook(ctx, hctx1))
	assert.Contains(t, hctx1.AdditionalContext, "Instructions for A.")

	// Second hook pop gets skill-b
	hctx2 := &dive.HookContext{Tool: &mockTool{name: "Skill"}}
	assert.NoError(t, hook(ctx, hctx2))
	assert.Contains(t, hctx2.AdditionalContext, "Instructions for B.")

	// Queue is now empty
	loader.mu.RLock()
	assert.Equal(t, 0, len(loader.pendingInstructions))
	loader.mu.RUnlock()
}

func TestIsLocal(t *testing.T) {
	tests := []struct {
		name      string
		sourceURI string
		want      bool
	}{
		{"empty URI is local", "", true},
		{"file URI is local", "file:///path/to/skill.md", true},
		{"http URI is not local", "http://example.com/skill.md", false},
		{"https URI is not local", "https://example.com/skill.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Skill{SourceURI: tt.sourceURI}
			assert.Equal(t, tt.want, s.IsLocal())
		})
	}
}

// mockTool implements dive.Tool for testing
type mockTool struct {
	name string
}

func (m *mockTool) Name() string                                            { return m.name }
func (m *mockTool) Description() string                                     { return "" }
func (m *mockTool) Schema() *schema.Schema                                  { return nil }
func (m *mockTool) Annotations() *dive.ToolAnnotations                      { return nil }
func (m *mockTool) Call(_ context.Context, _ any) (*dive.ToolResult, error) { return nil, nil }
