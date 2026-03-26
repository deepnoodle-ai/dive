# PRD: Production Skills System

| Field | Content |
|-------|---------|
| Title | Production Skills & Slash Commands |
| Author | Curtis / DeepNoodle |
| Status | Draft |
| Last Updated | 2026-03-27 |
| Stakeholders | Dive library users, CLI users, agent builders |

## Problem & Opportunity

### The Problem

AI coding agents are converging on **skills** as the primary extensibility mechanism — lazily-loaded, markdown-based instruction sets that agents invoke contextually. Claude Code, Codex CLI, and Gemini CLI all have skill systems, and community consensus is clear: skills are the highest-value investment for customizing agent behavior.

Dive has experimental skill support (`experimental/skill/`) that handles basic loading and parsing. But it's missing the features that make skills powerful in practice:

1. **No variable substitution.** `$ARGUMENTS`, `$1`, `{{args}}` — all absent. Skills can't be parameterized.
2. **No command substitution.** Gemini CLI's `!{git diff --staged}` pattern lets commands inject live context. Dive has nothing equivalent.
3. **No allowed-tools enforcement.** The `allowed-tools` frontmatter field is parsed in tests but never wired up. Skills can't constrain what tools an agent uses.
4. **No network discovery.** Skills are local-only. No way to fetch skills from a URL, registry, or Git repository.
5. **No slash command unification.** Skills and slash commands are separate packages with duplicated code. The industry has converged: a slash command is just a skill without frontmatter (no auto-invocation).
6. **No CLI integration.** The Dive CLI has zero skill support — no listing, no invocation, no management.
7. **Not thread-safe.** The `Loader` explicitly documents it is NOT safe for concurrent use.
8. **No skill composition.** Skills can't reference or invoke other skills.

The research is unambiguous:

> "Skills are the killer feature because they are a very composable tool... A skill is basically a plain-text description of an application." — mergesort, Hacker News

> "Skills are flaky, as it's up to Claude to figure out if they should be used. Commands are much more reliable." — u/rair41, r/ClaudeCode

> "9/10 times I have had to explicitly add 'use skill' in the prompt." — u/mo_rawr16, r/ClaudeCode

The reliability problem with auto-invocation is the #1 pain point across all three tools. Dive has an opportunity to do this better.

### Why Now

1. **Industry convergence.** Claude Code merged skills and slash commands. Codex adopted SKILL.md. The format is standardizing and Dive should support it natively.
2. **Dive's experimental code is solid.** The parser, loader, and tool integration are well-tested. This is a promotion and enhancement, not a rewrite.
3. **Community demand for cross-tool skills.** Users want skills that work across Claude Code, Codex, and Dive without modification. Dive should be a first-class citizen in that ecosystem.
4. **The Go API use case is unique.** No other skill system provides a Go library for programmatic skill management. Dive can offer features like network-based skill providers, runtime skill registration, and dynamic toolsets — things CLI-only tools can't.

### What Happens If We Do Nothing

Dive's skill support stays experimental and incomplete. Developers building agents with Dive will either ignore skills entirely or build their own skill loading on top of the raw experimental package, creating fragmentation. The CLI stays less capable than Claude Code and Codex for everyday use.

## Goals & Success Metrics

| Goal | Metric |
|------|--------|
| **Primary:** Go developers can load, discover, and invoke skills with a production-quality API | Skills load from filesystem, URLs, and custom providers with one unified interface |
| **Primary:** Full feature parity with Claude Code / Codex skill semantics | Variable substitution, command substitution, allowed-tools, auto-invocation triggers all working |
| **Secondary:** The CLI provides a complete skill experience | `dive skills list`, `/skill-name`, and agent auto-invocation all work in the CLI |
| **Secondary:** Cross-tool compatibility | Claude Code SKILL.md files work in Dive without modification |
| **Guardrail:** No breaking changes to the core `dive` package | Existing agent code compiles and runs unchanged |
| **Guardrail:** The API is simple for the common case | Loading filesystem skills requires <5 lines of Go |

## Target Users

### Primary: Go developers building AI agents (Library API)

Developers using Dive's Go API to build agents that need customizable, domain-specific behaviors. They want to load skills from the filesystem, from a URL, or from a custom backend. They want skills to constrain tool access, support variables, and compose with each other. They may be building platforms where end users define skills.

### Secondary: CLI power users (Dive CLI)

Developers using the Dive CLI interactively who want `/review-code` to just work, want to see available skills, and want the agent to auto-invoke skills when appropriate. They expect parity with Claude Code's skill UX.

### Tertiary: Teams sharing skills across tools

Teams that maintain skill libraries in Git repositories and use them across Claude Code, Codex, and Dive. They need format compatibility and predictable behavior.

## Jobs To Be Done

### JTBD-1: "Load my existing Claude Code skills into a Dive agent"

A developer has `.claude/skills/` with 10 skills they use daily in Claude Code. They're building an agent with Dive and want those same skills available.

**What would blow them away:** Zero-config. `skill.NewLoader()` finds and loads them automatically. One field on the agent wires everything up — the tool, toolset, catalog injection, and usage rules.

```go
loader := skill.NewLoader(skill.LoaderOptions{ProjectDir: "."})
loader.Load(ctx)

opts := dive.AgentOptions{
    Model: anthropic.New(),
    Tools: tools,
}
skill.ConfigureAgent(&opts, loader)

agent, _ := dive.NewAgent(opts)
```

### JTBD-2: "Fetch skills from our team's skill registry at build time or runtime"

A platform team maintains shared skills in a Git repository or HTTP endpoint. Individual developers shouldn't need to clone the repo — the agent should pull skills at startup.

**What would blow them away:** Pluggable providers. Filesystem is the default. HTTP, Git, or a custom backend are one interface implementation away.

```go
loader := skill.NewLoader(skill.LoaderOptions{
    Providers: []skill.Provider{
        skill.NewFilesystemProvider(".claude/skills"),
        skill.NewHTTPProvider("https://skills.internal.co/api/v1"),
    },
})
loader.Load(ctx)
```

### JTBD-3: "Invoke a skill with arguments and have them substituted into the instructions"

A user types `/deploy staging` and expects the skill instructions to contain "staging" where `$ARGUMENTS` or `$1` appeared in the template. Or a skill uses `!{git branch --show-current}` to inject the current branch name.

**What would blow them away:** Full substitution support — positional args, `$ARGUMENTS`, and `!{command}` shell expansion — matching the superset of what Claude Code, Codex, and Gemini CLI support.

```markdown
---
name: deploy
description: Deploy the current project to an environment.
argument-hint: <environment>
---

Deploy to $1 environment.

Current branch: !{git branch --show-current}
Recent changes: !{git log --oneline -5}
```

### JTBD-4: "Skills should constrain what tools the agent can use"

A "read-only reviewer" skill should only have access to Read, Grep, and Glob — not Bash or WriteFile. The skill author specifies this in frontmatter and the agent enforces it.

**What would blow them away:** `allowed-tools` in frontmatter dynamically filters the agent's toolset while the skill is active.

```markdown
---
name: code-reviewer
description: Review code for best practices.
allowed-tools: [Read, Grep, Glob]
---
```

### JTBD-5: "I want the agent to pick the right skill automatically, but reliably"

Auto-invocation is the dream, but reliability is terrible in practice (reported 9/10 failure rate). The developer wants the agent to use skills when appropriate without being told, but with better matching than "hope the LLM reads the description."

**What would blow them away:** A three-layer system modeled after Claude Code. The LLM sees the skill catalog in conversation context (not the tool definition — that's a token waste). Skills can also declare explicit `triggers` — keyword patterns or regex that the system matches against the user's input before the LLM even sees it. When a trigger matches, the skill is suggested with high confidence.

```markdown
---
name: code-reviewer
description: Review code for best practices and potential issues.
triggers:
  - "review"
  - "code review"
  - pattern: "review .+"
---
```

### JTBD-6: "Use slash commands and skills interchangeably in the CLI"

A developer has both `.dive/commands/commit.md` and `.dive/skills/code-reviewer/SKILL.md`. They want `/commit` and `/code-reviewer` to both work in the CLI. They don't want to think about which is which.

**What would blow them away:** Unified invocation. Skills with no frontmatter (or `user-invocable: true`) are slash commands. `/name args` works for both. The agent can auto-invoke skills; humans invoke both via `/`.

## Proposed Solution

### Architecture Overview

The skill system has three distinct layers, modeled after how Claude Code implements skills internally. This separation is critical — conflating them (e.g., stuffing the skill catalog into the tool description) wastes tokens and reduces reliability.

```text
┌─────────────────────────────────────────────────────┐
│ Layer 1: Rules                                      │
│ System prompt instructions for how to use skills    │
│ (injected by agent when Skills loader is set)       │
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

The `skill/` package provides the loader, providers, tool, and toolset. `skill.ConfigureAgent` provides first-class agent integration.

```text
skill/                        # Core types, parsing, variable expansion
├── skill.go                  # Skill type, SkillConfig, expansion logic
├── parser.go                 # SKILL.md parsing (promoted from experimental)
├── expand.go                 # Variable expansion ($ARGUMENTS, $1, !{cmd})
├── loader.go                 # Multi-provider loader with priority ordering
├── provider.go               # Provider interface + filesystem provider
├── provider_http.go          # HTTP provider (fetch skills from URLs)
├── tool.go                   # Skill tool (trigger mechanism)
├── toolset.go                # Dynamic toolset for allowed-tools filtering
├── catalog.go                # Catalog builder, hash, rules
└── agent.go                  # ConfigureAgent, catalog hook, content hook

system_reminder.go            # Core dive package: named <system-reminder> blocks
```

### Layer 1: Core Types and Parsing

*(Implemented — see `skill/skill.go`, `skill/parser.go`, `skill/expand.go`)*

The `Skill` type, `SkillConfig` with all frontmatter fields, unified parser (SKILL.md + COMMAND.md + plain .md), and variable expansion (`$ARGUMENTS`, `$1`-`$9`, `!{command}`) are already built and tested.

### Layer 2: Provider System

*(Implemented — see `skill/provider.go`, `skill/provider_http.go`, `skill/loader.go`)*

The `Provider` interface, `FilesystemProvider`, `HTTPProvider`, and thread-safe `Loader` are already built and tested. Discovery paths include `.dive/`, `.agents/` (generic standard), and `.claude/` directories.

### Layer 3: Agent Integration — Three-Layer Model

This is the key architectural change from the initial implementation. Claude Code separates skill awareness into three layers, and Dive should do the same.

#### How Claude Code Does It

Based on direct investigation of Claude Code's internals:

1. **Rules** — the system prompt contains general instructions: "When a task matches a skill's description, invoke the Skill tool before proceeding," "Don't guess skill names," etc.
2. **Catalog** — a `<system-reminder>` block is injected inline into user message text (not the system prompt, not the tool description) listing all available skills with names, descriptions, and trigger conditions.
3. **Tool** — the Skill tool itself is dumb. It accepts `skill` (name) and `args` (optional). Its schema contains no skill list. The LLM already knows what's available from the catalog.

Key insight: the catalog appears once in the conversation context. It is NOT repeated in every tool definition on every LLM request. This is dramatically more token-efficient.

#### How Dive Does It (Implemented)

**`skill.ConfigureAgent`** wires up all three layers in one call (avoids circular import between `dive` and `skill/`):

```go
opts := dive.AgentOptions{
    Model: anthropic.New(),
    Tools: tools,
}
skill.ConfigureAgent(&opts, loader)
agent, _ := dive.NewAgent(opts)
```

`ConfigureAgent` internally:

1. **Registers the Skill tool** — static description, no skill listing. The tool is a trigger: returns `"Launching skill: X"` and stores expanded instructions for the hook.
2. **Wraps tools with the skill Toolset** for `allowed-tools` filtering.
3. **Appends skill usage rules** to the system prompt.
4. **Registers a PreGenerationHook** that injects the skill catalog as a `<system-reminder name="skills">` block into the **first user message** via `dive.SetSystemReminder`. This is stable for prompt caching — it sits right after the system prompt and doesn't move. Re-injects in place if the catalog changes.
5. **Registers a PostToolUseHook** that, when the Skill tool fires, takes the expanded instructions from `loader.pendingInstructions` and sets them as `hctx.AdditionalContext`. The agent loop appends this as a text block on the tool result message — so the skill content appears adjacent to the tool result, matching Claude Code's pattern.

#### The Skill Tool (Trigger)

The tool's `Description()` is static:

```text
Execute a skill by name to receive specialized instructions for a task.
Use this tool when the available skills list includes one matching your current task.
```

When called, the tool:
- Looks up the skill by name
- Guards against re-invocation (returns "already active" if same skill)
- Performs variable expansion (`$ARGUMENTS`, `$1`, `!{command}`)
- Sets it as the active skill on the Loader (for `allowed-tools` filtering)
- Stores expanded instructions (with base directory for relative path resolution) as `pendingInstructions`
- Returns brief `"Launching skill: X"` — the PostToolUseHook delivers the full content

#### The Skill Catalog (Context Injection)

The catalog is built by `skill.BuildCatalog(loader)` and injected into the first user message via `dive.SetSystemReminder`:

```xml
<system-reminder name="skills">
<skills>
The following skills are available for use with the Skill tool:

- code-reviewer: Review code for best practices and potential issues.
  TRIGGER when: user mentions "review"
- deploy: Deploy the current project to an environment.
  TRIGGER when: user mentions "deploy"
- research: Conduct structured research on a topic.

When a task matches a skill's description or trigger, invoke the Skill
tool with the skill name before proceeding.
</skills>
</system-reminder>
```

Injected into the first user message for prompt caching stability. Replaced in place (by name) if the catalog changes after `loader.Load()`. `dive.SetSystemReminder` is a general-purpose core API for managing named blocks — any system can use it, not just skills.

#### The Skill Toolset (allowed-tools)

*(Implemented — see `skill/toolset.go`)*

`NewToolset(loader, tools)` returns a `dive.Toolset` that filters tools based on the active skill's `AllowedTools` config.

### Layer 4: CLI Integration

#### Skill Listing

```text
$ dive skills
Available skills:
  code-reviewer    Review code for best practices and potential issues
  deploy           Deploy the current project to an environment
  research         Conduct structured research on a topic

Available commands:
  /commit          Create a git commit with a good message
  /pr              Create a pull request
```

#### Slash Command Invocation

```text
> /code-reviewer src/main.go
```

The CLI parses `/name args`, looks up the skill/command, expands variables (including `!{...}` for local skills), and injects the expanded instructions into the conversation.

#### Agent Auto-Invocation

The agent sees the skill catalog in conversation context and can invoke the Skill tool autonomously. The CLI also runs trigger matching on user input and suggests relevant skills:

```text
> Review the authentication code in auth.go
[Skill suggestion: code-reviewer — activate? Y/n]
```

This hybrid approach (triggers + LLM catalog awareness) addresses the auto-invocation reliability problem.

## Unification: Skills and Slash Commands

The `experimental/slashcmd/` package is **deprecated and merged into `skill/`**. The unified model:

| Attribute | Skill (agent-invocable) | Command (user-invocable) |
|-----------|------------------------|--------------------------|
| Has `description` in frontmatter | Yes | Optional |
| Has `user-invocable: false` | Optional | No |
| Agent can auto-invoke | Yes | No |
| User can invoke via `/name` | Yes | Yes |
| Supports variable expansion | Yes | Yes |
| Supports `allowed-tools` | Yes | Yes |
| File format | SKILL.md or .md with frontmatter | .md (with or without frontmatter) |

A skill without a description (or with empty frontmatter / no frontmatter) is a command. The agent never sees it in its tool description. But the user can always invoke it with `/name`.

Commands are loaded from `.dive/commands/` and `.claude/commands/` in addition to the skills paths. The Loader handles both.

## Edge Cases & Constraints

### Shell Expansion Security

`!{command}` executes arbitrary shell commands. Rules:
- **Disabled by default** in the Go API. Must opt in with `WithShellExpansion(true)`.
- **Enabled by default** in the CLI for skills loaded from the local filesystem.
- **Always disabled** for skills loaded from HTTP providers. No exceptions.
- Commands run with the user's shell and environment. No sandboxing beyond the opt-in gate.
- Expansion errors (command fails, timeout) produce an error result, not silent empty strings.
- A 10-second timeout applies to each `!{...}` expansion.

### Name Collisions

When a skill and a command have the same name:
- The skill wins (skills paths are searched before command paths). The command is shadowed.
- A warning is logged during loading.
- `/name` invokes the skill. The shadowed command is not accessible. Authors should avoid name collisions by using distinct names.

### Large Skill Sets

Because the skill catalog is injected into conversation context (not the tool description), it doesn't hit the tool description length limits that plagued Claude Code (reported 77-skill limit). However, very large catalogs still consume input tokens. The catalog builder should:
- Include all agent-invocable skill names and descriptions.
- If the combined catalog exceeds a configurable limit, truncate descriptions to one-line summaries.
- The `Count()` method lets users check before loading.

### Backward Compatibility

- `experimental/skill/` continues to work but is marked deprecated with doc comments pointing to `skill/`.
- `experimental/slashcmd/` continues to work but is marked deprecated.
- `experimental/toolkit/extended.SkillTool` continues to work but is marked deprecated.
- The new `skill/` package has the same import path structure as other promoted packages (`session/`, `permission/`).

### Provider Error Handling

- Filesystem: missing directories are silently ignored (existing behavior). Malformed files are logged as warnings.
- HTTP: network errors are logged as warnings. The loader continues with skills from other providers. Retries are the caller's responsibility.
- All providers: duplicate names across providers follow priority order (first provider wins).

## Scope Boundaries

### In Scope

- Promote skill and slash command packages to `skill/`
- Full frontmatter: name, description, allowed-tools, model, argument-hint, triggers, user-invocable
- Variable expansion: `$ARGUMENTS`, `$1`-`$9`, `!{command}`
- Provider interface with filesystem and HTTP implementations
- Thread-safe Loader
- **`skill.ConfigureAgent` first-class agent integration** (tool, toolset, catalog hook, content hook, rules — all wired internally)
- **Three-layer model**: rules in system prompt, catalog in conversation context, dumb tool for invocation
- **Skill catalog injection via PreGenerationHook** (matches Claude Code's `<system-reminder>` pattern)
- CLI: `dive skills`, `/name args`, trigger-based suggestions
- Unify skills and slash commands
- Comprehensive tests and updated documentation

### Out of Scope

- Skill packaging/distribution format (future work)
- Git provider (future work, trivial with the Provider interface)
- Skill versioning or dependency management
- Skill marketplace or registry service
- Skill composition (one skill invoking another) — future work, complex UX
- Interactive skill creation wizard in the CLI
- Skill caching/persistence across sessions

## Implementation Sequence

### Phase 1: Core Package (DONE)

Promoted `experimental/skill/` to `skill/` with:
- Enhanced `Skill` and `SkillConfig` types (all frontmatter fields)
- Variable expansion (`$ARGUMENTS`, `$1`-`$9`, `!{command}`)
- Thread-safe Loader with Provider support
- Provider interface + FilesystemProvider + HTTPProvider
- `.agents/skills/` generic path support
- Merge `experimental/slashcmd/` into unified loading
- Skill Tool and Toolset
- Comprehensive tests (51 passing)
- Deprecation markers on experimental packages
- CLI updated to use new package

### Phase 2: Three-Layer Agent Integration (DONE)

1. **Skill tool as trigger** — static description, returns "Launching skill: X", stores expanded instructions for PostToolUseHook
2. **`skill/catalog.go`** — `BuildCatalog`, `CatalogHash`, `SkillRules`
3. **`skill/agent.go`** — `ConfigureAgent` wires tool, toolset, PreGenerationHook (catalog), PostToolUseHook (skill content)
4. **`system_reminder.go`** — `dive.SetSystemReminder/RemoveSystemReminder/HasSystemReminder` for named blocks in first user message
5. **Catalog injection** — `<system-reminder name="skills">` block in first user message, replaced in place on change, handles session resume
6. **Skill content injection** — PostToolUseHook sets `AdditionalContext` with expanded instructions + base directory
7. **Re-invocation guard** — returns "already active" if same skill invoked twice
8. **CLI updated** to use `skill.ConfigureAgent`

### Phase 3: Documentation (DONE)

- `docs/guides/skills.md` rewritten for three-layer architecture
- CLAUDE.md updated
- PRD and plan aligned with implementation

## Open Questions

1. ~~**Should the catalog be injected on every generation or only the first?**~~ **Resolved.** Injected into the first user message via `dive.SetSystemReminder`. Replaced in place if the catalog hash changes. Handles session resume by detecting existing blocks.

2. **Should the HTTP provider manifest format be standardized?** There's an emerging `agentskills.io` spec. We could adopt it or define our own. Recommendation: start with a simple JSON manifest and add spec compliance later if the standard gains traction.

3. **Should command expansion timeout be configurable?** The 10-second default for `!{command}` may be too short for some use cases (e.g., `!{go test ./...}`). Recommendation: make it configurable via `ExpandOptions` with a sensible default.

4. **Should the `model` frontmatter field be used by the agent?** Skills can declare a model override, but the agent currently has a single model. Supporting per-skill model switching would require `ConfigureAgent` to have access to model creation. Defer for now.
