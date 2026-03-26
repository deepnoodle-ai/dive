# Implementation Plan: Production Skills System

**PRD:** [prd-02-skills.md](../prds/prd-02-skills.md)
**Last Updated:** 2026-03-27

## Overview

Promote the experimental skill and slash command packages into a unified, production-quality `skill/` top-level package with first-class agent integration via `skill.ConfigureAgent`.

## Status: COMPLETE

All three phases are implemented and tested.

## Phase 1: Core Package (DONE)

Promoted `experimental/skill/` to `skill/` with:
- Enhanced `Skill` and `SkillConfig` types (all frontmatter fields)
- Variable expansion (`$ARGUMENTS`, `$1`-`$9`, `!{command}`)
- Thread-safe Loader with Provider support
- Provider interface + FilesystemProvider + HTTPProvider
- `.agents/skills/` generic path support (Codex/agentskills.io standard)
- Merge `experimental/slashcmd/` into unified loading
- Skill Tool and Toolset
- Deprecation markers on experimental packages
- CLI updated to use new package

## Phase 2: Three-Layer Agent Integration (DONE)

Aligned with Claude Code's architecture based on direct investigation:

### Three Layers

1. **Rules** — skill usage instructions appended to system prompt by `ConfigureAgent`
2. **Catalog** — skill names/descriptions/triggers injected as `<system-reminder name="skills">` in the **first user message** via `dive.SetSystemReminder`. Stable position for prompt caching.
3. **Tool** — trigger mechanism. Returns `"Launching skill: X"`. Full content delivered via PostToolUseHook as `AdditionalContext` on the tool result message.

### What Was Built

| File | What |
|------|------|
| `skill/catalog.go` | `BuildCatalog`, `CatalogHash`, `SkillRules` |
| `skill/agent.go` | `ConfigureAgent`, `catalogHook` (PreGeneration), `skillContentHook` (PostToolUse) |
| `system_reminder.go` | `dive.SetSystemReminder`, `RemoveSystemReminder`, `HasSystemReminder` — general-purpose named block management in first user message |
| `system_reminder_test.go` | 12 tests for system-reminder CRUD |
| `skill/catalog_test.go` | 5 tests for catalog building and hashing |
| `skill/agent_test.go` | 10 tests for ConfigureAgent and both hooks |

### Key Design Decisions

- **`skill.ConfigureAgent(&opts, loader)`** avoids circular import (skill→dive, not dive→skill). Follows the same one-call pattern as `AgentOptions.Session`.
- **First user message** for catalog (not last) — stable position for prompt caching, doesn't move as conversation grows.
- **`dive.SetSystemReminder`** is a general-purpose API — any system can manage named blocks, not just skills. Idempotent: insert or replace by name.
- **Tool as trigger** — matches Claude Code where the tool returns a brief acknowledgment and the content appears separately. Uses `pendingInstructions` on the Loader for tool→hook communication.
- **Base directory** included in skill content so the agent can resolve relative paths to reference files within the skill directory.
- **Re-invocation guard** — returns "already active" if the agent tries to invoke the same skill twice.
- **Session resume** — hook detects existing `<system-reminder name="skills">` block and skips re-injection.

## Phase 3: Documentation (DONE)

- `docs/guides/skills.md` — rewritten for three-layer architecture, ConfigureAgent usage, catalog format
- `CLAUDE.md` — updated with skill/ package description
- PRD and plan aligned with implementation
- Experimental guides have deprecation notices
