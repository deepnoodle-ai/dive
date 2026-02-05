package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestParseSkillContent(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		filePath    string
		wantName    string
		wantDesc    string
		wantTools   []string
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
			wantTools: []string{"Read", "Grep", "Glob"},
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
			wantTools: nil,
			wantInstr: "Some instructions here.",
		},
		{
			name: "derive name from directory",
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
			name:        "missing frontmatter",
			content:     "# Just markdown\n\nNo frontmatter here.",
			filePath:    "/path/to/bad.md",
			wantErr:     true,
			errContains: "must start with YAML frontmatter",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill, err := ParseSkillContent([]byte(tt.content), tt.filePath)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, skill)
			assert.Equal(t, tt.wantName, skill.Name)
			assert.Equal(t, tt.wantDesc, skill.Description)
			if tt.wantTools == nil {
				assert.Nil(t, skill.AllowedTools)
			} else {
				assert.Equal(t, len(tt.wantTools), len(skill.AllowedTools))
				for i, tool := range tt.wantTools {
					assert.Equal(t, tool, skill.AllowedTools[i])
				}
			}
			assert.Equal(t, tt.wantInstr, skill.Instructions)
			assert.Equal(t, tt.filePath, skill.FilePath)
		})
	}
}

func TestSkill_IsToolAllowed(t *testing.T) {
	tests := []struct {
		name         string
		allowedTools []string
		toolName     string
		want         bool
	}{
		{
			name:         "no restrictions - all allowed",
			allowedTools: nil,
			toolName:     "AnyTool",
			want:         true,
		},
		{
			name:         "empty restrictions - all allowed",
			allowedTools: []string{},
			toolName:     "AnyTool",
			want:         true,
		},
		{
			name:         "tool in allowed list",
			allowedTools: []string{"Read", "Grep", "Glob"},
			toolName:     "Read",
			want:         true,
		},
		{
			name:         "tool not in allowed list",
			allowedTools: []string{"Read", "Grep", "Glob"},
			toolName:     "Write",
			want:         false,
		},
		{
			name:         "case insensitive match",
			allowedTools: []string{"Read", "Grep"},
			toolName:     "read",
			want:         true,
		},
		{
			name:         "case insensitive match - uppercase",
			allowedTools: []string{"read", "grep"},
			toolName:     "READ",
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := &Skill{AllowedTools: tt.allowedTools}
			assert.Equal(t, tt.want, skill.IsToolAllowed(tt.toolName))
		})
	}
}

func TestLoader_LoadSkills(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create project .dive/skills directory
	projectDiveSkills := filepath.Join(tmpDir, "project", ".dive", "skills")
	assert.NoError(t, os.MkdirAll(projectDiveSkills, 0755))

	// Create project .claude/skills directory
	projectClaudeSkills := filepath.Join(tmpDir, "project", ".claude", "skills")
	assert.NoError(t, os.MkdirAll(projectClaudeSkills, 0755))

	// Create home .dive/skills directory
	homeDiveSkills := filepath.Join(tmpDir, "home", ".dive", "skills")
	assert.NoError(t, os.MkdirAll(homeDiveSkills, 0755))

	// Create skill in directory format
	skillDir := filepath.Join(projectDiveSkills, "code-reviewer")
	assert.NoError(t, os.MkdirAll(skillDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: code-reviewer
description: Review code.
allowed-tools:
  - Read
  - Grep
---

Instructions for code review.`), 0644))

	// Create standalone skill file
	assert.NoError(t, os.WriteFile(filepath.Join(projectClaudeSkills, "helper.md"), []byte(`---
name: helper
description: A helper skill.
---

Helper instructions.`), 0644))

	// Create skill in home directory (should be lower priority)
	assert.NoError(t, os.WriteFile(filepath.Join(homeDiveSkills, "helper.md"), []byte(`---
name: helper
description: Home helper skill - should be ignored.
---

Home helper instructions.`), 0644))

	// Create another home skill that should be loaded
	assert.NoError(t, os.WriteFile(filepath.Join(homeDiveSkills, "personal.md"), []byte(`---
name: personal
description: Personal skill.
---

Personal instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: filepath.Join(tmpDir, "project"),
		HomeDir:    filepath.Join(tmpDir, "home"),
	})

	err := loader.LoadSkills()
	assert.NoError(t, err)

	// Check loaded skills
	assert.Equal(t, 3, loader.SkillCount())

	// Check code-reviewer skill
	skill, ok := loader.GetSkill("code-reviewer")
	assert.True(t, ok)
	assert.Equal(t, "code-reviewer", skill.Name)
	assert.Equal(t, "Review code.", skill.Description)
	assert.Equal(t, 2, len(skill.AllowedTools))
	assert.Equal(t, "Read", skill.AllowedTools[0])
	assert.Equal(t, "Grep", skill.AllowedTools[1])

	// Check helper skill (project one should win)
	skill, ok = loader.GetSkill("helper")
	assert.True(t, ok)
	assert.Equal(t, "A helper skill.", skill.Description)

	// Check personal skill
	skill, ok = loader.GetSkill("personal")
	assert.True(t, ok)
	assert.Equal(t, "Personal skill.", skill.Description)

	// Check non-existent skill
	_, ok = loader.GetSkill("non-existent")
	assert.False(t, ok)

	// Check ListSkills returns sorted
	skills := loader.ListSkills()
	assert.Equal(t, 3, len(skills))
	assert.Equal(t, "code-reviewer", skills[0].Name)
	assert.Equal(t, "helper", skills[1].Name)
	assert.Equal(t, "personal", skills[2].Name)

	// Check ListSkillNames
	names := loader.ListSkillNames()
	assert.Equal(t, 3, len(names))
	assert.Equal(t, "code-reviewer", names[0])
	assert.Equal(t, "helper", names[1])
	assert.Equal(t, "personal", names[2])
}

func TestLoader_DisablePaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create skills in both Dive and Claude paths
	diveSkills := filepath.Join(tmpDir, ".dive", "skills")
	claudeSkills := filepath.Join(tmpDir, ".claude", "skills")
	assert.NoError(t, os.MkdirAll(diveSkills, 0755))
	assert.NoError(t, os.MkdirAll(claudeSkills, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(diveSkills, "dive-skill.md"), []byte(`---
name: dive-skill
description: Dive skill.
---

Dive instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(claudeSkills, "claude-skill.md"), []byte(`---
name: claude-skill
description: Claude skill.
---

Claude instructions.`), 0644))

	t.Run("disable Claude paths", func(t *testing.T) {
		loader := NewLoader(LoaderOptions{
			ProjectDir:         tmpDir,
			HomeDir:            "/nonexistent",
			DisableClaudePaths: true,
		})
		assert.NoError(t, loader.LoadSkills())

		_, ok := loader.GetSkill("dive-skill")
		assert.True(t, ok)

		_, ok = loader.GetSkill("claude-skill")
		assert.False(t, ok)
	})

	t.Run("disable Dive paths", func(t *testing.T) {
		loader := NewLoader(LoaderOptions{
			ProjectDir:       tmpDir,
			HomeDir:          "/nonexistent",
			DisableDivePaths: true,
		})
		assert.NoError(t, loader.LoadSkills())

		_, ok := loader.GetSkill("dive-skill")
		assert.False(t, ok)

		_, ok = loader.GetSkill("claude-skill")
		assert.True(t, ok)
	})
}

func TestLoader_AdditionalPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create custom skills path
	customSkills := filepath.Join(tmpDir, "custom-skills")
	assert.NoError(t, os.MkdirAll(customSkills, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(customSkills, "custom.md"), []byte(`---
name: custom
description: Custom skill.
---

Custom instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir:         "/nonexistent",
		HomeDir:            "/nonexistent",
		AdditionalPaths:    []string{customSkills},
		DisableClaudePaths: true,
		DisableDivePaths:   true,
	})
	assert.NoError(t, loader.LoadSkills())

	skill, ok := loader.GetSkill("custom")
	assert.True(t, ok)
	assert.Equal(t, "Custom skill.", skill.Description)
}

func TestLoader_MissingDirectories(t *testing.T) {
	loader := NewLoader(LoaderOptions{
		ProjectDir: "/nonexistent/project",
		HomeDir:    "/nonexistent/home",
	})

	// Should not error on missing directories
	err := loader.LoadSkills()
	assert.NoError(t, err)
	assert.Equal(t, 0, loader.SkillCount())
}

// Additional comprehensive parser tests

func TestParseSkillContent_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		filePath    string
		wantName    string
		wantDesc    string
		wantTools   []string
		wantInstr   string
		wantErr     bool
		errContains string
	}{
		{
			name:        "empty content",
			content:     "",
			filePath:    "/path/to/empty.md",
			wantErr:     true,
			errContains: "must start with YAML frontmatter",
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
			name: "empty body after frontmatter",
			content: `---
name: empty-body
description: Has empty body.
---

`,
			filePath:  "/path/to/empty-body.md",
			wantName:  "empty-body",
			wantDesc:  "Has empty body.",
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
			name: "special characters in name and description",
			content: `---
name: my-skill-v2
description: "This skill does: many things! Including 'quotes' and \"double quotes\"."
---

Instructions.`,
			filePath:  "/path/to/special.md",
			wantName:  "my-skill-v2",
			wantDesc:  "This skill does: many things! Including 'quotes' and \"double quotes\".",
			wantInstr: "Instructions.",
		},
		{
			name: "unicode content",
			content: `---
name: unicode-skill
description: Supports émojis 🎉 and ünïcödé
---

Instructions with 中文 and 日本語 and emoji 🚀`,
			filePath:  "/path/to/unicode.md",
			wantName:  "unicode-skill",
			wantDesc:  "Supports émojis 🎉 and ünïcödé",
			wantInstr: "Instructions with 中文 and 日本語 and emoji 🚀",
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
			name: "allowed-tools inline array format",
			content: `---
name: inline-tools
description: Uses inline array.
allowed-tools: [Read, Write, Grep]
---

Instructions.`,
			filePath:  "/path/to/inline.md",
			wantName:  "inline-tools",
			wantDesc:  "Uses inline array.",
			wantTools: []string{"Read", "Write", "Grep"},
			wantInstr: "Instructions.",
		},
		{
			name: "empty allowed-tools array",
			content: `---
name: no-tools
description: Empty tools list.
allowed-tools: []
---

Instructions.`,
			filePath:  "/path/to/no-tools.md",
			wantName:  "no-tools",
			wantDesc:  "Empty tools list.",
			wantTools: []string{},
			wantInstr: "Instructions.",
		},
		{
			name: "single allowed-tool",
			content: `---
name: single-tool
description: Single tool.
allowed-tools:
  - Read
---

Instructions.`,
			filePath:  "/path/to/single.md",
			wantName:  "single-tool",
			wantDesc:  "Single tool.",
			wantTools: []string{"Read"},
			wantInstr: "Instructions.",
		},
		{
			name: "very long instructions",
			content: `---
name: long-skill
description: Has long instructions.
---

` + string(make([]byte, 10000)),
			filePath:  "/path/to/long.md",
			wantName:  "long-skill",
			wantDesc:  "Has long instructions.",
			wantInstr: string(make([]byte, 10000)),
		},
		{
			name: "multiline description using folded style",
			content: `---
name: multiline-desc
description: >-
  This is a multiline
  description that spans
  multiple lines.
---

Instructions.`,
			filePath:  "/path/to/multiline.md",
			wantName:  "multiline-desc",
			wantDesc:  "This is a multiline description that spans multiple lines.",
			wantInstr: "Instructions.",
		},
		{
			name: "name with numbers",
			content: `---
name: skill-v2-beta-3
description: Versioned skill.
---

Instructions.`,
			filePath:  "/path/to/versioned.md",
			wantName:  "skill-v2-beta-3",
			wantDesc:  "Versioned skill.",
			wantInstr: "Instructions.",
		},
		{
			name: "only description no name - derive from SKILL.md parent",
			content: `---
description: No name field.
---

Instructions.`,
			filePath:  "/path/to/my-derived-skill/SKILL.md",
			wantName:  "my-derived-skill",
			wantDesc:  "No name field.",
			wantInstr: "Instructions.",
		},
		{
			name: "only description no name - derive from filename",
			content: `---
description: No name field.
---

Instructions.`,
			filePath:  "/path/to/skills/derived-from-file.md",
			wantName:  "derived-from-file",
			wantDesc:  "No name field.",
			wantInstr: "Instructions.",
		},
		{
			name: "extra unknown fields in frontmatter",
			content: `---
name: extra-fields
description: Has extra fields.
unknown-field: some value
another-field: 123
---

Instructions.`,
			filePath:  "/path/to/extra.md",
			wantName:  "extra-fields",
			wantDesc:  "Has extra fields.",
			wantInstr: "Instructions.",
		},
		{
			name: "frontmatter with comments",
			content: `---
name: commented
description: Has YAML comments.
# This is a comment line
allowed-tools:
  - Read
  - Write
---

Instructions.`,
			filePath:  "/path/to/commented.md",
			wantName:  "commented",
			wantDesc:  "Has YAML comments.",
			wantTools: []string{"Read", "Write"},
			wantInstr: "Instructions.",
		},
		{
			name:     "tabs in frontmatter",
			content:  "---\nname: tabbed\ndescription: Uses tabs.\nallowed-tools:\n\t- Read\n\t- Write\n---\n\nInstructions.",
			filePath: "/path/to/tabbed.md",
			wantErr:  true, // YAML doesn't allow tabs for indentation
		},
		{
			name:        "just dashes",
			content:     "---",
			filePath:    "/path/to/just-dashes.md",
			wantErr:     true,
			errContains: "missing closing frontmatter delimiter",
		},
		{
			name: "code block in instructions",
			content: `---
name: code-skill
description: Has code blocks.
---

Here's some code:

` + "```go" + `
func main() {
    fmt.Println("Hello")
}
` + "```" + `

More text.`,
			filePath:  "/path/to/code.md",
			wantName:  "code-skill",
			wantDesc:  "Has code blocks.",
			wantInstr: "Here's some code:\n\n```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```\n\nMore text.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill, err := ParseSkillContent([]byte(tt.content), tt.filePath)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, skill)
			assert.Equal(t, tt.wantName, skill.Name)
			assert.Equal(t, tt.wantDesc, skill.Description)
			if tt.wantTools == nil {
				assert.Nil(t, skill.AllowedTools)
			} else {
				assert.Equal(t, len(tt.wantTools), len(skill.AllowedTools))
				for i, tool := range tt.wantTools {
					assert.Equal(t, tool, skill.AllowedTools[i])
				}
			}
			assert.Equal(t, tt.wantInstr, skill.Instructions)
			assert.Equal(t, tt.filePath, skill.FilePath)
		})
	}
}

func TestParseSkillFile_ActualFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a skill file
	skillPath := filepath.Join(tmpDir, "test-skill.md")
	content := `---
name: file-test
description: Testing file parsing.
allowed-tools:
  - Read
  - Write
---

# File Test

These are the instructions.`
	assert.NoError(t, os.WriteFile(skillPath, []byte(content), 0644))

	// Parse the file
	skill, err := ParseSkillFile(skillPath)
	assert.NoError(t, err)
	assert.NotNil(t, skill)
	assert.Equal(t, "file-test", skill.Name)
	assert.Equal(t, "Testing file parsing.", skill.Description)
	assert.Equal(t, 2, len(skill.AllowedTools))
	assert.Equal(t, skillPath, skill.FilePath)
}

func TestParseSkillFile_NonExistent(t *testing.T) {
	_, err := ParseSkillFile("/nonexistent/path/skill.md")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading skill file")
}

func TestDeriveSkillName(t *testing.T) {
	tests := []struct {
		filePath string
		wantName string
	}{
		{"/path/to/my-skill/SKILL.md", "my-skill"},
		{"/path/to/my-skill/skill.md", "my-skill"}, // case insensitive
		{"/path/to/skills/helper.md", "helper"},
		{"/path/to/skills/my-tool.md", "my-tool"},
		{"SKILL.md", "."}, // edge case: relative path in current dir
		{"test.md", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := deriveSkillName(tt.filePath)
			assert.Equal(t, tt.wantName, got)
		})
	}
}

// Additional comprehensive loader tests

func TestLoader_ReloadSkills(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	// Create initial skill
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "skill1.md"), []byte(`---
name: skill1
description: First skill.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})

	// First load
	assert.NoError(t, loader.LoadSkills())
	assert.Equal(t, 1, loader.SkillCount())

	// Add another skill
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "skill2.md"), []byte(`---
name: skill2
description: Second skill.
---
Instructions.`), 0644))

	// Reload - should pick up new skill
	assert.NoError(t, loader.LoadSkills())
	assert.Equal(t, 2, loader.SkillCount())

	// Remove a skill
	assert.NoError(t, os.Remove(filepath.Join(skillsDir, "skill1.md")))

	// Reload - should reflect removal
	assert.NoError(t, loader.LoadSkills())
	assert.Equal(t, 1, loader.SkillCount())
	_, ok := loader.GetSkill("skill1")
	assert.False(t, ok)
	_, ok = loader.GetSkill("skill2")
	assert.True(t, ok)
}

func TestLoader_MalformedSkillFile(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	// Create a valid skill
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "valid.md"), []byte(`---
name: valid
description: Valid skill.
---
Instructions.`), 0644))

	// Create a malformed skill (should be skipped)
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "malformed.md"), []byte(`
No frontmatter here, just text.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.LoadSkills())

	// Should only load the valid skill
	assert.Equal(t, 1, loader.SkillCount())
	_, ok := loader.GetSkill("valid")
	assert.True(t, ok)
}

func TestLoader_NonMdFiles(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	// Create a valid skill
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "valid.md"), []byte(`---
name: valid
description: Valid skill.
---
Instructions.`), 0644))

	// Create non-.md files that should be ignored
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "readme.txt"), []byte("Just a readme"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, "script.py"), []byte("print('hello')"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(skillsDir, ".hidden"), []byte("hidden file"), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.LoadSkills())

	// Should only load the .md skill
	assert.Equal(t, 1, loader.SkillCount())
}

func TestLoader_EmptySkillsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.LoadSkills())
	assert.Equal(t, 0, loader.SkillCount())
}

func TestLoader_NestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	assert.NoError(t, os.MkdirAll(skillsDir, 0755))

	// Create skill in a subdirectory with SKILL.md
	skillSubDir := filepath.Join(skillsDir, "my-skill")
	assert.NoError(t, os.MkdirAll(skillSubDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(skillSubDir, "SKILL.md"), []byte(`---
name: my-skill
description: Skill in subdirectory.
---
Instructions.`), 0644))

	// Create a nested directory (should NOT be scanned - only one level deep)
	deepDir := filepath.Join(skillSubDir, "nested")
	assert.NoError(t, os.MkdirAll(deepDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(deepDir, "SKILL.md"), []byte(`---
name: nested-skill
description: Should not be loaded.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.LoadSkills())

	// Should only load the top-level skill directory
	assert.Equal(t, 1, loader.SkillCount())
	_, ok := loader.GetSkill("my-skill")
	assert.True(t, ok)
	_, ok = loader.GetSkill("nested-skill")
	assert.False(t, ok)
}

func TestLoader_SkillMdCaseSensitivity(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	skillSubDir := filepath.Join(skillsDir, "test-skill")
	assert.NoError(t, os.MkdirAll(skillSubDir, 0755))

	// Create skill.md (lowercase) in a subdirectory
	// The loader looks specifically for "SKILL.md" in subdirectories
	assert.NoError(t, os.WriteFile(filepath.Join(skillSubDir, "skill.md"), []byte(`---
name: lowercase-skill
description: Lowercase skill.md.
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.LoadSkills())

	// Note: Behavior depends on filesystem case sensitivity
	// On case-insensitive filesystems (macOS default), skill.md == SKILL.md
	// On case-sensitive filesystems (Linux), skill.md != SKILL.md
	// We just verify the loader doesn't error - actual behavior is filesystem-dependent
	// The loader explicitly looks for "SKILL.md", so on case-sensitive systems this would be 0
}

func TestLoader_SkillDirectoryWithSupportingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".dive", "skills")
	skillSubDir := filepath.Join(skillsDir, "complex-skill")
	assert.NoError(t, os.MkdirAll(skillSubDir, 0755))

	// Create SKILL.md
	assert.NoError(t, os.WriteFile(filepath.Join(skillSubDir, "SKILL.md"), []byte(`---
name: complex-skill
description: Complex skill with supporting files.
---
See reference.md for details.`), 0644))

	// Create supporting files (should be ignored by loader, but exist)
	assert.NoError(t, os.WriteFile(filepath.Join(skillSubDir, "reference.md"), []byte("Reference content"), 0644))
	assert.NoError(t, os.WriteFile(filepath.Join(skillSubDir, "examples.md"), []byte("Examples"), 0644))
	scriptsDir := filepath.Join(skillSubDir, "scripts")
	assert.NoError(t, os.MkdirAll(scriptsDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(scriptsDir, "helper.py"), []byte("# helper"), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.LoadSkills())

	// Should only load the main skill, supporting files don't create separate skills
	assert.Equal(t, 1, loader.SkillCount())
	skill, ok := loader.GetSkill("complex-skill")
	assert.True(t, ok)
	assert.Contains(t, skill.Instructions, "reference.md")
}

func TestLoader_PriorityOrder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the same skill in all four locations with different descriptions
	projectDive := filepath.Join(tmpDir, "project", ".dive", "skills")
	projectClaude := filepath.Join(tmpDir, "project", ".claude", "skills")
	homeDive := filepath.Join(tmpDir, "home", ".dive", "skills")
	homeClaude := filepath.Join(tmpDir, "home", ".claude", "skills")

	for _, dir := range []string{projectDive, projectClaude, homeDive, homeClaude} {
		assert.NoError(t, os.MkdirAll(dir, 0755))
	}

	// Create same skill in each location
	assert.NoError(t, os.WriteFile(filepath.Join(projectDive, "priority.md"), []byte(`---
name: priority
description: From project .dive (should win).
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(projectClaude, "priority.md"), []byte(`---
name: priority
description: From project .claude (second priority).
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(homeDive, "priority.md"), []byte(`---
name: priority
description: From home .dive (third priority).
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(homeClaude, "priority.md"), []byte(`---
name: priority
description: From home .claude (lowest priority).
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: filepath.Join(tmpDir, "project"),
		HomeDir:    filepath.Join(tmpDir, "home"),
	})
	assert.NoError(t, loader.LoadSkills())

	// Project .dive should win
	skill, ok := loader.GetSkill("priority")
	assert.True(t, ok)
	assert.Equal(t, "From project .dive (should win).", skill.Description)
}

func TestLoader_DefaultPaths(t *testing.T) {
	// Test with empty options - should use current dir and home dir
	loader := NewLoader(LoaderOptions{})

	// This should not panic or error, even if directories don't exist
	err := loader.LoadSkills()
	assert.NoError(t, err)
}

func TestLoader_ListSkillsEmpty(t *testing.T) {
	loader := NewLoader(LoaderOptions{
		ProjectDir: "/nonexistent",
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.LoadSkills())

	skills := loader.ListSkills()
	assert.NotNil(t, skills)
	assert.Equal(t, 0, len(skills))

	names := loader.ListSkillNames()
	assert.NotNil(t, names)
	assert.Equal(t, 0, len(names))
}

func TestSkill_IsToolAllowed_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		allowedTools []string
		toolName     string
		want         bool
	}{
		{
			name:         "empty tool name",
			allowedTools: []string{"Read", "Write"},
			toolName:     "",
			want:         false,
		},
		{
			name:         "tool name with spaces not matching",
			allowedTools: []string{"Read", "Write"},
			toolName:     "Read ",
			want:         false,
		},
		{
			name:         "mixed case in allowed list",
			allowedTools: []string{"ReAd", "WrItE"},
			toolName:     "read",
			want:         true,
		},
		{
			name:         "unicode tool name",
			allowedTools: []string{"読む", "書く"},
			toolName:     "読む",
			want:         true,
		},
		{
			name:         "tool with numbers",
			allowedTools: []string{"Tool1", "Tool2"},
			toolName:     "tool1",
			want:         true,
		},
		{
			name:         "tool with underscores",
			allowedTools: []string{"my_tool", "other_tool"},
			toolName:     "MY_TOOL",
			want:         true,
		},
		{
			name:         "tool with hyphens",
			allowedTools: []string{"my-tool", "other-tool"},
			toolName:     "MY-TOOL",
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill := &Skill{AllowedTools: tt.allowedTools}
			assert.Equal(t, tt.want, skill.IsToolAllowed(tt.toolName))
		})
	}
}
