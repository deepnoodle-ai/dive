# Core Refinement Plan

## Goal

Establish a clear boundary between core Dive (stable, minimal, extensible) and experimental/optional features. The core should be small enough to understand quickly, stable enough to depend on, and extensible enough to support diverse use cases.

## Current State Analysis

### Top-Level Files (29 .go files)

| File | Lines | Category | Notes |
|------|-------|----------|-------|
| `dive.go` | 200 | **Core** | Agent interface, Response types |
| `agent.go` | 1000 | **Core** | StandardAgent implementation |
| `tool.go` | 300 | **Core** | Tool interface, ToolResult |
| `schema.go` | 20 | **Core** | Schema type alias |
| `response.go` | 230 | **Core** | Response, ResponseItem types |
| `hooks.go` | 370 | **Core** | Generation hooks infrastructure |
| `utilities.go` | 25 | **Core** | Helper functions |
| `session.go` | 300 | Experimental | Session, SessionRepository |
| `file_session_repository.go` | 200 | Experimental | File-based session storage |
| `compaction.go` | 300 | Experimental | Context compaction |
| `permission.go` | 320 | Experimental | Permission evaluation |
| `permission_config.go` | 700 | Experimental | Permission configuration |
| `permission_rules.go` | 700 | Experimental | Rule matching |
| `interactor.go` | 530 | Experimental | User interaction |
| `confirmer.go` | 120 | Experimental | Confirmation flow |
| `subagent.go` | 160 | Experimental | Subagent definitions |
| `subagent_loader.go` | 200 | Experimental | Subagent loading |
| `settings.go` | 300 | Experimental | Settings loading |
| `todo_tracker.go` | 120 | Experimental | Todo tracking |
| `system_prompt.go` | 40 | Experimental | Default system prompt |

### Packages

| Package | Category | Notes |
|---------|----------|-------|
| `llm/` | **Core** | LLM interface, messages, tools |
| `providers/` | **Core** | LLM provider implementations |
| `toolkit/` | Optional | Built-in tool implementations |
| `sandbox/` | Experimental | Docker/Seatbelt sandboxing |
| `mcp/` | Experimental | Model Context Protocol |
| `skill/` | Experimental | Skill loading system |
| `slashcmd/` | Experimental | Slash command loading |
| `experimental/` | Experimental | New hook-based implementations |

## Proposed Core

The core package should contain only what's needed to create and run an agent:

```
dive/
├── dive.go           # Agent interface
├── agent.go          # StandardAgent (simplified)
├── tool.go           # Tool interface, ToolResult
├── schema.go         # Schema type
├── response.go       # Response types
├── hooks.go          # Generation hooks
├── utilities.go      # Helpers
└── llm/              # LLM abstractions
    └── providers/    # Provider implementations
```

**Core API surface:**
- `Agent` interface
- `NewAgent(AgentOptions)`
- `Tool` interface
- `PreGenerationHook`, `PostGenerationHook`
- `PreToolUseHook`, `PostToolUseHook`
- LLM interface and providers

## Migration Plan

### Phase 1: Move to Experimental

Move these to `experimental/` with deprecation notices on the originals:

1. **Session Management** → `experimental/session/` ✅ Done
   - `session.go` → Keep minimal types, deprecate repository
   - `file_session_repository.go` → Move entirely

2. **Permission System** → `experimental/permission/` ✅ Done
   - `permission.go`, `permission_config.go`, `permission_rules.go`
   - Keep only `PreToolUseHook`/`PostToolUseHook` types in core

3. **Interactor** → `experimental/interactor/` ✅ Done
   - `interactor.go`, `confirmer.go`

4. **Subagents** → `experimental/subagent/` ✅ Done
   - `subagent.go`, `subagent_loader.go`

5. **Compaction** → `experimental/compaction/` ✅ Done
   - `compaction.go`
   - `CompactionHookWithModel` moved to experimental/compaction/hooks.go

6. **Settings** → `experimental/settings/` ✅ Done
   - `settings.go`
   - Settings are CLI/application concern, not core

7. **Todo Tracker** → `experimental/todo/` ✅ Done
   - `todo_tracker.go`

8. **System Prompt** → Remove or move to examples
   - `system_prompt.go`
   - Callers should provide their own
   - Note: template utilities still used by agent.go

### Phase 2: Move Packages

1. **Sandbox** → `experimental/sandbox/` ✅ Done
   - Complex, specialized feature
   - Not needed for basic agent usage

2. **MCP** → `experimental/mcp/` ✅ Done
   - Protocol integration, not core
   - Can be used via tools

3. **Skill** → `experimental/skill/` ✅ Done
   - File-based skill loading
   - Application concern

4. **Slash Commands** → `experimental/slashcmd/` ✅ Done
   - CLI-specific feature

5. **Toolkit** → Split between core and experimental
   - Core tools stay in `toolkit/` (Read, Write, Edit, Bash, Glob, Grep, etc.)
   - External service integrations move to `experimental/toolkit/`:
     - `firecrawl/` - Web scraping service ✅ Moved
     - `google/` - Google Search API ✅ Moved
     - `kagi/` - Kagi Search API ✅ Moved

### Phase 3: Simplify Agent

1. **Remove embedded dependencies from AgentOptions:**
   ```go
   type AgentOptions struct {
       // Required
       SystemPrompt string
       Model        llm.LLM

       // Optional
       Tools          []Tool
       PreGeneration  []PreGenerationHook
       PostGeneration []PostGenerationHook
       PreToolUse     []PreToolUseHook
       PostToolUse    []PostToolUseHook

       // Infrastructure
       Logger        llm.Logger
       ModelSettings *ModelSettings
   }
   ```

2. **Remove deprecated fields** (after deprecation period):
   - `SessionRepository`
   - `Permission`
   - `Interactor`
   - `Subagents`, `SubagentLoader`
   - `Name`, `Goal`, `Instructions` (use SystemPrompt)
   - `IsSupervisor`, `Subordinates` (obsolete)

3. **Simplify StandardAgent struct** to match

### Phase 4: Documentation

1. Update CLAUDE.md with new structure
2. Create migration guide for experimental packages
3. Update examples to use new patterns
4. Document hook composition patterns

## File Counts (Target)

**Core (target: ~10 files, ~2000 lines):**
- `dive.go` - Agent interface
- `agent.go` - StandardAgent (simplified)
- `tool.go` - Tool interface
- `schema.go` - Schema type
- `response.go` - Response types
- `hooks.go` - Hook infrastructure
- `utilities.go` - Helpers

**Experimental (everything else):**
- `experimental/session/`
- `experimental/permission/`
- `experimental/interactor/`
- `experimental/subagent/`
- `experimental/compaction/`
- `experimental/settings/`
- `experimental/todo/`
- `experimental/sandbox/`
- `experimental/mcp/`
- `experimental/skill/`
- `experimental/slashcmd/`

## Success Criteria

1. **Core is small**: Under 2500 lines of Go code
2. **Core is stable**: No breaking changes for 6 months
3. **Core is testable**: 90%+ test coverage
4. **Core is documented**: Every public type/function has godoc
5. **Experimental is useful**: All moved features remain functional
6. **Migration is smooth**: Deprecation warnings guide users

## Open Questions

1. Should `providers/` be a separate module?
2. Should `toolkit/` be a separate module?
3. How long should deprecation period be? (Suggest: 2 major versions)
4. Should experimental packages graduate to stable? (Criteria needed)

## Next Steps

1. [x] Move `compaction.go` to `experimental/compaction/`
2. [x] Move `settings.go` to `experimental/settings/`
3. [x] Move `todo_tracker.go` to `experimental/todo/`
4. [x] Move `sandbox/` to `experimental/sandbox/`
5. [x] Move `mcp/` to `experimental/mcp/`
6. [x] Move `skill/` to `experimental/skill/`
7. [x] Move `slashcmd/` to `experimental/slashcmd/`
8. [ ] Remove deprecated fields from AgentOptions
9. [ ] Update all documentation
10. [ ] Create migration guide
