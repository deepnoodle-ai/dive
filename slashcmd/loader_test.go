package slashcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestParseCommandContent(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		filePath     string
		wantName     string
		wantDesc     string
		wantTools    []string
		wantModel    string
		wantArgHint  string
		wantInstr    string
		wantErr      bool
		errContains  string
	}{
		{
			name: "valid command with all fields",
			content: "---\ndescription: Review code for best practices.\nallowed-tools:\n  - Read\n  - Grep\n  - Glob\nmodel: claude-sonnet-4-5-20250929\nargument-hint: \"[file-pattern]\"\n---\n\n# Code Review\n\nReview files matching: $ARGUMENTS",
			filePath:    "/path/to/review.md",
			wantName:    "review",
			wantDesc:    "Review code for best practices.",
			wantTools:   []string{"Read", "Grep", "Glob"},
			wantModel:   "claude-sonnet-4-5-20250929",
			wantArgHint: "[file-pattern]",
			wantInstr:   "# Code Review\n\nReview files matching: $ARGUMENTS",
		},
		{
			name: "command without frontmatter",
			content: `# Simple Command

Just do the thing.`,
			filePath:  "/path/to/simple.md",
			wantName:  "simple",
			wantInstr: "# Simple Command\n\nJust do the thing.",
		},
		{
			name: "command with only description",
			content: `---
description: A helper command.
---

Help with tasks.`,
			filePath:  "/path/to/helper.md",
			wantName:  "helper",
			wantDesc:  "A helper command.",
			wantInstr: "Help with tasks.",
		},
		{
			name: "derive name from directory",
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
		{
			name: "missing closing delimiter",
			content: `---
description: Missing closing`,
			filePath:    "/path/to/incomplete.md",
			wantErr:     true,
			errContains: "missing closing frontmatter delimiter",
		},
		{
			name: "invalid YAML",
			content: `---
description: [invalid yaml
---

Instructions`,
			filePath:    "/path/to/invalid.md",
			wantErr:     true,
			errContains: "parsing command frontmatter",
		},
		{
			name: "leading whitespace before frontmatter",
			content: `
---
description: Has leading whitespace.
---

Instructions`,
			filePath:  "/path/to/whitespace.md",
			wantName:  "whitespace",
			wantDesc:  "Has leading whitespace.",
			wantInstr: "Instructions",
		},
		{
			name:        "argument placeholders in content",
			content:     "---\ndescription: Fix an issue.\nargument-hint: \"[issue-number] [priority]\"\n---\n\nFix issue #$1 with priority $2.\nFull args: $ARGUMENTS",
			filePath:    "/path/to/fix-issue.md",
			wantName:    "fix-issue",
			wantDesc:    "Fix an issue.",
			wantArgHint: "[issue-number] [priority]",
			wantInstr:   "Fix issue #$1 with priority $2.\nFull args: $ARGUMENTS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := ParseCommandContent([]byte(tt.content), tt.filePath)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, cmd)
			assert.Equal(t, tt.wantName, cmd.Name)
			assert.Equal(t, tt.wantDesc, cmd.Description)
			assert.Equal(t, tt.wantModel, cmd.Model)
			assert.Equal(t, tt.wantArgHint, cmd.ArgumentHint)
			if tt.wantTools == nil {
				assert.Nil(t, cmd.AllowedTools)
			} else {
				assert.Equal(t, len(tt.wantTools), len(cmd.AllowedTools))
				for i, tool := range tt.wantTools {
					assert.Equal(t, tool, cmd.AllowedTools[i])
				}
			}
			assert.Equal(t, tt.wantInstr, cmd.Instructions)
			assert.Equal(t, tt.filePath, cmd.FilePath)
		})
	}
}

func TestCommand_ExpandArguments(t *testing.T) {
	tests := []struct {
		name         string
		instructions string
		args         string
		want         string
	}{
		{
			name:         "single positional argument",
			instructions: "Fix issue #$1",
			args:         "123",
			want:         "Fix issue #123",
		},
		{
			name:         "multiple positional arguments",
			instructions: "Fix issue #$1 with priority $2",
			args:         "123 high",
			want:         "Fix issue #123 with priority high",
		},
		{
			name:         "ARGUMENTS placeholder",
			instructions: "Process: $ARGUMENTS",
			args:         "file1.txt file2.txt",
			want:         "Process: file1.txt file2.txt",
		},
		{
			name:         "mixed placeholders",
			instructions: "Command: $1, Priority: $2, All: $ARGUMENTS",
			args:         "cmd high extra",
			want:         "Command: cmd, Priority: high, All: cmd high extra",
		},
		{
			name:         "unused positional placeholder",
			instructions: "Arg1: $1, Arg2: $2, Arg3: $3",
			args:         "one two",
			want:         "Arg1: one, Arg2: two, Arg3: $3",
		},
		{
			name:         "empty arguments",
			instructions: "Process: $ARGUMENTS, First: $1",
			args:         "",
			want:         "Process: , First: $1",
		},
		{
			name:         "no placeholders",
			instructions: "Just do the thing.",
			args:         "ignored args",
			want:         "Just do the thing.",
		},
		{
			name:         "repeated placeholders",
			instructions: "First: $1, Again: $1",
			args:         "value",
			want:         "First: value, Again: value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Command{Instructions: tt.instructions}
			result := cmd.ExpandArguments(tt.args)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestCommand_IsToolAllowed(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Command{AllowedTools: tt.allowedTools}
			assert.Equal(t, tt.want, cmd.IsToolAllowed(tt.toolName))
		})
	}
}

func TestLoader_LoadCommands(t *testing.T) {
	tmpDir := t.TempDir()

	// Create project .dive/commands directory
	projectDiveCommands := filepath.Join(tmpDir, "project", ".dive", "commands")
	assert.NoError(t, os.MkdirAll(projectDiveCommands, 0755))

	// Create project .claude/commands directory
	projectClaudeCommands := filepath.Join(tmpDir, "project", ".claude", "commands")
	assert.NoError(t, os.MkdirAll(projectClaudeCommands, 0755))

	// Create home .dive/commands directory
	homeDiveCommands := filepath.Join(tmpDir, "home", ".dive", "commands")
	assert.NoError(t, os.MkdirAll(homeDiveCommands, 0755))

	// Create command in directory format
	cmdDir := filepath.Join(projectDiveCommands, "code-review")
	assert.NoError(t, os.MkdirAll(cmdDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(cmdDir, "COMMAND.md"), []byte(`---
description: Review code.
allowed-tools:
  - Read
  - Grep
---

Review the code.`), 0644))

	// Create standalone command file
	assert.NoError(t, os.WriteFile(filepath.Join(projectClaudeCommands, "helper.md"), []byte(`---
description: A helper command.
---

Helper instructions.`), 0644))

	// Create command in home directory (should be lower priority)
	assert.NoError(t, os.WriteFile(filepath.Join(homeDiveCommands, "helper.md"), []byte(`---
description: Home helper - should be ignored.
---

Home instructions.`), 0644))

	// Create another home command that should be loaded
	assert.NoError(t, os.WriteFile(filepath.Join(homeDiveCommands, "personal.md"), []byte(`---
description: Personal command.
---

Personal instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: filepath.Join(tmpDir, "project"),
		HomeDir:    filepath.Join(tmpDir, "home"),
	})

	err := loader.LoadCommands()
	assert.NoError(t, err)

	// Check loaded commands
	assert.Equal(t, 3, loader.CommandCount())

	// Check code-review command
	cmd, ok := loader.GetCommand("code-review")
	assert.True(t, ok)
	assert.Equal(t, "code-review", cmd.Name)
	assert.Equal(t, "Review code.", cmd.Description)
	assert.Equal(t, 2, len(cmd.AllowedTools))
	assert.Equal(t, "project", cmd.Source)

	// Check helper command (project one should win)
	cmd, ok = loader.GetCommand("helper")
	assert.True(t, ok)
	assert.Equal(t, "A helper command.", cmd.Description)
	assert.Equal(t, "project", cmd.Source)

	// Check personal command
	cmd, ok = loader.GetCommand("personal")
	assert.True(t, ok)
	assert.Equal(t, "Personal command.", cmd.Description)
	assert.Equal(t, "user", cmd.Source)

	// Check non-existent command
	_, ok = loader.GetCommand("non-existent")
	assert.False(t, ok)

	// Check ListCommands returns sorted
	commands := loader.ListCommands()
	assert.Equal(t, 3, len(commands))
	assert.Equal(t, "code-review", commands[0].Name)
	assert.Equal(t, "helper", commands[1].Name)
	assert.Equal(t, "personal", commands[2].Name)

	// Check ListCommandNames
	names := loader.ListCommandNames()
	assert.Equal(t, 3, len(names))
	assert.Equal(t, "code-review", names[0])
	assert.Equal(t, "helper", names[1])
	assert.Equal(t, "personal", names[2])
}

func TestLoader_DisablePaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create commands in both Dive and Claude paths
	diveCommands := filepath.Join(tmpDir, ".dive", "commands")
	claudeCommands := filepath.Join(tmpDir, ".claude", "commands")
	assert.NoError(t, os.MkdirAll(diveCommands, 0755))
	assert.NoError(t, os.MkdirAll(claudeCommands, 0755))

	assert.NoError(t, os.WriteFile(filepath.Join(diveCommands, "dive-cmd.md"), []byte(`---
description: Dive command.
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(claudeCommands, "claude-cmd.md"), []byte(`---
description: Claude command.
---
Instructions.`), 0644))

	t.Run("disable Claude paths", func(t *testing.T) {
		loader := NewLoader(LoaderOptions{
			ProjectDir:         tmpDir,
			HomeDir:            "/nonexistent",
			DisableClaudePaths: true,
		})
		assert.NoError(t, loader.LoadCommands())

		_, ok := loader.GetCommand("dive-cmd")
		assert.True(t, ok)

		_, ok = loader.GetCommand("claude-cmd")
		assert.False(t, ok)
	})

	t.Run("disable Dive paths", func(t *testing.T) {
		loader := NewLoader(LoaderOptions{
			ProjectDir:       tmpDir,
			HomeDir:          "/nonexistent",
			DisableDivePaths: true,
		})
		assert.NoError(t, loader.LoadCommands())

		_, ok := loader.GetCommand("dive-cmd")
		assert.False(t, ok)

		_, ok = loader.GetCommand("claude-cmd")
		assert.True(t, ok)
	})
}

func TestLoader_PriorityOrder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the same command in all four locations with different descriptions
	projectDive := filepath.Join(tmpDir, "project", ".dive", "commands")
	projectClaude := filepath.Join(tmpDir, "project", ".claude", "commands")
	homeDive := filepath.Join(tmpDir, "home", ".dive", "commands")
	homeClaude := filepath.Join(tmpDir, "home", ".claude", "commands")

	for _, dir := range []string{projectDive, projectClaude, homeDive, homeClaude} {
		assert.NoError(t, os.MkdirAll(dir, 0755))
	}

	// Create same command in each location
	assert.NoError(t, os.WriteFile(filepath.Join(projectDive, "priority.md"), []byte(`---
description: From project .dive (should win).
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(projectClaude, "priority.md"), []byte(`---
description: From project .claude (second priority).
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(homeDive, "priority.md"), []byte(`---
description: From home .dive (third priority).
---
Instructions.`), 0644))

	assert.NoError(t, os.WriteFile(filepath.Join(homeClaude, "priority.md"), []byte(`---
description: From home .claude (lowest priority).
---
Instructions.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: filepath.Join(tmpDir, "project"),
		HomeDir:    filepath.Join(tmpDir, "home"),
	})
	assert.NoError(t, loader.LoadCommands())

	// Project .dive should win
	cmd, ok := loader.GetCommand("priority")
	assert.True(t, ok)
	assert.Equal(t, "From project .dive (should win).", cmd.Description)
}

func TestLoader_MissingDirectories(t *testing.T) {
	loader := NewLoader(LoaderOptions{
		ProjectDir: "/nonexistent/project",
		HomeDir:    "/nonexistent/home",
	})

	// Should not error on missing directories
	err := loader.LoadCommands()
	assert.NoError(t, err)
	assert.Equal(t, 0, loader.CommandCount())
}

func TestLoader_CommandWithoutFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	commandsDir := filepath.Join(tmpDir, ".dive", "commands")
	assert.NoError(t, os.MkdirAll(commandsDir, 0755))

	// Create command without frontmatter
	assert.NoError(t, os.WriteFile(filepath.Join(commandsDir, "simple.md"), []byte(`# Simple Command

Just do the thing. No frontmatter needed.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})
	assert.NoError(t, loader.LoadCommands())

	cmd, ok := loader.GetCommand("simple")
	assert.True(t, ok)
	assert.Equal(t, "simple", cmd.Name)
	assert.Equal(t, "", cmd.Description)
	assert.Contains(t, cmd.Instructions, "Just do the thing")
}

func TestLoader_InvalidCommandFiles(t *testing.T) {
	tmpDir := t.TempDir()
	commandsDir := filepath.Join(tmpDir, ".dive", "commands")
	assert.NoError(t, os.MkdirAll(commandsDir, 0755))

	// Create a valid command
	assert.NoError(t, os.WriteFile(filepath.Join(commandsDir, "valid.md"), []byte(`---
description: Valid command.
---
Instructions.`), 0644))

	// Create an invalid command (unquoted YAML sequence in string field)
	assert.NoError(t, os.WriteFile(filepath.Join(commandsDir, "invalid.md"), []byte(`---
description: Invalid command.
argument-hint: [unquoted-brackets]
---
Instructions.`), 0644))

	// Create another invalid command (missing closing frontmatter)
	assert.NoError(t, os.WriteFile(filepath.Join(commandsDir, "broken.md"), []byte(`---
description: Broken command.
No closing delimiter here.`), 0644))

	loader := NewLoader(LoaderOptions{
		ProjectDir: tmpDir,
		HomeDir:    "/nonexistent",
	})

	err := loader.LoadCommands()

	// Should return an error for the invalid files
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid.md")
	assert.Contains(t, err.Error(), "broken.md")

	// Valid commands should still be loaded
	_, ok := loader.GetCommand("valid")
	assert.True(t, ok)

	// Invalid commands should not be loaded
	_, ok = loader.GetCommand("invalid")
	assert.False(t, ok)
	_, ok = loader.GetCommand("broken")
	assert.False(t, ok)
}

func TestDeriveCommandName(t *testing.T) {
	tests := []struct {
		filePath string
		wantName string
	}{
		{"/path/to/my-command/COMMAND.md", "my-command"},
		{"/path/to/my-command/command.md", "my-command"}, // case insensitive
		{"/path/to/commands/helper.md", "helper"},
		{"/path/to/commands/my-tool.md", "my-tool"},
		{"COMMAND.md", "."},
		{"test.md", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := deriveCommandName(tt.filePath)
			assert.Equal(t, tt.wantName, got)
		})
	}
}
