# Implementation Plan: Production Skills System

**PRD:** [prd-02-skills.md](../prds/prd-02-skills.md)
**Last Updated:** 2026-07-10

## Overview

Promote the experimental skill and slash command packages into a unified, production-quality `skill/` top-level package with first-class agent integration via `skill.ConfigureAgent`.

## Status: COMPLETE

All three phases are implemented and tested.

## Phase 1: Core Package (DONE)

Promoted `experimental/skill/` to `skill/` with:

- Enhanced `Skill` and `SkillConfig` types (all frontmatter fields)
- Variable expansion (`$ARGUMENTS`, `$1`-`$9`, `!{command}`)
- Thread-safe Loader with Provider support
- Provider interface + FilesystemProvider
- `.agents/skills/` generic path support (Codex/agentskills.io standard)
- Merge `experimental/slashcmd/` into unified loading
- Skill Tool
- Deprecation markers on experimental packages
- CLI updated to use new package

## Phase 2: Three-Layer Agent Integration (DONE)

Aligned with Claude Code's architecture based on direct investigation:

### Three Layers

1. **Rules** — skill usage instructions appended to system prompt by `ConfigureAgent`
2. **Catalog** — skill names/descriptions/triggers created as a typed contextual reminder and appended at the request tail via `hctx.AppendReminder(..., dive.ModelOnly)`. A later same-name block supersedes stale history without rewriting it.
3. **Tool** — trigger mechanism. Returns `"Launching skill: X"`. Full content delivered via PostToolUseHook as `AdditionalContext` on the tool result message, keyed by tool call ID for parallel safety.

### What Was Built

| File                             | What                                                                                                                       |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `skill/catalog.go`               | `BuildCatalog`, `CatalogHash`, `SkillRules`                                                                                |
| `skill/agent.go`                 | `ConfigureAgent`, `catalogHook` (PreGeneration), `skillContentHook` (PostToolUse)                                          |
| `skill/skill.go`                 | `Skill`, `SkillConfig`, `IsLocal()`, `IsCommand()`                                                                         |
| `reminder.go`, `llm/reminder.go` | Typed reminders, contextual/operator tiers, recorded/model-only lifetimes, inspection helpers, and provider-edge rendering |
| `system_reminder.go`             | Legacy plain-text block helpers retained for session migration and compatibility                                           |
| `context.go`                     | `dive.WithToolCallID`, `dive.ToolCallID` — context key for tool call ID propagation                                        |

### Key Design Decisions

- **`skill.ConfigureAgent(&opts, loader)`** avoids circular import (skill→dive, not dive→skill). Follows the same one-call pattern as `AgentOptions.Session`.
- **Hooks always installed** — even with zero skills, hooks are registered so that stale catalog blocks from a previous session can be cleaned up on resume.
- **Request tail** for catalog — appending does not rewrite the first user message as the conversation grows, so the long session prefix remains cacheable.
- **Typed model-only reminder** — the catalog hook uses `NewContextReminder` plus `hctx.AppendReminder(..., dive.ModelOnly)`. A later same-name block supersedes older values, does not mutate loaded messages, and is not persisted.
- **Per-call-ID content association** — `tool.Call()` reads `dive.ToolCallID(ctx)` to key pending instructions; the PostToolUse hook reads `hctx.Call.ID` to retrieve the correct content. This is safe under parallel tool execution where results arrive out of order.
- **No tool restrictions** — `allowed-tools` is parsed as metadata but not enforced at runtime. Simplifies the implementation and avoids the complexity of skill activation/deactivation lifecycle.
- **No HTTP provider** — removed due to security implications (remote code could exploit shell expansion). The `Provider` interface remains extensible for custom backends.
- **Shell expansion gated by `IsLocal()`** — only skills with `file://` or empty `SourceURI` can have `!{command}` expanded, regardless of global config.
- **Session resume** — catalog hook detects legacy `<system-reminder name="skills">` blocks and appends a same-name typed reminder so stale text is superseded without rewriting history.
- **Base directory** included in skill content so the agent can resolve relative paths to reference files within the skill directory.
- **File path in catalog** — each catalog entry includes `Location:` so the agent can answer "where is this skill?" without guessing.
- **Symlink resolution** — filesystem provider resolves symlinked skill directories, so `~/.claude/skills/` entries that are symlinks are discovered correctly.
- **Shared `PathValidator`** — toolkit tools accept an optional `Validator` field (takes precedence over `WorkspaceDir`). `PathValidator.AllowReadPath(dir)` grants read access to skill reference directories outside the workspace. `Loader.BaseDirs()` returns unique skill base directories for this purpose.

## Phase 3: Documentation (DONE)

- `docs/guides/skills.md` — rewritten for three-layer architecture, ConfigureAgent usage, catalog format
- `CLAUDE.md` — updated with skill/ package description
- PRD and plan aligned with implementation
- Experimental guides have deprecation notices
