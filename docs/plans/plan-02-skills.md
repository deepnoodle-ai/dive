# Implementation Plan: Production Skills System

**PRD:** [prd-02-skills.md](../prds/prd-02-skills.md)
**Last Updated:** 2026-03-26

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
2. **Catalog** — skill names/descriptions/triggers injected as `<system-reminder name="skills">` in the **first user message** via `dive.SetSystemReminder`. Stable position for prompt caching.
3. **Tool** — trigger mechanism. Returns `"Launching skill: X"`. Full content delivered via PostToolUseHook as `AdditionalContext` on the tool result message, keyed by tool call ID for parallel safety.

### What Was Built

| File | What |
|------|------|
| `skill/catalog.go` | `BuildCatalog`, `CatalogHash`, `SkillRules` |
| `skill/agent.go` | `ConfigureAgent`, `catalogHook` (PreGeneration), `skillContentHook` (PostToolUse) |
| `skill/skill.go` | `Skill`, `SkillConfig`, `IsLocal()`, `IsCommand()` |
| `system_reminder.go` | `dive.SetSystemReminder`, `RemoveSystemReminder`, `HasSystemReminder` — general-purpose named block management in first user message |
| `context.go` | `dive.WithToolCallID`, `dive.ToolCallID` — context key for tool call ID propagation |

### Key Design Decisions

- **`skill.ConfigureAgent(&opts, loader)`** avoids circular import (skill→dive, not dive→skill). Follows the same one-call pattern as `AgentOptions.Session`.
- **Hooks always installed** — even with zero skills, hooks are registered so that stale catalog blocks from a previous session can be cleaned up on resume.
- **First user message** for catalog (not last) — stable position for prompt caching, doesn't move as conversation grows.
- **`dive.SetSystemReminder`** is a general-purpose API — any system can manage named blocks, not just skills. Idempotent: insert or replace by name.
- **Per-call-ID content association** — `tool.Call()` reads `dive.ToolCallID(ctx)` to key pending instructions; the PostToolUse hook reads `hctx.Call.ID` to retrieve the correct content. This is safe under parallel tool execution where results arrive out of order.
- **No tool restrictions** — `allowed-tools` is parsed as metadata but not enforced at runtime. Simplifies the implementation and avoids the complexity of skill activation/deactivation lifecycle.
- **No HTTP provider** — removed due to security implications (remote code could exploit shell expansion). The `Provider` interface remains extensible for custom backends.
- **Shell expansion gated by `IsLocal()`** — only skills with `file://` or empty `SourceURI` can have `!{command}` expanded, regardless of global config.
- **Session resume** — catalog hook detects existing `<system-reminder name="skills">` blocks in messages directly (not just via in-memory hash state), handles fresh-process resume correctly.
- **Base directory** included in skill content so the agent can resolve relative paths to reference files within the skill directory.
- **File path in catalog** — each catalog entry includes `Location:` so the agent can answer "where is this skill?" without guessing.
- **Symlink resolution** — filesystem provider resolves symlinked skill directories, so `~/.claude/skills/` entries that are symlinks are discovered correctly.
- **Shared `PathValidator`** — toolkit tools accept an optional `Validator` field (takes precedence over `WorkspaceDir`). `PathValidator.AllowReadPath(dir)` grants read access to skill reference directories outside the workspace. `Loader.BaseDirs()` returns unique skill base directories for this purpose.

## Phase 3: Documentation (DONE)

- `docs/guides/skills.md` — rewritten for three-layer architecture, ConfigureAgent usage, catalog format
- `CLAUDE.md` — updated with skill/ package description
- PRD and plan aligned with implementation
- Experimental guides have deprecation notices
