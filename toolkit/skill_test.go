package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive/skill"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func setupSkillLoader(t *testing.T) *skill.Loader {
	tmpDir := t.TempDir()

	// Create skills directory
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	// Create a skill with allowed-tools
	skillDir := filepath.Join(skillsDir, "code-reviewer")
	assert.NoError(t, os.MkdirAll(skillDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: code-reviewer
description: Review code for best practices and issues.
allowed-tools:
  - Read
  - Grep
  - Glob
---

# Code Reviewer

## Instructions
1. Read the target files using the Read tool
2. Search for patterns using Grep
3. Find related files using Glob
4. Provide detailed feedback on code quality`), 0644))

	// Create a skill without allowed-tools
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "helper.md"), []byte(`---
name: helper
description: A general helper skill.
---

# Helper

Just follow the user's instructions.`), 0644))

	loader := skill.NewLoader(skill.LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.LoadSkills())

	return loader
}

func TestSkillTool_Name(t *testing.T) {
	loader := setupSkillLoader(t)
	tool := NewSkillTool(SkillToolOptions{Loader: loader})
	assert.Equal(t, "Skill", tool.Name())
}

func TestSkillTool_Description(t *testing.T) {
	loader := setupSkillLoader(t)
	tool := NewSkillTool(SkillToolOptions{Loader: loader})
	desc := tool.Description()

	assert.Contains(t, desc, "Execute a skill")
	assert.Contains(t, desc, "Available skills:")
	assert.Contains(t, desc, "code-reviewer")
	assert.Contains(t, desc, "helper")
}

func TestSkillTool_Schema(t *testing.T) {
	loader := setupSkillLoader(t)
	tool := NewSkillTool(SkillToolOptions{Loader: loader})
	s := tool.Schema()

	assert.Equal(t, schema.Object, s.Type)
	assert.Contains(t, s.Required, "skill")
	assert.Contains(t, s.Properties, "skill")
	assert.Contains(t, s.Properties, "args")
}

func TestSkillTool_Annotations(t *testing.T) {
	loader := setupSkillLoader(t)
	tool := NewSkillTool(SkillToolOptions{Loader: loader})
	ann := tool.Annotations()

	assert.Equal(t, "Skill", ann.Title)
	assert.True(t, ann.ReadOnlyHint)
	assert.False(t, ann.DestructiveHint)
}

func TestSkillTool_Call(t *testing.T) {
	ctx := context.Background()

	t.Run("activate skill successfully", func(t *testing.T) {
		loader := setupSkillLoader(t)
		tool := NewSkillTool(SkillToolOptions{Loader: loader})

		result, err := tool.Call(ctx, &SkillToolInput{Skill: "code-reviewer"})
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		text := result.Content[0].Text
		assert.Contains(t, text, "Skill Activated: code-reviewer")
		assert.Contains(t, text, "Read the target files")
		assert.Contains(t, text, "Tool Restrictions:")
		assert.Contains(t, text, "Read, Grep, Glob")

		// Check active skill is set
		active := tool.GetActiveSkill()
		assert.NotNil(t, active)
		assert.Equal(t, "code-reviewer", active.Name)
	})

	t.Run("activate skill without allowed-tools", func(t *testing.T) {
		loader := setupSkillLoader(t)
		tool := NewSkillTool(SkillToolOptions{Loader: loader})

		result, err := tool.Call(ctx, &SkillToolInput{Skill: "helper"})
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		text := result.Content[0].Text
		assert.Contains(t, text, "Skill Activated: helper")
		// Should not contain tool restrictions
		assert.NotContains(t, text, "Tool Restrictions:")
	})

	t.Run("activate skill with args", func(t *testing.T) {
		loader := setupSkillLoader(t)
		tool := NewSkillTool(SkillToolOptions{Loader: loader})

		result, err := tool.Call(ctx, &SkillToolInput{
			Skill: "code-reviewer",
			Args:  "focus on security issues",
		})
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		text := result.Content[0].Text
		assert.Contains(t, text, "Arguments:")
		assert.Contains(t, text, "focus on security issues")
	})

	t.Run("skill not found", func(t *testing.T) {
		loader := setupSkillLoader(t)
		tool := NewSkillTool(SkillToolOptions{Loader: loader})

		result, err := tool.Call(ctx, &SkillToolInput{Skill: "nonexistent"})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "skill \"nonexistent\" not found")
		assert.Contains(t, result.Content[0].Text, "Available skills:")
	})

	t.Run("empty skill name", func(t *testing.T) {
		loader := setupSkillLoader(t)
		tool := NewSkillTool(SkillToolOptions{Loader: loader})

		result, err := tool.Call(ctx, &SkillToolInput{Skill: ""})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "skill name is required")
	})
}

func TestSkillTool_IsToolAllowed(t *testing.T) {
	ctx := context.Background()

	t.Run("no active skill allows all tools", func(t *testing.T) {
		loader := setupSkillLoader(t)
		tool := NewSkillTool(SkillToolOptions{Loader: loader})

		assert.True(t, tool.IsToolAllowed("AnyTool"))
		assert.True(t, tool.IsToolAllowed("Read"))
		assert.True(t, tool.IsToolAllowed("Write"))
	})

	t.Run("active skill with restrictions", func(t *testing.T) {
		loader := setupSkillLoader(t)
		tool := NewSkillTool(SkillToolOptions{Loader: loader})

		// Activate code-reviewer skill which restricts to Read, Grep, Glob
		_, err := tool.Call(ctx, &SkillToolInput{Skill: "code-reviewer"})
		assert.NoError(t, err)

		// These should be allowed
		assert.True(t, tool.IsToolAllowed("Read"))
		assert.True(t, tool.IsToolAllowed("Grep"))
		assert.True(t, tool.IsToolAllowed("Glob"))

		// These should not be allowed
		assert.False(t, tool.IsToolAllowed("Write"))
		assert.False(t, tool.IsToolAllowed("Bash"))
		assert.False(t, tool.IsToolAllowed("Edit"))
	})

	t.Run("active skill without restrictions allows all", func(t *testing.T) {
		loader := setupSkillLoader(t)
		tool := NewSkillTool(SkillToolOptions{Loader: loader})

		// Activate helper skill which has no restrictions
		_, err := tool.Call(ctx, &SkillToolInput{Skill: "helper"})
		assert.NoError(t, err)

		// All tools should be allowed
		assert.True(t, tool.IsToolAllowed("Read"))
		assert.True(t, tool.IsToolAllowed("Write"))
		assert.True(t, tool.IsToolAllowed("Bash"))
	})
}

func TestSkillTool_ClearActiveSkill(t *testing.T) {
	ctx := context.Background()
	loader := setupSkillLoader(t)
	tool := NewSkillTool(SkillToolOptions{Loader: loader})

	// Activate a skill
	_, err := tool.Call(ctx, &SkillToolInput{Skill: "code-reviewer"})
	assert.NoError(t, err)
	assert.NotNil(t, tool.GetActiveSkill())

	// Clear it
	tool.ClearActiveSkill()
	assert.Nil(t, tool.GetActiveSkill())

	// All tools should be allowed again
	assert.True(t, tool.IsToolAllowed("Write"))
}

func TestSkillTool_EmptyLoader(t *testing.T) {
	tmpDir := t.TempDir()

	loader := skill.NewLoader(skill.LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.LoadSkills())

	tool := NewSkillTool(SkillToolOptions{Loader: loader})

	// Description should indicate no skills available
	desc := tool.Description()
	assert.Contains(t, desc, "No skills are currently available")

	// Calling with any skill name should fail
	ctx := context.Background()
	result, err := tool.Call(ctx, &SkillToolInput{Skill: "anything"})
	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "No skills are currently available")
}

func TestSkillTool_SkillToolAlwaysAllowed(t *testing.T) {
	ctx := context.Background()
	loader := setupSkillLoader(t)
	tool := NewSkillTool(SkillToolOptions{Loader: loader})

	// Activate code-reviewer skill which restricts to Read, Grep, Glob
	_, err := tool.Call(ctx, &SkillToolInput{Skill: "code-reviewer"})
	assert.NoError(t, err)

	// The Skill tool itself should always be allowed
	assert.True(t, tool.IsToolAllowed("Skill"))

	// But Write should not be allowed
	assert.False(t, tool.IsToolAllowed("Write"))
}
