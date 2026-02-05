# Skills Guide

> **Experimental**: This package is in `experimental/skill/`. The API may change.

Skills are modular capabilities that extend agent functionality through specialized instructions. When activated, a skill provides focused instructions to the agent.

## Skill File Format

Skills are defined in Markdown files with YAML frontmatter:

```markdown
---
name: code-reviewer
description: Review code for best practices and potential issues.
---

# Code Reviewer

You are a code reviewer focused on identifying issues and suggesting improvements.

## Instructions

1. Read the target files using the Read tool
2. Analyze code for bugs, security issues, and readability
3. Provide actionable, specific feedback
```

### Frontmatter Fields

| Field         | Required | Description                              |
| ------------- | -------- | ---------------------------------------- |
| `name`        | No       | Unique identifier (defaults to filename) |
| `description` | No       | Brief explanation shown to the LLM       |

## Skill Discovery

Skills are discovered from multiple locations in priority order:

1. `./.dive/skills/` (project-level)
2. `./.claude/skills/` (project-level, Claude compatibility)
3. `~/.dive/skills/` (user-level)
4. `~/.claude/skills/` (user-level, Claude compatibility)

The first skill found with a given name takes precedence.

## Using Skills with Agents

```go
import (
    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/experimental/skill"
    "github.com/deepnoodle-ai/dive/experimental/toolkit/extended"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
    "github.com/deepnoodle-ai/dive/toolkit"
)

// Create a skill loader
loader := skill.NewLoader(skill.LoaderOptions{
    ProjectDir: ".",
})
if err := loader.LoadSkills(); err != nil {
    log.Fatal(err)
}

// Create the skill tool (from experimental/toolkit/extended)
skillTool := extended.NewSkillTool(extended.SkillToolOptions{
    Loader: loader,
})

// Create an agent with skills
agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:         "Skilled Assistant",
    SystemPrompt: "You are a helpful assistant with access to specialized skills.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        dive.ToolAdapter(skillTool),
        toolkit.NewReadFileTool(),
        toolkit.NewGrepTool(),
        toolkit.NewGlobTool(),
    },
})

response, _ := agent.CreateResponse(ctx,
    dive.WithInput("Review the code in main.go"),
)
fmt.Println(response.OutputText())
```

## Best Practices

1. Write descriptions that help the LLM understand when to use the skill
2. Keep skills focused on a single domain or task type
3. Put project-specific skills in `.dive/skills/`, personal skills in `~/.dive/skills/`
