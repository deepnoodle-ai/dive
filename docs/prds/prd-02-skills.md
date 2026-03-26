# PRD: Production Skills System

| Field | Content |
|-------|---------|
| Title | Production Skills & Slash Commands |
| Author | Curtis / DeepNoodle |
| Status | Implemented |
| Last Updated | 2026-03-26 |
| Stakeholders | Dive library users, CLI users, agent builders |

## Problem & Opportunity

### The Problem

AI coding agents are converging on **skills** as the primary extensibility mechanism — lazily-loaded, markdown-based instruction sets that agents invoke contextually. Claude Code, Codex CLI, and Gemini CLI all have skill systems, and community consensus is clear: skills are the highest-value investment for customizing agent behavior.

Dive has experimental skill support (`experimental/skill/`) that handles basic loading and parsing. But it's missing the features that make skills powerful in practice:

1. **No variable substitution.** `$ARGUMENTS`, `$1`, `{{args}}` — all absent. Skills can't be parameterized.
2. **No command substitution.** Gemini CLI's `!{git diff --staged}` pattern lets commands inject live context. Dive has nothing equivalent.
3. **No slash command unification.** Skills and slash commands are separate packages with duplicated code. The industry has converged: a slash command is just a skill without frontmatter (no auto-invocation).
4. **No CLI integration.** The Dive CLI has zero skill support — no listing, no invocation, no management.
5. **Not thread-safe.** The `Loader` explicitly documents it is NOT safe for concurrent use.

### Why Now

1. **Industry convergence.** Claude Code merged skills and slash commands. Codex adopted SKILL.md. The format is standardizing and Dive should support it natively.
2. **Dive's experimental code is solid.** The parser, loader, and tool integration are well-tested. This is a promotion and enhancement, not a rewrite.
3. **Community demand for cross-tool skills.** Users want skills that work across Claude Code, Codex, and Dive without modification. Dive should be a first-class citizen in that ecosystem.
4. **The Go API use case is unique.** No other skill system provides a Go library for programmatic skill management. Dive can offer features like custom skill providers, runtime skill registration, and dynamic catalog injection — things CLI-only tools can't.

## Goals & Success Metrics

| Goal | Metric |
|------|--------|
| **Primary:** Go developers can load, discover, and invoke skills with a production-quality API | Skills load from filesystem and custom providers with one unified interface |
| **Primary:** Full feature parity with Claude Code / Codex skill semantics | Variable substitution, command substitution, auto-invocation triggers all working |
| **Secondary:** The CLI provides a complete skill experience | `dive skills list`, `/skill-name`, and agent auto-invocation all work in the CLI |
| **Secondary:** Cross-tool compatibility | Claude Code SKILL.md files work in Dive without modification |
| **Guardrail:** No breaking changes to the core `dive` package | Existing agent code compiles and runs unchanged |
| **Guardrail:** The API is simple for the common case | Loading filesystem skills requires <5 lines of Go |

## Target Users

### Primary: Go developers building AI agents (Library API)

Developers using Dive's Go API to build agents that need customizable, domain-specific behaviors. They want to load skills from the filesystem or from a custom backend. They may be building platforms where end users define skills.

### Secondary: CLI power users (Dive CLI)

Developers using the Dive CLI interactively who want `/review-code` to just work, want to see available skills, and want the agent to auto-invoke skills when appropriate. They expect parity with Claude Code's skill UX.

### Tertiary: Teams sharing skills across tools

Teams that maintain skill libraries in Git repositories and use them across Claude Code, Codex, and Dive. They need format compatibility and predictable behavior.

## Proposed Solution

### Architecture Overview

The skill system has three distinct layers, modeled after how Claude Code implements skills internally. This separation is critical — conflating them (e.g., stuffing the skill catalog into the tool description) wastes tokens and reduces reliability.

```text
┌─────────────────────────────────────────────────────┐
│ Layer 1: Rules                                      │
│ System prompt instructions for how to use skills    │
│ (injected by ConfigureAgent when skills are loaded) │
├─────────────────────────────────────────────────────┤
│ Layer 2: Catalog                                    │
│ Skill names, descriptions, and trigger hints        │
│ (injected into conversation context via hook,       │
│  NOT in the tool description)                       │
├─────────────────────────────────────────────────────┤
│ Layer 3: Invocation                                 │
│ Dumb Skill tool — just name + args                  │
│ (returns "Launching skill: X"; full content injected│
│  via PostToolUseHook as AdditionalContext)           │
└─────────────────────────────────────────────────────┘
```

The `skill/` package provides the loader, providers, and tool. `skill.ConfigureAgent` provides first-class agent integration.

```text
skill/                        # Core types, parsing, variable expansion
├── skill.go                  # Skill type, SkillConfig, IsLocal, IsCommand
├── parser.go                 # SKILL.md parsing (promoted from experimental)
├── expand.go                 # Variable expansion ($ARGUMENTS, $1, !{cmd})
├── loader.go                 # Multi-provider loader with priority ordering
├── provider.go               # Provider interface + filesystem provider
├── tool.go                   # Skill tool (trigger mechanism)
├── catalog.go                # Catalog builder, hash, rules
└── agent.go                  # ConfigureAgent, catalog hook, content hook

context.go                    # Core dive package: tool call ID context propagation
system_reminder.go            # Core dive package: named <system-reminder> blocks
```

### How `ConfigureAgent` Works (Implemented)

```go
opts := dive.AgentOptions{
    Model: anthropic.New(),
    Tools: tools,
}
skill.ConfigureAgent(&opts, loader)
agent, _ := dive.NewAgent(opts)
```

`ConfigureAgent` internally:

1. **Always registers hooks** — even with zero skills. This ensures stale catalog blocks from a previous session can be cleaned up on resume.
2. **Registers the Skill tool** (only when skills are loaded) — static description, no skill listing. The tool is a trigger: returns `"Launching skill: X"` and stores expanded instructions keyed by tool call ID for the PostToolUse hook.
3. **Appends skill usage rules** to the system prompt (only when skills are loaded).
4. **Registers a PreGenerationHook** that injects the skill catalog as a `<system-reminder name="skills">` block into the first user message via `dive.SetSystemReminder`. Replaced in place if catalog changes; removed if skills become empty. Handles fresh-process session resume.
5. **Registers a PostToolUseHook** that reads `hctx.Call.ID` to retrieve the correct expanded instructions from the per-call-ID map and sets them as `hctx.AdditionalContext`. Safe under parallel tool execution.

### The Skill Tool (Trigger)

When called, the tool:
- Looks up the skill by name
- Performs variable expansion (`$ARGUMENTS`, `$1`, `!{command}` — shell only for local skills)
- Stores expanded instructions keyed by `dive.ToolCallID(ctx)` for the PostToolUse hook
- Returns brief `"Launching skill: X"` — the PostToolUseHook delivers the full content

### The Skill Catalog (Context Injection)

Built by `skill.BuildCatalog(loader)` and injected into the first user message via `dive.SetSystemReminder`:

```xml
<system-reminder name="skills">
<skills>
The following skills are available for use with the Skill tool:

- code-reviewer: Review code for best practices and potential issues.
  TRIGGER when: user mentions "review"
- deploy: Deploy the current project to an environment.

When a task matches a skill's description or trigger, invoke the Skill
tool with the skill name before proceeding.
</skills>
</system-reminder>
```

Injected into the first user message for prompt caching stability. `dive.SetSystemReminder` is a general-purpose core API for managing named blocks — any system can use it, not just skills.

## Unification: Skills and Slash Commands

The `experimental/slashcmd/` package is **deprecated and merged into `skill/`**. The unified model:

| Attribute | Skill (agent-invocable) | Command (user-invocable) |
|-----------|------------------------|--------------------------|
| Has `description` in frontmatter | Yes | Optional |
| Has `user-invocable: false` | Optional | No |
| Agent can auto-invoke | Yes | No |
| User can invoke via `/name` | Yes | Yes |
| Supports variable expansion | Yes | Yes |
| File format | SKILL.md or .md with frontmatter | .md (with or without frontmatter) |

## Edge Cases & Constraints

### Shell Expansion Security

`!{command}` executes arbitrary shell commands. Rules:
- **Disabled by default** in the Go API. Must opt in with `WithShellExpansion(true)`.
- **Enabled by default** in the CLI for skills loaded from the local filesystem.
- **Always disabled** for non-local skills (any `SourceURI` that doesn't start with `file://`). Enforced by `Skill.IsLocal()` in `tool.Call()` regardless of global config.
- Commands run with the user's shell and environment. No sandboxing beyond the opt-in gate.
- A 10-second timeout applies to each `!{...}` expansion.

### Parallel Tool Execution Safety

The Skill tool supports parallel execution. When the agent issues multiple Skill tool calls in one response:
- Each `tool.Call()` reads `dive.ToolCallID(ctx)` and stores expanded instructions keyed by that ID.
- The PostToolUse hook reads `hctx.Call.ID` to retrieve the correct instructions.
- This ensures correct association even when results arrive out of completion order.

### Name Collisions

When a skill and a command have the same name:
- The skill wins (skills paths are searched before command paths). The command is shadowed.
- A warning is logged during loading.

### Backward Compatibility

- `experimental/skill/` continues to work but is marked deprecated with doc comments pointing to `skill/`.
- `experimental/slashcmd/` continues to work but is marked deprecated.
- `experimental/toolkit/extended.SkillTool` continues to work but is marked deprecated.
- The new `skill/` package has the same import path structure as other promoted packages (`session/`, `permission/`).

### Provider Error Handling

- Filesystem: missing directories are silently ignored. Malformed files are logged as warnings.
- All providers: duplicate names across providers follow priority order (first provider wins).

## Scope Boundaries

### In Scope

- Promote skill and slash command packages to `skill/`
- Full frontmatter: name, description, allowed-tools (metadata), model, argument-hint, triggers, user-invocable
- Variable expansion: `$ARGUMENTS`, `$1`-`$9`, `!{command}`
- Provider interface with filesystem implementation
- Thread-safe Loader
- **`skill.ConfigureAgent` first-class agent integration** (tool, catalog hook, content hook, rules — all wired internally)
- **Three-layer model**: rules in system prompt, catalog in conversation context, dumb tool for invocation
- **Skill catalog injection via PreGenerationHook** (matches Claude Code's `<system-reminder>` pattern)
- CLI: `/name args` and agent auto-invocation
- Unify skills and slash commands
- Comprehensive tests and updated documentation

### Out of Scope

- Tool restriction enforcement from `allowed-tools` (parsed as metadata only)
- HTTP/network skill providers (security risk; use custom `Provider` implementation)
- Skill packaging/distribution format
- Skill versioning or dependency management
- Skill composition (one skill invoking another)
- Interactive skill creation wizard in the CLI

## Implementation Sequence

### Phase 1: Core Package (DONE)

Promoted `experimental/skill/` to `skill/` with:
- Enhanced `Skill` and `SkillConfig` types (all frontmatter fields)
- Variable expansion (`$ARGUMENTS`, `$1`-`$9`, `!{command}`)
- Thread-safe Loader with Provider support
- Provider interface + FilesystemProvider
- `.agents/skills/` generic path support
- Merge `experimental/slashcmd/` into unified loading
- Skill Tool
- Deprecation markers on experimental packages
- CLI updated to use new package

### Phase 2: Three-Layer Agent Integration (DONE)

1. **Skill tool as trigger** — static description, returns "Launching skill: X", stores expanded instructions keyed by tool call ID for PostToolUseHook
2. **`skill/catalog.go`** — `BuildCatalog`, `CatalogHash`, `SkillRules`
3. **`skill/agent.go`** — `ConfigureAgent` wires tool, PreGenerationHook (catalog), PostToolUseHook (skill content)
4. **`system_reminder.go`** — `dive.SetSystemReminder/RemoveSystemReminder/HasSystemReminder` for named blocks in first user message
5. **`context.go`** — `dive.WithToolCallID/ToolCallID` for tool call ID propagation via context
6. **Catalog injection** — `<system-reminder name="skills">` block in first user message, replaced in place on change, removed on empty, handles session resume
7. **Skill content injection** — PostToolUseHook sets `AdditionalContext` with expanded instructions + base directory, keyed by call ID
8. **CLI updated** to use `skill.ConfigureAgent`

### Phase 3: Documentation (DONE)

- `docs/guides/skills.md` rewritten for three-layer architecture
- CLAUDE.md updated
- PRD and plan aligned with implementation

## Open Questions

1. ~~**Should the catalog be injected on every generation or only the first?**~~ **Resolved.** Injected into the first user message via `dive.SetSystemReminder`. Replaced in place if the catalog hash changes. Removed when skills become empty. Handles session resume by checking messages directly.

2. **Should command expansion timeout be configurable?** The 10-second default for `!{command}` may be too short for some use cases (e.g., `!{go test ./...}`). Recommendation: make it configurable via `ExpandOptions` with a sensible default. Currently configurable via `WithShellTimeout`.

3. **Should the `model` frontmatter field be used by the agent?** Skills can declare a model override, but the agent currently has a single model. Supporting per-skill model switching would require `ConfigureAgent` to have access to model creation. Defer for now.
