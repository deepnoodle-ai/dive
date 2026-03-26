# Skills Guide

Skills are modular capabilities that extend agent functionality through specialized instructions. They provide focused expertise for specific tasks, activated automatically by the agent or manually by users via `/name` syntax.

Skills and slash commands are unified: a slash command is simply a skill without a description.

## Quick Start

```go
loader := skill.NewLoader(skill.LoaderOptions{ProjectDir: "."})
loader.Load(ctx)

opts := dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        anthropic.New(),
    Tools:        tools,
}
skill.ConfigureAgent(&opts, loader)

agent, _ := dive.NewAgent(opts)
```

`ConfigureAgent` wires up everything — the Skill tool, catalog injection into conversation context, and usage rules in the system prompt. See [Agent Integration](#agent-integration) for details.

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
argument-hint: "[file-pattern]"
triggers:
  - keyword: review
  - pattern: "review .+"
---

# Code Reviewer

Review the specified files for issues.
Target: $ARGUMENTS
```

### Frontmatter Fields

| Field           | Required | Description                                          |
| --------------- | -------- | ---------------------------------------------------- |
| `name`          | No       | Unique identifier (defaults to filename/directory)   |
| `description`   | No       | Brief explanation for the LLM; presence makes it agent-invocable |
| `allowed-tools` | No       | Metadata only — parsed but not enforced at runtime   |
| `model`         | No       | Model override for this skill (reserved for future use) |
| `argument-hint` | No       | Describes expected arguments (shown in CLI help) |
| `triggers`      | No       | Keyword/regex patterns for automatic skill suggestion |
| `user-invocable`| No       | Override: `true` = command only, `false` = skill only |

Frontmatter is optional. Files without `---` are treated as commands.

### Skills vs. Commands

| | Skill | Command |
|---|---|---|
| Has `description` | Yes | No |
| Agent can auto-invoke | Yes | No |
| User can invoke via `/name` | Yes | Yes |
| Appears in skill catalog | Yes | No |

A skill with a description is both agent-invocable and user-invocable. Setting `user-invocable: true` forces it to be user-only even with a description.

## Variable Expansion

Skills support three kinds of variable substitution in their instructions:

| Syntax | Description | Example |
|--------|-------------|---------|
| `$ARGUMENTS` | Full argument string | `/deploy staging prod` → `"staging prod"` |
| `$1`, `$2`, ..., `$9` | Positional arguments | `/deploy staging` → `$1` = `"staging"` |
| `!{command}` | Shell command output | `!{git branch --show-current}` → `"main"` |

Shell expansion (`!{...}`) is **disabled by default** for security. Enable it with `skill.WithConfigShellExpansion(true)` in `ConfigureAgent`, or `skill.WithShellExpansion(true)` when calling `Expand()` directly.

**Security:** Shell expansion is only allowed for local skills (`file://` or empty SourceURI). Skills loaded from remote providers (e.g., custom HTTP providers) never get shell expansion regardless of configuration. This is enforced by `Skill.IsLocal()`.

## Skill Discovery

Skills are discovered from multiple locations in priority order:

| Priority | Path | Scope |
|----------|------|-------|
| 1 | `.dive/skills/` | Project |
| 2 | `.dive/commands/` | Project |
| 3 | `.agents/skills/` | Project (generic cross-tool standard) |
| 4 | `.claude/skills/` | Project (Claude compatibility) |
| 5 | `.claude/commands/` | Project (Claude compatibility) |
| 6 | `~/.dive/skills/` | User |
| 7 | `~/.dive/commands/` | User |
| 8 | `~/.agents/skills/` | User (generic cross-tool standard) |
| 9 | `~/.claude/skills/` | User (Claude compatibility) |
| 10 | `~/.claude/commands/` | User (Claude compatibility) |

The first skill found with a given name takes precedence. The `.agents/skills/` path follows the generic standard used by Codex CLI, enabling cross-tool skill sharing.

Symlinked skill directories are supported — the filesystem provider resolves symlinks when scanning, so you can symlink skill directories from other locations into your skills path.

### Organization Patterns

**Directory-based** (for skills with supporting files):
```text
.dive/skills/
└── code-reviewer/
    ├── SKILL.md
    ├── references/
    └── scripts/
```

**File-based** (for simple skills):
```text
.dive/skills/
└── helper.md
```

Commands use `COMMAND.md` as the directory marker instead of `SKILL.md`.

## Agent Integration

### Three-Layer Architecture

Dive's skill integration follows Claude Code's three-layer architecture:

```text
┌──────────────────────────────────────────────────────┐
│ Layer 1: Rules                                       │
│ Skill usage instructions in the system prompt        │
├──────────────────────────────────────────────────────┤
│ Layer 2: Catalog                                     │
│ Skill names, descriptions, and triggers injected     │
│ as <system-reminder name="skills"> in the first      │
│ user message (NOT in the tool description)            │
├──────────────────────────────────────────────────────┤
│ Layer 3: Tool                                        │
│ Trigger: returns "Launching skill: X"                │
│ Full content injected via PostToolUseHook             │
└──────────────────────────────────────────────────────┘
```

The key insight: the skill catalog is injected into the **first user message** via `dive.SetSystemReminder`, not repeated in the tool description on every LLM request. This is stable for prompt caching — it sits right after the system prompt and doesn't move as the conversation grows.

### ConfigureAgent (Recommended)

`skill.ConfigureAgent` sets up all three layers in one call:

```go
loader := skill.NewLoader(skill.LoaderOptions{ProjectDir: "."})
loader.Load(ctx)

opts := dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        anthropic.New(),
    Tools: []dive.Tool{
        toolkit.NewReadFileTool(),
        toolkit.NewGrepTool(),
        toolkit.NewGlobTool(),
    },
}

// Adds: Skill tool, catalog hook, content hook, system prompt rules
skill.ConfigureAgent(&opts, loader)

// Enable shell expansion for local skills:
// skill.ConfigureAgent(&opts, loader, skill.WithConfigShellExpansion(true))

agent, _ := dive.NewAgent(opts)
```

What `ConfigureAgent` does internally:
1. **Always registers hooks** — even with zero skills, hooks are installed so that stale catalog blocks from a previous session can be cleaned up on resume.
2. **Appends the Skill tool** to `opts.Tools` (only when skills are loaded)
3. **Appends skill usage rules** to `opts.SystemPrompt` (only when skills are loaded)
4. Registers a **PreGenerationHook** that injects the skill catalog as a `<system-reminder name="skills">` block into the first user message (replaced in place if the catalog changes; removed if skills become empty)
5. Registers a **PostToolUseHook** that injects expanded skill instructions as `AdditionalContext` on the tool result message, keyed by tool call ID for correct association under parallel execution

### Catalog Injection

The catalog is injected into the first user message as a named `<system-reminder>` block:

```xml
<system-reminder name="skills">
The following skills are available for use with the Skill tool:

- code-reviewer: Review code for best practices and potential issues.
  Location: /home/user/.dive/skills/code-reviewer/SKILL.md
  TRIGGER when: user mentions "review"
- deploy: Deploy the current project to an environment.
  Location: /home/user/project/.claude/skills/deploy.md

When a task matches a skill's description or trigger, invoke the Skill
tool with the skill name before proceeding. Do not guess skill names —
only use skills listed above.
</system-reminder>
```

Only agent-invocable skills appear in the catalog. Commands are excluded. Each entry includes its `Location:` on disk so the agent can tell the user where a skill lives. The block is managed by `dive.SetSystemReminder` — a general-purpose API for named blocks that any system can use.

### Skill Content Injection

When the Skill tool is invoked, it returns a brief `"Launching skill: X"`. The full expanded instructions are injected by the `PostToolUseHook` as `AdditionalContext` — a text block appended to the tool result message. This matches Claude Code's pattern where the tool triggers loading and the content appears separately.

The injected content includes:
- **Base directory** — the skill's file path parent, so the agent can resolve relative paths to reference files (e.g., `references/05-prd.md`)
- **Expanded instructions** — with `$ARGUMENTS`, `$1`-`$9`, and `!{command}` substituted

Content is keyed by tool call ID internally, so parallel Skill tool calls in a single response each get their correct instructions regardless of completion order.

### Skill Reference File Access

Skills often include reference files alongside the SKILL.md (e.g., `references/`, `examples/`). By default, the workspace `PathValidator` restricts reads to the project directory, which blocks access to user-level skill references in `~/.claude/skills/`.

To allow the agent to read skill reference files, use a shared `PathValidator` and add skill base directories:

```go
validator, _ := toolkit.NewPathValidator(workspaceDir)

// After loading skills, allow read access to their directories
for _, dir := range skillLoader.BaseDirs() {
    _ = validator.AllowReadPath(dir)
}

// Pass the shared validator to tools
tools := []dive.Tool{
    toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{Validator: validator}),
    toolkit.NewGlobTool(toolkit.GlobToolOptions{Validator: validator}),
    // ...
}
```

All toolkit tools accept a `Validator` field that takes precedence over `WorkspaceDir`. Existing code using `WorkspaceDir` is unaffected.

### Session Resume

The catalog hook handles session resume correctly:
- On a fresh process resuming a session, stale catalog blocks from a previous run are detected and updated (or removed if skills are no longer available)
- Hooks are always installed by `ConfigureAgent` (even with zero skills) specifically to handle this cleanup

## Provider System

The loader supports pluggable providers for loading skills from different sources.

### Filesystem Provider (default)

When no providers are specified, the loader uses a filesystem provider that scans the standard directories listed above.

### Custom Providers

Implement the `Provider` interface to load skills from any source:

```go
type Provider interface {
    Name() string
    Load(ctx context.Context) ([]*Skill, error)
}
```

Example: a database-backed provider, a Git provider, or an API-backed provider. Skills loaded from non-local providers have `SourceURI` set to a non-`file://` URI, which prevents shell expansion for security.

## Trigger Matching

Skills can define triggers for automatic suggestion:

```yaml
triggers:
  - keyword: deploy       # Case-insensitive substring match
  - pattern: "deploy .+"  # Regular expression match
```

Use `loader.Match(input)` to find skills whose triggers match user input. This enables CLI-level skill suggestions before the LLM sees the input.

## Migration from Experimental

The `experimental/skill/` and `experimental/slashcmd/` packages are deprecated. The new `skill/` package is a drop-in replacement:

| Experimental | Production |
|---|---|
| `experimental/skill.NewLoader` | `skill.NewLoader` |
| `loader.LoadSkills()` | `loader.Load(ctx)` |
| `loader.GetSkill(name)` | `loader.Get(name)` |
| `loader.ListSkills()` | `loader.List()` |
| `experimental/slashcmd.NewLoader` | `skill.NewLoader` (unified) |
| `loader.LoadCommands()` | `loader.Load(ctx)` |
| `loader.GetCommand(name)` | `loader.Get(name)` |
| Manual tool + toolset wiring | `skill.ConfigureAgent(&opts, loader)` |

Backward-compatible aliases (`LoadSkills`, `GetSkill`, `GetCommand`, etc.) are provided on the new Loader.
