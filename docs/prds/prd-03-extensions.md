# PRD: Agent Extensions

| Field | Content |
|-------|---------|
| Title | Agent Extension Interface |
| Author | Curtis / DeepNoodle |
| Status | Draft |
| Last Updated | 2026-03-27 |
| Stakeholders | Dive library users, agent builders |

## Problem & Opportunity

### The Problem

Dive's agent capabilities are extended through packages like `skill/`, `permission/`, and (eventually) `experimental/mcp/`. Each package that wants to add tools, hooks, or system prompt rules to an agent must either:

1. **Mutate `AgentOptions` externally** — The current pattern requires callers to use package-level functions like `skill.ConfigureAgent(&opts, loader)` that reach into `AgentOptions` and append tools, hooks, and system prompt text. This is awkward: the caller must remember to make the call, the function takes a pointer to opts, and the wiring logic lives outside the agent.

2. **Require manual hook/tool assembly** — Without a helper, the caller must know the internal wiring details: which hooks to register, which tools to add, what system prompt rules to append. This is error-prone and couples callers to implementation details.

Neither approach scales. Each new capability package reinvents the same pattern. There's no standard way to bundle "tools + hooks + rules" as a unit and hand it to the agent.

### Why Now

1. **Skills just shipped.** The `skill.ConfigureAgent` pattern works but feels bolted-on. Before more packages adopt this pattern, we should establish the right abstraction.
2. **MCP and permission have similar needs.** MCP servers provide tools and may need hooks. Permission provides hooks and rules. A unified extension interface prevents each from inventing its own wiring.
3. **Library ergonomics matter.** Dive's API should feel cohesive. Having `Session` as a first-class field while skills require a separate `ConfigureAgent` call is inconsistent.

## Goals & Success Metrics

| Goal | Metric |
|------|--------|
| **Primary:** Agent capabilities can be extended through a single, composable interface | One `Extension` interface that bundles tools, hooks, and rules |
| **Primary:** Skills integrate via Extension instead of `ConfigureAgent` | `skill.Loader` satisfies `dive.Extension`; `ConfigureAgent` is deprecated |
| **Secondary:** The pattern is reusable for future capability packages | MCP, permission, or any new package can implement `Extension` without new agent code |
| **Guardrail:** No breaking changes to existing public API | `ConfigureAgent` continues to work (deprecated); all existing agent code compiles unchanged |
| **Guardrail:** No hook ordering sensitivity | Extensions are merged in declaration order with no required ordering between them |

## Users & Use Cases

### Primary User: Go developers building agents with Dive

**UC1: Adding skills to an agent**

Today:
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

With extensions:
```go
loader := skill.NewLoader(skill.LoaderOptions{ProjectDir: "."})
loader.Load(ctx)

agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        anthropic.New(),
    Tools:        tools,
    Extensions:   []dive.Extension{loader},
})
```

**UC2: Composing multiple extensions**

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Model:      anthropic.New(),
    Extensions: []dive.Extension{skillLoader, mcpServer, permissionManager},
})
```

**UC3: Building a custom extension**

A developer creates an extension that adds an audit logging hook and a custom tool:

```go
type AuditExtension struct { /* ... */ }

func (a *AuditExtension) Tools() []dive.Tool    { return []dive.Tool{a.auditTool} }
func (a *AuditExtension) Hooks() dive.Hooks      { return dive.Hooks{PostToolUse: []dive.PostToolUseHook{a.logHook}} }
func (a *AuditExtension) Rules() string           { return "Log all tool usage for compliance." }
```

## Proposed Solution

### The Extension Interface

Add to the `dive` package:

```go
// Extension bundles tools, hooks, and system prompt rules that extend
// an agent's capabilities. Implementations provide any combination of
// these — returning nil/empty for aspects they don't use.
type Extension interface {
    // Tools returns additional tools to make available to the agent.
    Tools() []Tool

    // Hooks returns hooks to register on the agent.
    Hooks() Hooks

    // Rules returns text to append to the agent's system prompt.
    // Returns empty string if no rules are needed.
    Rules() string
}
```

### AgentOptions Change

```go
type AgentOptions struct {
    // ...existing fields...

    // Extensions provide additional tools, hooks, and system prompt rules.
    // Extensions are merged in order: tools are appended, hooks are appended
    // to their respective slices, and rules are appended to the system prompt.
    // Extension tools and hooks come after those set directly on AgentOptions.
    Extensions []Extension
}
```

### NewAgent Merge Logic

During `NewAgent`, after processing direct fields, iterate extensions:

```go
for _, ext := range opts.Extensions {
    if ext == nil {
        continue
    }
    // Merge tools
    tools = append(tools, ext.Tools()...)

    // Merge hooks
    hooks.PreGeneration = append(hooks.PreGeneration, ext.Hooks().PreGeneration...)
    hooks.PostGeneration = append(hooks.PostGeneration, ext.Hooks().PostGeneration...)
    hooks.PreToolUse = append(hooks.PreToolUse, ext.Hooks().PreToolUse...)
    hooks.PostToolUse = append(hooks.PostToolUse, ext.Hooks().PostToolUse...)
    hooks.PostToolUseFailure = append(hooks.PostToolUseFailure, ext.Hooks().PostToolUseFailure...)
    hooks.Stop = append(hooks.Stop, ext.Hooks().Stop...)
    hooks.PreIteration = append(hooks.PreIteration, ext.Hooks().PreIteration...)

    // Merge rules
    if rules := ext.Rules(); rules != "" {
        systemPrompt = strings.TrimRight(systemPrompt, "\n") + "\n\n" + rules
    }
}
```

### skill.Loader as Extension

The `skill.Loader` gains three methods to satisfy `dive.Extension`:

- `Tools() []dive.Tool` — Returns the Skill tool (or empty if no skills loaded)
- `Hooks() dive.Hooks` — Returns the catalog and content hooks
- `Rules() string` — Returns `SkillRules()` (or empty if no skills loaded)

`ConfigureAgent` is deprecated but continues to work for backward compatibility.

## Scope & Non-Goals

### In Scope

- `Extension` interface in the `dive` package
- `Extensions` field on `AgentOptions`
- Merge logic in `NewAgent`
- `skill.Loader` satisfying `Extension`
- Deprecation of `skill.ConfigureAgent`
- Tests for extension merging and skill loader as extension

### Out of Scope

- Migrating `permission` or `mcp` to Extension (future work, validates the pattern)
- Extension lifecycle (init/shutdown hooks) — not needed yet
- Extension ordering guarantees — current extensions are order-independent
- Named extensions or extension discovery/registry

## Edge Cases & Decisions

| Case | Decision |
|------|----------|
| Nil extension in the slice | Skip silently |
| Extension returns nil from Tools() or empty Hooks() | Safe — append of nil slice is a no-op |
| Multiple extensions add same-named tool | Last one wins (same as adding duplicate tools directly) |
| Extension Rules() with leading/trailing whitespace | Trimmed during merge |
| Zero extensions | No-op, identical to current behavior |
| skill.Loader with no skills loaded | Tools() returns empty, Rules() returns empty, hooks still returned (for session resume catalog cleanup) |

## Implementation Plan

### Phase 1: Extension interface and merge logic
1. Define `Extension` interface in `dive` package
2. Add `Extensions` field to `AgentOptions`
3. Implement merge logic in `NewAgent`
4. Add tests for extension merging

### Phase 2: Skill loader as extension
1. Add `Tools()`, `Hooks()`, `Rules()` methods to `skill.Loader`
2. The Loader needs `ConfigOption` support (e.g. shell expansion) — add a `SetConfigOptions` method or handle via `LoaderOptions`
3. Deprecate `skill.ConfigureAgent` (keep working, add deprecation comment)
4. Update tests
5. Update docs and CLAUDE.md

### Open Questions

1. **ConfigOptions for skills:** `ConfigureAgent` accepts `ConfigOption` (e.g. `WithConfigShellExpansion`). When skills are wired via Extension, where do these options live? Options: (a) on `LoaderOptions` at construction time, (b) a `SetConfigOptions` method on Loader, (c) the Loader's `Tools()` method accepts options. Recommendation: (a) move to `LoaderOptions` since they're known at construction time.
