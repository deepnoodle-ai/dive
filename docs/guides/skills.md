# Skills Guide

Skills are modular capabilities that extend agent functionality through specialized instructions. They enable agents to autonomously activate focused behaviors based on task requirements, providing a way to inject domain-specific expertise and constrain agent actions.

## Table of Contents

- [Overview](#overview)
- [Skill File Format](#skill-file-format)
- [Skill Discovery](#skill-discovery)
- [Creating Skills](#creating-skills)
- [Tool Restrictions](#tool-restrictions)
- [Using Skills with Agents](#using-skills-with-agents)
- [Best Practices](#best-practices)

## Overview

Skills provide a declarative way to define specialized agent behaviors. When an agent activates a skill, it receives detailed instructions for handling a specific type of task. Skills can optionally restrict which tools the agent may use, providing both focus and security.

Key features:

- **Modular instructions**: Define specialized behaviors in standalone files
- **Automatic discovery**: Skills are loaded from standard locations
- **Tool restrictions**: Optionally limit which tools an agent can use
- **Priority-based loading**: Project skills override user-level skills
- **Claude compatibility**: Works with both `.dive/skills/` and `.claude/skills/` directories

## Skill File Format

Skills are defined in Markdown files with YAML frontmatter:

```markdown
---
name: code-reviewer
description: Review code for best practices and potential issues.
allowed-tools:
  - Read
  - Grep
  - Glob
---

# Code Reviewer

You are a code reviewer focused on identifying issues and suggesting improvements.

## Instructions

1. Read the target files using the Read tool
2. Analyze code for:
   - Potential bugs and logic errors
   - Security vulnerabilities
   - Performance issues
   - Code style and readability
3. Provide actionable, specific feedback
4. Suggest concrete improvements with examples

## Output Format

Structure your review as:

- **Critical Issues**: Must be fixed
- **Warnings**: Should be addressed
- **Suggestions**: Nice to have improvements
```

### Frontmatter Fields

| Field           | Required | Description                                                           |
| --------------- | -------- | --------------------------------------------------------------------- |
| `name`          | No\*     | Unique identifier for the skill (lowercase letters, numbers, hyphens) |
| `description`   | No       | Brief explanation of what the skill does (shown to the LLM)           |
| `allowed-tools` | No       | List of tools permitted when this skill is active                     |

\*If `name` is not specified, it's derived from the directory or filename.

### Name Derivation

When the `name` field is omitted:

- For `SKILL.md` files: The parent directory name becomes the skill name
- For standalone `.md` files: The filename (without extension) becomes the skill name

```
.dive/skills/
├── code-reviewer/
│   └── SKILL.md        # name: "code-reviewer" (from directory)
└── helper.md           # name: "helper" (from filename)
```

## Skill Discovery

Skills are discovered from multiple locations in priority order:

1. `./.dive/skills/` (project-level, Dive)
2. `./.claude/skills/` (project-level, Claude)
3. `~/.dive/skills/` (user-level, Dive)
4. `~/.claude/skills/` (user-level, Claude)

The first skill found with a given name takes precedence. This allows project-specific skills to override personal defaults.

### Directory Structure

Skills support two organization patterns:

**Directory-based** (recommended for complex skills):

```
.dive/skills/
└── code-reviewer/
    ├── SKILL.md           # Main skill definition
    ├── templates/         # Supporting files
    │   └── review.md
    └── examples/
        └── sample.md
```

**File-based** (for simple skills):

```
.dive/skills/
├── helper.md
├── summarizer.md
└── translator.md
```

## Creating Skills

### Simple Skill Example

Create `.dive/skills/summarizer.md`:

```markdown
---
name: summarizer
description: Summarize documents and code files concisely.
---

# Document Summarizer

Create concise summaries of documents and code files.

## Guidelines

- Focus on key points and main concepts
- Use bullet points for clarity
- Keep summaries under 200 words
- For code: explain purpose, inputs, outputs, and key logic
```

### Skill with Tool Restrictions

Create `.dive/skills/safe-reader/SKILL.md`:

```markdown
---
name: safe-reader
description: Read-only file exploration with no write capabilities.
allowed-tools:
  - Read
  - Glob
  - Grep
  - ListDirectory
---

# Safe Reader

You are a read-only assistant that can explore and analyze files
but cannot modify anything.

## Capabilities

- Read file contents
- Search for patterns in files
- List directory contents
- Find files by name patterns

## Restrictions

You cannot:

- Write or modify files
- Execute commands
- Make network requests
```

### Advanced Skill with Context

Create `.dive/skills/api-designer/SKILL.md`:

```markdown
---
name: api-designer
description: Design RESTful APIs following best practices.
allowed-tools:
  - Read
  - Write
  - Grep
  - Glob
---

# API Designer

You are an API design specialist focused on creating clean,
consistent, and well-documented RESTful APIs.

## Design Principles

1. **Resource-oriented**: URLs represent resources, not actions
2. **Consistent naming**: Use plural nouns, kebab-case
3. **Proper HTTP methods**: GET, POST, PUT, PATCH, DELETE
4. **Status codes**: Use appropriate HTTP status codes
5. **Versioning**: Include API version in URL or headers

## Output Format

When designing an API, provide:

1. Resource definitions
2. Endpoint specifications with:
   - Method and URL
   - Request/response schemas
   - Status codes
   - Example requests
3. Authentication requirements
4. Rate limiting recommendations
```

## Tool Restrictions

Skills can restrict which tools an agent may use via the `allowed-tools` field. When a skill with restrictions is active:

- Only listed tools are permitted
- The `Skill` tool is always allowed (to enable switching skills)
- Tool matching is case-insensitive (`Read`, `read`, `READ` are equivalent)
- If `allowed-tools` is empty or omitted, all tools are allowed

### Example Restrictions

```yaml
# Read-only exploration
allowed-tools:
  - Read
  - Grep
  - Glob
  - ListDirectory

# Full file access
allowed-tools:
  - Read
  - Write
  - Edit
  - ListDirectory

# Web research only
allowed-tools:
  - WebSearch
  - Fetch
  - Read
  - Write
```

## Using Skills with Agents

### Loading Skills

```go
package main

import (
    "fmt"
    "log"

    "github.com/deepnoodle-ai/dive/skill"
)

func main() {
    // Create a skill loader
    loader := skill.NewLoader(skill.LoaderOptions{
        ProjectDir: ".",  // Search from current directory
    })

    // Load skills from all configured paths
    if err := loader.LoadSkills(); err != nil {
        log.Fatal(err)
    }

    // List available skills
    fmt.Printf("Loaded %d skills:\n", loader.SkillCount())
    for _, s := range loader.ListSkills() {
        fmt.Printf("  - %s: %s\n", s.Name, s.Description)
    }

    // Get a specific skill
    if s, ok := loader.GetSkill("code-reviewer"); ok {
        fmt.Printf("\nSkill: %s\n", s.Name)
        fmt.Printf("Instructions:\n%s\n", s.Instructions)
    }
}
```

### Integrating with Agents

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
    "github.com/deepnoodle-ai/dive/skill"
    "github.com/deepnoodle-ai/dive/toolkit"
)

func main() {
    // Load skills
    loader := skill.NewLoader(skill.LoaderOptions{
        ProjectDir: ".",
    })
    if err := loader.LoadSkills(); err != nil {
        log.Fatal(err)
    }

    // Create the skill tool
    skillTool := toolkit.NewSkillTool(toolkit.SkillToolOptions{
        Loader: loader,
    })

    // Create an agent with the skill tool
    agent, err := dive.NewAgent(dive.AgentOptions{
        Name: "Skilled Assistant",
        Instructions: `You are a helpful assistant with access to specialized skills.
                      Use the Skill tool to activate skills when appropriate.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(skillTool),
            dive.ToolAdapter(toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{})),
            dive.ToolAdapter(toolkit.NewWriteFileTool(toolkit.WriteFileToolOptions{})),
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // The agent can now activate skills as needed
    response, err := agent.CreateResponse(
        context.Background(),
        dive.WithInput("Review the code in main.go for potential issues"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.OutputText())
}
```

### Checking Tool Permissions

When implementing tool restriction enforcement:

```go
// Check if a tool is allowed by the active skill
if !skillTool.IsToolAllowed("Write") {
    return errors.New("Write tool is not permitted by the active skill")
}

// Get the currently active skill
if activeSkill := skillTool.GetActiveSkill(); activeSkill != nil {
    fmt.Printf("Active skill: %s\n", activeSkill.Name)
    fmt.Printf("Allowed tools: %v\n", activeSkill.AllowedTools)
}

// Clear the active skill when done
skillTool.ClearActiveSkill()
```

### Custom Skill Paths

```go
loader := skill.NewLoader(skill.LoaderOptions{
    ProjectDir:      "/path/to/project",
    HomeDir:         "/home/user",
    AdditionalPaths: []string{
        "/shared/team-skills",
        "/custom/skills",
    },
    // Optionally disable certain paths
    DisableClaudePaths: false,  // Include .claude/skills/ paths
    DisableDivePaths:   false,  // Include .dive/skills/ paths
})
```

### Logging Skill Loading

```go
type MyLogger struct{}

func (l *MyLogger) Debug(msg string, args ...any) {
    log.Printf("[DEBUG] %s", msg)
}

func (l *MyLogger) Warn(msg string, args ...any) {
    log.Printf("[WARN] %s", msg)
}

loader := skill.NewLoader(skill.LoaderOptions{
    ProjectDir: ".",
    Logger:     &MyLogger{},
})
```

## Best Practices

### Skill Design

1. **Clear descriptions**: Write descriptions that help the LLM understand when to use the skill
2. **Focused instructions**: Keep skills focused on a single domain or task type
3. **Minimal restrictions**: Only restrict tools when necessary for security or focus
4. **Structured output**: Define clear output formats to ensure consistent results

### Organization

1. **Project vs user skills**: Put project-specific skills in `.dive/skills/`, personal skills in `~/.dive/skills/`
2. **Directory structure**: Use directories for complex skills with supporting files
3. **Naming conventions**: Use lowercase with hyphens (e.g., `code-reviewer`, `api-designer`)
4. **Documentation**: Include clear instructions and examples in skill files

### Security

1. **Principle of least privilege**: Only allow necessary tools
2. **Read-only defaults**: Default to read-only tool access when possible
3. **Review skills**: Audit skills, especially those from external sources
4. **Test restrictions**: Verify tool restrictions work as expected

### Performance

1. **Lazy loading**: Load skills once at startup, not per-request
2. **Reload on change**: Use `LoadSkills()` again to pick up file changes
3. **Cache skill instances**: The `SkillTool` maintains active skill state

## Related Documentation

- [Tools Guide](tools.md) - Built-in tools reference
- [Custom Tools Guide](custom-tools.md) - Creating custom tools
- [Agents Guide](agents.md) - Agent configuration and usage
