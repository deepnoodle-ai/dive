# Slash Commands Guide

Slash commands are user-invocable actions that execute directly from the CLI. They provide quick access to common operations and custom automation through a simple `/command` syntax.

## Table of Contents

- [Overview](#overview)
- [Built-in Commands](#built-in-commands)
- [Custom Commands](#custom-commands)
- [Command File Format](#command-file-format)
- [Argument Placeholders](#argument-placeholders)
- [Command Discovery](#command-discovery)
- [Using Commands in the CLI](#using-commands-in-the-cli)
- [Programmatic Usage](#programmatic-usage)
- [Best Practices](#best-practices)

## Overview

Slash commands start with `/` and can be typed directly into the Dive CLI. They fall into two categories:

- **Built-in commands**: Core operations like `/clear`, `/compact`, and `/help`
- **Custom commands**: User-defined commands loaded from markdown files

Key features:

- **Instant execution**: Commands run immediately without agent processing
- **Argument support**: Pass arguments with placeholders like `$1`, `$2`, `$ARGUMENTS`
- **Claude compatibility**: Uses the same file format as Claude Code commands
- **Priority-based loading**: Project commands override user-level commands
- **Tool restrictions**: Optionally limit which tools the agent can use

## Built-in Commands

| Command | Aliases | Description |
|---------|---------|-------------|
| `/help` | `/?` | Show available commands (built-in and custom) |
| `/clear` | - | Clear screen and reset conversation state |
| `/compact` | - | Manually compact conversation to save context tokens |
| `/todos` | `/t` | Toggle the todo list display |
| `/quit` | `/q`, `/exit` | Exit the application |

### /clear

Clears both the terminal screen and the conversation history:

```
> /clear
```

This resets the conversation to a fresh state, as if you just started the CLI. Use this when you want to start a new topic without carrying over previous context.

### /compact

Manually triggers context compaction to summarize older messages:

```
> /compact
Compacting conversation...
Compacted: ~15000 -> ~3000 tokens
```

This is useful when approaching token limits. The agent summarizes the conversation history while preserving important context.

### /help

Displays all available commands:

```
> /help

Built-in Commands:
  /quit, /q      Exit
  /clear         Clear conversation and screen
  /compact       Compact conversation to save context
  /todos, /t     Toggle todo list
  /help, /?      Show this help

Custom Commands:
  /review [file-pattern]
      Review code for best practices
  /fix-issue [issue-number]
      Fix a GitHub issue

Input:
  @filename      Include file contents
  Enter          Send message
  Shift+Enter    New line
  Ctrl+C twice   Exit
```

## Custom Commands

Custom commands are defined in markdown files and loaded from standard locations.

### Simple Command

Create `.dive/commands/review.md`:

```markdown
---
description: Review code for best practices
---

Review the current codebase for:
1. Code quality and readability
2. Security vulnerabilities
3. Performance implications

Provide specific, actionable feedback.
```

Usage:

```
> /review
```

### Command with Arguments

Create `.dive/commands/fix-issue.md`:

```markdown
---
description: Fix a GitHub issue
argument-hint: [issue-number]
---

Fix issue #$1.

1. Fetch the issue details from GitHub
2. Understand the problem
3. Implement the fix
4. Verify the solution
```

Usage:

```
> /fix-issue 123
```

## Command File Format

Commands are defined in markdown files with optional YAML frontmatter:

```markdown
---
description: Brief description shown in /help
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
model: claude-sonnet-4-5-20250929
argument-hint: [arg1] [arg2]
---

# Command Instructions

Detailed instructions for what the command should do...
```

### Frontmatter Fields

| Field | Required | Description |
|-------|----------|-------------|
| `description` | No | Brief explanation shown in `/help` output |
| `allowed-tools` | No | List of tools permitted when command runs |
| `model` | No | Model override for this command |
| `argument-hint` | No | Shows expected arguments in `/help` |
| `name` | No | Explicit name (defaults to filename) |

### Commands Without Frontmatter

Frontmatter is optional. A simple command can be just markdown:

```markdown
# Quick Review

Review the recent changes for issues and suggest improvements.
Focus on the files modified in the last commit.
```

## Argument Placeholders

Commands support dynamic arguments:

| Placeholder | Description |
|-------------|-------------|
| `$1`, `$2`, etc. | Positional arguments |
| `$ARGUMENTS` | Full argument string |

### Example

Command file (`.dive/commands/search.md`):

```markdown
---
description: Search for a pattern in files
argument-hint: [pattern] [file-glob]
---

Search for "$1" in files matching "$2".

Full search parameters: $ARGUMENTS
```

Usage:

```
> /search "TODO" "**/*.go"
```

Expands to:

```
Search for "TODO" in files matching "**/*.go".

Full search parameters: TODO **/*.go
```

### Unused Placeholders

If fewer arguments are provided than placeholders, unused placeholders remain in the text:

```
> /search "pattern"
# $2 stays as "$2" in the output
```

## Command Discovery

Commands are discovered from multiple locations in priority order:

1. `./.dive/commands/` (project-level, Dive)
2. `./.claude/commands/` (project-level, Claude)
3. `~/.dive/commands/` (user-level, Dive)
4. `~/.claude/commands/` (user-level, Claude)

The first command found with a given name takes precedence.

### Directory Structure

Commands support two organization patterns:

**File-based** (recommended for most commands):

```
.dive/commands/
├── review.md
├── fix-issue.md
└── deploy.md
```

**Directory-based** (for commands with supporting files):

```
.dive/commands/
└── complex-task/
    ├── COMMAND.md      # Main command definition
    └── templates/      # Supporting files
        └── checklist.md
```

### Name Derivation

When `name` is not specified in frontmatter:

- For `COMMAND.md` files: Parent directory name becomes the command name
- For standalone `.md` files: Filename (without extension) becomes the command name

## Using Commands in the CLI

### Basic Usage

```
> /review
```

### With Arguments

```
> /fix-issue 42 high
```

### Tab Completion

Commands support tab completion for `@filename` references in arguments (the same as regular input).

## Programmatic Usage

### Loading Commands

```go
package main

import (
    "fmt"
    "log"

    "github.com/deepnoodle-ai/dive/slashcmd"
)

func main() {
    // Create a command loader
    loader := slashcmd.NewLoader(slashcmd.LoaderOptions{
        ProjectDir: ".",  // Search from current directory
    })

    // Load commands from all configured paths
    if err := loader.LoadCommands(); err != nil {
        log.Fatal(err)
    }

    // List available commands
    fmt.Printf("Loaded %d commands:\n", loader.CommandCount())
    for _, cmd := range loader.ListCommands() {
        fmt.Printf("  /%s", cmd.Name)
        if cmd.ArgumentHint != "" {
            fmt.Printf(" %s", cmd.ArgumentHint)
        }
        if cmd.Description != "" {
            fmt.Printf(" - %s", cmd.Description)
        }
        fmt.Println()
    }

    // Get a specific command
    if cmd, ok := loader.GetCommand("review"); ok {
        fmt.Printf("\nCommand: %s\n", cmd.Name)
        fmt.Printf("Instructions:\n%s\n", cmd.Instructions)
    }
}
```

### Executing Commands

```go
// Get command and expand arguments
cmd, ok := loader.GetCommand("fix-issue")
if !ok {
    log.Fatal("command not found")
}

// Expand argument placeholders
expanded := cmd.ExpandArguments("123 high")
// Result: "Fix issue #123 with priority high..."

// Send to agent
response, err := agent.CreateResponse(
    context.Background(),
    dive.WithInput(expanded),
)
```

### Custom Loader Options

```go
loader := slashcmd.NewLoader(slashcmd.LoaderOptions{
    ProjectDir:      "/path/to/project",
    HomeDir:         "/home/user",
    AdditionalPaths: []string{
        "/shared/team-commands",
    },
    DisableClaudePaths: false,  // Include .claude/commands/ paths
    DisableDivePaths:   false,  // Include .dive/commands/ paths
})
```

### Checking Tool Permissions

```go
cmd, _ := loader.GetCommand("review")

// Check if a tool is allowed
if cmd.IsToolAllowed("Write") {
    // Tool is permitted
}

// Check tool restrictions
if len(cmd.AllowedTools) > 0 {
    fmt.Printf("Restricted to: %v\n", cmd.AllowedTools)
}
```

## Best Practices

### Command Design

1. **Clear descriptions**: Write descriptions that explain the command's purpose
2. **Focused scope**: Each command should do one thing well
3. **Helpful hints**: Use `argument-hint` to show expected arguments
4. **Sensible defaults**: Design commands to work without arguments when possible

### Organization

1. **Project vs user commands**: Put project-specific commands in `.dive/commands/`, personal commands in `~/.dive/commands/`
2. **Naming conventions**: Use lowercase with hyphens (e.g., `fix-issue`, `code-review`)
3. **Keep it simple**: Prefer standalone `.md` files over directory-based commands

### Security

1. **Tool restrictions**: Limit tools when commands don't need full access
2. **Review commands**: Audit commands from external sources
3. **Avoid secrets**: Don't include credentials in command files

### Examples

#### Code Review Command

`.dive/commands/review.md`:

```markdown
---
description: Comprehensive code review
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

## Recent Changes

Review the changes from the last commit.

## Checklist

1. Code quality and readability
2. Security vulnerabilities
3. Performance implications
4. Test coverage
5. Documentation

Provide specific, prioritized feedback.
```

#### Test Runner Command

`.dive/commands/test.md`:

```markdown
---
description: Run tests with optional pattern
argument-hint: [pattern]
allowed-tools:
  - Bash
  - Read
  - Grep
---

Run tests matching: $ARGUMENTS

1. Detect the test framework
2. Run matching tests
3. If tests fail, analyze and suggest fixes
```

#### Git Commit Command

`.dive/commands/commit.md`:

```markdown
---
description: Create a well-formatted git commit
allowed-tools:
  - Bash
  - Read
  - Grep
---

Create a git commit for the current changes:

1. Review staged changes with `git diff --staged`
2. Generate a clear, conventional commit message
3. Create the commit

Follow conventional commit format:
- feat: new features
- fix: bug fixes
- docs: documentation
- refactor: code refactoring
```

## Related Documentation

- [Skills Guide](skills.md) - Agent-invocable specialized behaviors
- [Tools Guide](tools.md) - Built-in tools reference
- [Agents Guide](agents.md) - Agent configuration and usage
