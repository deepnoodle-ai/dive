package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestParseContent(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		filePath    string
		wantName    string
		wantDesc    string
		wantInstr   string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid skill with all fields",
			content: `---
name: code-reviewer
description: Review code for best practices.
allowed-tools:
  - Read
  - Grep
  - Glob
---

# Code Reviewer

## Instructions
1. Read the files
2. Check for issues`,
			filePath:  "/path/to/skill/SKILL.md",
			wantName:  "code-reviewer",
			wantDesc:  "Review code for best practices.",
			wantInstr: "# Code Reviewer\n\n## Instructions\n1. Read the files\n2. Check for issues",
		},
		{
			name: "valid skill without allowed-tools",
			content: `---
name: helper
description: A helpful skill.
---

Some instructions here.`,
			filePath:  "/path/to/helper.md",
			wantName:  "helper",
			wantDesc:  "A helpful skill.",
			wantInstr: "Some instructions here.",
		},
		{
			name: "derive name from SKILL.md parent directory",
			content: `---
description: My skill description.
---

Instructions`,
			filePath:  "/path/to/my-skill/SKILL.md",
			wantName:  "my-skill",
			wantDesc:  "My skill description.",
			wantInstr: "Instructions",
		},
		{
			name: "derive name from COMMAND.md parent directory",
			content: `---
description: My command description.
---

Instructions`,
			filePath:  "/path/to/my-command/COMMAND.md",
			wantName:  "my-command",
			wantDesc:  "My command description.",
			wantInstr: "Instructions",
		},
		{
			name: "derive name from filename",
			content: `---
description: Another skill.
---

More instructions`,
			filePath:  "/path/to/skills/another-skill.md",
			wantName:  "another-skill",
			wantDesc:  "Another skill.",
			wantInstr: "More instructions",
		},
		{
			name:      "no frontmatter - treated as command",
			content:   "# Simple Command\n\nJust do the thing.",
			filePath:  "/path/to/simple.md",
			wantName:  "simple",
			wantInstr: "# Simple Command\n\nJust do the thing.",
		},
		{
			name: "missing closing delimiter",
			content: `---
name: incomplete
description: Missing closing`,
			filePath:    "/path/to/incomplete.md",
			wantErr:     true,
			errContains: "missing closing frontmatter delimiter",
		},
		{
			name: "invalid YAML",
			content: `---
name: [invalid yaml
description: bad
---

Instructions`,
			filePath:    "/path/to/invalid.md",
			wantErr:     true,
			errContains: "parsing skill frontmatter",
		},
		{
			name: "leading whitespace before frontmatter",
			content: `
---
name: whitespace-skill
description: Has leading whitespace.
---

Instructions`,
			filePath:  "/path/to/skill.md",
			wantName:  "whitespace-skill",
			wantDesc:  "Has leading whitespace.",
			wantInstr: "Instructions",
		},
		{
			name:      "empty content - no frontmatter",
			content:   "",
			filePath:  "/path/to/empty.md",
			wantName:  "empty",
			wantInstr: "",
		},
		{
			name: "only frontmatter no body",
			content: `---
name: minimal
description: Minimal skill.
---`,
			filePath:  "/path/to/minimal.md",
			wantName:  "minimal",
			wantDesc:  "Minimal skill.",
			wantInstr: "",
		},
		{
			name: "triple dashes in body content",
			content: `---
name: dashes-in-body
description: Has dashes in body.
---

Here is some code:
---
This is a separator
---
More content`,
			filePath:  "/path/to/dashes.md",
			wantName:  "dashes-in-body",
			wantDesc:  "Has dashes in body.",
			wantInstr: "Here is some code:\n---\nThis is a separator\n---\nMore content",
		},
		{
			name: "unicode content",
			content: `---
name: unicode-skill
description: Supports émojis and ünïcödé
---

Instructions with 中文 and 日本語`,
			filePath:  "/path/to/unicode.md",
			wantName:  "unicode-skill",
			wantDesc:  "Supports émojis and ünïcödé",
			wantInstr: "Instructions with 中文 and 日本語",
		},
		{
			name:      "windows line endings",
			content:   "---\r\nname: windows\r\ndescription: Windows line endings.\r\n---\r\n\r\nInstructions with CRLF.",
			filePath:  "/path/to/windows.md",
			wantName:  "windows",
			wantDesc:  "Windows line endings.",
			wantInstr: "Instructions with CRLF.",
		},
		{
			name:     "tabs in frontmatter",
			content:  "---\nname: tabbed\ndescription: Uses tabs.\nallowed-tools:\n\t- Read\n---\n\nInstructions.",
			filePath: "/path/to/tabbed.md",
			wantErr:  true, // YAML doesn't allow tabs for indentation
		},
		{
			name: "argument placeholders in content",
			content: `---
description: Fix an issue.
argument-hint: "[issue-number] [priority]"
---

Fix issue #$1 with priority $2.
Full args: $ARGUMENTS`,
			filePath:  "/path/to/fix-issue.md",
			wantName:  "fix-issue",
			wantDesc:  "Fix an issue.",
			wantInstr: "Fix issue #$1 with priority $2.\nFull args: $ARGUMENTS",
		},
		{
			name: "explicit name overrides filename",
			content: `---
name: explicit-name
description: Has explicit name.
---

Instructions`,
			filePath:  "/path/to/different-filename.md",
			wantName:  "explicit-name",
			wantDesc:  "Has explicit name.",
			wantInstr: "Instructions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := ParseContent([]byte(tt.content), tt.filePath)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, s)
			assert.Equal(t, tt.wantName, s.Name)
			assert.Equal(t, tt.wantDesc, s.Description)
			assert.Equal(t, tt.wantInstr, s.Instructions)
			assert.Equal(t, tt.filePath, s.FilePath)
		})
	}
}

func TestParseContent_AllFrontmatterFields(t *testing.T) {
	content := `---
name: deploy
description: Deploy to an environment.
allowed-tools:
  - Bash
  - Read
model: claude-sonnet-4-5-20250929
argument-hint: "<environment>"
triggers:
  - keyword: deploy
  - pattern: "deploy .+"
user-invocable: false
---

Deploy instructions.`

	s, err := ParseContent([]byte(content), "/path/to/deploy.md")
	assert.NoError(t, err)
	assert.Equal(t, "deploy", s.Name)
	assert.Equal(t, "Deploy to an environment.", s.Description)
	assert.Equal(t, []string{"Bash", "Read"}, s.Config.AllowedTools)
	assert.Equal(t, "claude-sonnet-4-5-20250929", s.Config.Model)
	assert.Equal(t, "<environment>", s.Config.ArgumentHint)
	assert.Equal(t, 2, len(s.Config.Triggers))
	assert.Equal(t, "deploy", s.Config.Triggers[0].Keyword)
	assert.Equal(t, "deploy .+", s.Config.Triggers[1].Pattern)
	assert.NotNil(t, s.Config.UserInvocable)
	assert.False(t, *s.Config.UserInvocable)
}

func TestParseContent_AgentskillsFields(t *testing.T) {
	content := `---
name: my-skill
description: A skill with ecosystem metadata.
license: MIT
compatibility:
  - claude
  - gpt-4o
metadata:
  author: Acme Corp
  version: 1.2.0
---

Instructions.`

	s, err := ParseContent([]byte(content), "/path/to/my-skill.md")
	assert.NoError(t, err)
	assert.Equal(t, "MIT", s.Config.License)
	assert.Equal(t, []string{"claude", "gpt-4o"}, s.Config.Compatibility)
	assert.Equal(t, map[string]string{"author": "Acme Corp", "version": "1.2.0"}, s.Config.Metadata)
}

func TestParseContent_AllowedToolsInlineArray(t *testing.T) {
	content := `---
name: inline-tools
description: Uses inline array.
allowed-tools: [Read, Write, Grep]
---

Instructions.`

	s, err := ParseContent([]byte(content), "/path/to/inline.md")
	assert.NoError(t, err)
	assert.Equal(t, []string{"Read", "Write", "Grep"}, s.Config.AllowedTools)
}

func TestParseFile(t *testing.T) {
	tmpDir := t.TempDir()
	skillPath := filepath.Join(tmpDir, "test-skill.md")
	content := `---
name: file-test
description: Testing file parsing.
---

# File Test

These are the instructions.`
	assert.NoError(t, os.WriteFile(skillPath, []byte(content), 0644))

	s, err := ParseFile(skillPath)
	assert.NoError(t, err)
	assert.NotNil(t, s)
	assert.Equal(t, "file-test", s.Name)
	assert.Equal(t, "Testing file parsing.", s.Description)
	assert.Equal(t, skillPath, s.FilePath)
}

func TestParseFile_NonExistent(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/skill.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading skill file")
}

func TestDeriveName(t *testing.T) {
	tests := []struct {
		filePath string
		wantName string
	}{
		{"/path/to/my-skill/SKILL.md", "my-skill"},
		{"/path/to/my-skill/skill.md", "my-skill"},
		{"/path/to/my-command/COMMAND.md", "my-command"},
		{"/path/to/my-command/command.md", "my-command"},
		{"/path/to/skills/helper.md", "helper"},
		{"/path/to/skills/my-tool.md", "my-tool"},
		{"SKILL.md", "."},
		{"test.md", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := deriveName(tt.filePath)
			assert.Equal(t, tt.wantName, got)
		})
	}
}

func TestIsCommand(t *testing.T) {
	t.Run("no description is a command", func(t *testing.T) {
		s := &Skill{Name: "test"}
		assert.True(t, s.IsCommand())
	})

	t.Run("with description is a skill", func(t *testing.T) {
		s := &Skill{Name: "test", Description: "A skill."}
		assert.False(t, s.IsCommand())
	})

	t.Run("explicit user-invocable true", func(t *testing.T) {
		v := true
		s := &Skill{Name: "test", Description: "A skill.", Config: SkillConfig{UserInvocable: &v}}
		assert.True(t, s.IsCommand())
	})

	t.Run("explicit user-invocable false", func(t *testing.T) {
		v := false
		s := &Skill{Name: "test", Config: SkillConfig{UserInvocable: &v}}
		assert.False(t, s.IsCommand())
	})
}
