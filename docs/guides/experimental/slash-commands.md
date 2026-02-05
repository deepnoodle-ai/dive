# Slash Commands Guide

> **Experimental**: This package is in `experimental/slashcmd/`. The API may change.

Slash commands are user-invocable actions that execute from the CLI via a `/command` syntax. They provide quick access to common operations through markdown-defined instructions.

## Command File Format

Commands are defined in markdown files with optional YAML frontmatter:

```markdown
---
description: Review code for best practices
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
argument-hint: [file-pattern]
---

Review the current codebase for:

1. Code quality and readability
2. Security vulnerabilities
3. Performance implications
```

### Frontmatter Fields

| Field           | Required | Description                         |
| --------------- | -------- | ----------------------------------- |
| `description`   | No       | Shown in `/help` output             |
| `allowed-tools` | No       | Tools permitted when command runs   |
| `model`         | No       | Model override for this command     |
| `argument-hint` | No       | Shows expected arguments in `/help` |

## Argument Placeholders

| Placeholder      | Description          |
| ---------------- | -------------------- |
| `$1`, `$2`, etc. | Positional arguments |
| `$ARGUMENTS`     | Full argument string |

Example command file (`.dive/commands/fix-issue.md`):

```markdown
---
description: Fix a GitHub issue
argument-hint: [issue-number]
---

Fix issue #$1.

1. Fetch the issue details
2. Understand the problem
3. Implement the fix
```

Usage: `/fix-issue 123`

## Command Discovery

Commands are discovered from:

1. `./.dive/commands/` (project-level)
2. `./.claude/commands/` (project-level, Claude compatibility)
3. `~/.dive/commands/` (user-level)
4. `~/.claude/commands/` (user-level, Claude compatibility)

## Programmatic Usage

```go
import "github.com/deepnoodle-ai/dive/experimental/slashcmd"

loader := slashcmd.NewLoader(slashcmd.LoaderOptions{
    ProjectDir: ".",
})
if err := loader.LoadCommands(); err != nil {
    log.Fatal(err)
}

// Get and expand a command
cmd, ok := loader.GetCommand("fix-issue")
if ok {
    expanded := cmd.ExpandArguments("123")
    // Send expanded text to agent
}
```

## Best Practices

1. Write clear descriptions for `/help` output
2. Use `argument-hint` to show expected arguments
3. Restrict tools when commands don't need full access
4. Put project commands in `.dive/commands/`, personal in `~/.dive/commands/`
