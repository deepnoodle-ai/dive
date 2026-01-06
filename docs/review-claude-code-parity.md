# Dive Codebase Review: Claude Code Parity Analysis

This document provides a comprehensive analysis of gaps, bugs, and areas needing clarification in the Dive codebase, with a focus on achieving feature parity with Claude Code.

**Review Date:** January 2025
**Reviewed Areas:** Compaction, Subagents, Skills, Permissions, Todo Lists

---

## Table of Contents

1. [Compaction](#1-compaction)
2. [Subagents](#2-subagents)
3. [Skills](#3-skills)
4. [Permissions](#4-permissions)
5. [Todo Lists](#5-todo-lists)
6. [Additional Gaps vs Claude Code](#6-additional-gaps-vs-claude-code)
7. [Code Quality Issues](#7-code-quality-issues)
8. [Recommendations](#8-recommendations)

---

## 1. Compaction

### Current Implementation

Dive implements client-side context compaction that automatically summarizes conversation history when token thresholds are exceeded.

**Core Files:**

- `compaction.go` - Types and constants
- `agent.go:1059-1135` - Compaction logic in `performCompaction()`
- `thread.go:54-56` - CompactionHistory storage
- `llm/content.go:1091-1116` - SummaryContent type

### What Works Well

| Feature                   | Location              | Description                                                    |
| ------------------------- | --------------------- | -------------------------------------------------------------- |
| Token threshold           | `compaction.go:10`    | Default 100k tokens (`DefaultContextTokenThreshold`)           |
| Structured summary prompt | `compaction.go:14-43` | `DefaultCompactionSummaryPrompt` based on Anthropic's SDK spec |
| Per-request override      | `dive.go:187-206`     | `WithCompaction()` option for per-request config               |
| History tracking          | `compaction.go:82-95` | `CompactionRecord` stores timestamp, token counts              |
| Event emission            | `response.go:38-40`   | `ResponseItemTypeCompaction` for monitoring                    |

### Gaps

#### Gap 1.1: No Persistence of Compaction History Across Sessions

**Problem:** `CompactionHistory` is stored in `Thread`, but the CLI doesn't use a persistent `ThreadRepository`.

**Location:** `thread.go:54-56`

```go
// CompactionHistory tracks when context compaction has occurred
CompactionHistory []CompactionRecord `json:"compaction_history,omitempty"`
```

**Impact:** Compaction history is lost when the process exits. Users cannot see historical compaction events.

**Claude Code Behavior:** Persists compaction metadata for session resumption.

---

#### Gap 1.2: No Incremental/Rolling Compaction

**Problem:** Dive performs full replacement - all prior messages become a single summary.

**Location:** `agent.go:1115-1125`

```go
// Replace messages with summary
summaryMessage := &llm.Message{
    Role: llm.User,
    Content: []llm.Content{
        &llm.SummaryContent{Summary: summary},
    },
}
thread.Messages = []*llm.Message{summaryMessage}
```

**Impact:** Recent context that might be relevant is lost in the summary.

**Claude Code Behavior:** Uses rolling compaction that preserves the N most recent messages while summarizing older ones.

---

#### Gap 1.3: Token Counting Accuracy Across Providers

**Problem:** Token calculation may not accurately reflect context window usage for non-Anthropic providers.

**Location:** `agent.go:1037-1040`

```go
func (a *StandardAgent) calculateTotalTokens(usage *llm.Usage) int {
    return usage.InputTokens + usage.OutputTokens +
           usage.CacheCreationInputTokens + usage.CacheReadInputTokens
}
```

**Impact:** Different providers have different tokenization. Using Anthropic-specific token counts for other providers may trigger compaction too early or too late.

**Recommendation:** Add provider-specific token estimation or allow configurable token counting strategy.

---

### Bugs

#### Bug 1.1: Potential Race Condition in `performCompaction`

**Problem:** `thread.Messages` is modified while the generation loop might still reference it.

**Location:** `agent.go:1115-1125`

**Risk:** If another goroutine reads `thread.Messages` during modification, undefined behavior may occur.

**Fix:** Use mutex protection or copy-on-write pattern:

```go
// Safe approach
newMessages := []*llm.Message{summaryMessage}
a.mu.Lock()
thread.Messages = newMessages
a.mu.Unlock()
```

---

## 2. Subagents

### Current Implementation

Dive supports spawning specialized subagents via the Task tool, with tool filtering and background execution.

**Core Files:**

- `subagent.go` - SubagentDefinition, SubagentRegistry, FilterTools
- `subagent_loader.go` - FileSubagentLoader for loading from disk
- `toolkit/task.go` - TaskTool, TaskOutputTool, TaskRegistry

### What Works Well

| Feature                 | Location                   | Description                                       |
| ----------------------- | -------------------------- | ------------------------------------------------- |
| Definition structure    | `subagent.go:16-33`        | Description, Prompt, Tools, Model fields          |
| Thread-safe registry    | `subagent.go:44-133`       | `SubagentRegistry` with RWMutex                   |
| Tool filtering          | `subagent.go:135-167`      | `FilterTools()` excludes Task tool from subagents |
| Background execution    | `toolkit/task.go:297-301`  | `RunInBackground` parameter                       |
| File-based loading      | `subagent_loader.go:22-84` | Loads from `.dive/agents/` and `.claude/agents/`  |
| General-purpose default | `subagent.go:37-42`        | `GeneralPurposeSubagent` always available         |

### Gaps

#### Gap 2.1: No Context Sharing with Parent Agent

**Problem:** Subagents start fresh with only the prompt provided. No way to pass parent conversation context.

**Location:** `toolkit/task.go:281-284`

```go
message := &llm.Message{Role: llm.User}
message.Content = append(message.Content, &llm.TextContent{Text: input.Prompt})

response, err := agent.CreateResponse(ctx, dive.WithMessage(message))
```

**Impact:** Parent must include all relevant context in the prompt, leading to verbose prompts and potential information loss.

**Claude Code Behavior:** Has `access_to_current_context` flag that allows subagents to see parent's full conversation history:

```
"Agents with 'access to current context' can see the full conversation history
before the tool call. When using these agents, you can write concise prompts
that reference earlier context."
```

**Recommendation:** Add `InheritContext bool` field to `SubagentDefinition` and `TaskToolInput`.

---

#### Gap 2.2: Resume is Process-Scoped, Not Persistent

**Problem:** `TaskRegistry` is in-memory only. Resume fails after process restart.

**Location:** `toolkit/task.go:38-49`

```go
type TaskRegistry struct {
    mu    sync.RWMutex
    tasks map[string]*TaskRecord
}
```

**Impact:** Cannot resume long-running tasks after crash or intentional restart.

**Claude Code Behavior:** Task state persists, allowing `/tasks` command to show history and resume.

**Recommendation:** Integrate with `ThreadRepository` or add dedicated task persistence.

---

#### Gap 2.3: No Subagent Output Streaming to Parent

**Problem:** Parent waits for full completion or timeout with no incremental updates.

**Location:** `toolkit/task.go:313-326`

```go
select {
case <-done:
    // Full result only available here
case <-timeoutCtx.Done():
    // Timeout with no partial result
}
```

**Impact:** For long-running tasks, no visibility into progress.

**Recommendation:** Add event streaming from subagent to parent via callback chain.

---

#### Gap 2.4: Missing Subagent Types from Claude Code

**Problem:** Dive lacks several specialized subagent types that Claude Code provides.

**Claude Code Subagents Not in Dive:**

| Subagent            | Purpose                                                                                 | Tools                                 |
| ------------------- | --------------------------------------------------------------------------------------- | ------------------------------------- |
| `Explore`           | Fast codebase exploration with thoroughness levels (`quick`, `medium`, `very thorough`) | Glob, Grep, Read                      |
| `Plan`              | Software architect for implementation planning                                          | All tools                             |
| `statusline-setup`  | Configure status line settings                                                          | Read, Edit                            |
| `claude-code-guide` | Documentation lookup for Claude Code/Agent SDK                                          | Glob, Grep, Read, WebFetch, WebSearch |

**Location for adding:** `.dive/agents/` or programmatically via `SubagentRegistry.Register()`

---

#### Gap 2.5: Model Override Mapping Unclear

**Problem:** Model field accepts strings like `"sonnet"`, `"opus"`, `"haiku"` but mapping to actual model IDs is unclear.

**Location:** `subagent.go:30-32`

```go
// Model overrides the LLM model for this subagent.
// Valid values: "sonnet", "opus", "haiku", or "" to inherit from parent.
Model string
```

**Questions:**

- How does `"sonnet"` map to actual model ID for non-Anthropic providers?
- What happens if the string doesn't match a known model?
- No validation or error handling visible.

**Recommendation:** Add explicit model mapping or validation in `AgentFactory`.

---

## 3. Skills

### Current Implementation

Skills provide focused expertise via markdown files with YAML frontmatter, loaded from multiple locations.

**Core Files:**

- `skill/skill.go` - Skill struct and SkillConfig
- `skill/loader.go` - Multi-location discovery and loading
- `skill/parser.go` - YAML frontmatter parsing
- `toolkit/skill.go` - SkillTool for runtime activation

### What Works Well

| Feature                  | Location                   | Description                                 |
| ------------------------ | -------------------------- | ------------------------------------------- |
| YAML frontmatter         | `skill/parser.go`          | Clean separation of config and instructions |
| Priority-based discovery | `skill/loader.go:67-94`    | Project → user level ordering               |
| Tool restrictions        | `skill/skill.go:31-46`     | `AllowedTools` field with enforcement       |
| ToolAllowanceChecker     | `toolkit/skill.go:257-270` | Interface for agent integration             |
| Thread-safe state        | `toolkit/skill.go:74-76`   | RWMutex protects activeSkill                |

### Gaps

#### Gap 3.1: No Slash Command Invocation from User Input

**Problem:** CLI doesn't intercept `/skill-name` patterns to invoke skills.

**Location:** `cmd/dive/app.go:1040-1061`

```go
func (a *App) handleCommand(input string) bool {
    switch input {
    case "/quit", "/exit", "/q":
        // ...
    case "/clear":
        // ...
    case "/todos", "/t":
        // ...
    case "/help", "/?":
        // ...
    }
    return false
}
```

**Impact:** Users cannot type `/commit` to invoke a commit skill - only the LLM can invoke skills via the Skill tool.

**Claude Code Behavior:** Supports `/<skill-name>` syntax directly from user input:

```
"/<skill-name> (e.g., /commit) is shorthand for users to invoke a user-invocable skill."
```

**Recommendation:** Add skill lookup in `handleCommand()`:

```go
if strings.HasPrefix(input, "/") {
    skillName := strings.TrimPrefix(input, "/")
    if skill, ok := a.skillLoader.GetSkill(skillName); ok {
        // Inject skill invocation into conversation
    }
}
```

---

#### Gap 3.2: Tool Restrictions Don't Stack Across Skills

**Problem:** While skill instructions ARE additive (each invocation adds instructions to conversation context), tool restrictions only apply from the most recently activated skill.

**Location:** `toolkit/skill.go:191-194`

```go
// Set as active skill (singular, not a list)
t.mu.Lock()
t.activeSkill = s  // Replaces previous, doesn't append
t.mu.Unlock()
```

**Behavior:**
- `/code-reviewer` → instructions added to context ✓
- `/security-focus` → instructions ALSO added to context ✓
- But only `security-focus`'s `allowed-tools` restrictions are enforced

**Impact:** If both skills have `allowed-tools` defined, only the last skill's restrictions apply. Earlier skills' restrictions are lost.

**Recommendation:** Stack tool restrictions when multiple skills with `allowed-tools` are active:
```go
type SkillTool struct {
    activeSkills []*skill.Skill  // Stack instead of single
}

func (t *SkillTool) IsToolAllowed(toolName string) bool {
    // Intersect allowed tools from all active skills
}
```

---

#### Gap 3.3: No Skill Completion/Deactivation Mechanism

**Problem:** No tool to signal skill work is complete. `ClearActiveSkill()` exists but isn't exposed.

**Location:** `toolkit/skill.go:239-243`

```go
func (t *SkillTool) ClearActiveSkill() {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.activeSkill = nil
}
```

**Impact:** Tool restrictions persist indefinitely until another skill is activated.

**Recommendation:** Add `deactivate_skill` tool or auto-deactivate after task completion.

---

#### Gap 3.4: Skill vs Subagent Distinction Unclear

**Problem:** Both skills and subagents modify agent behavior - documentation doesn't clarify when to use each.

**Conceptual Difference:**

- **Skills:** Modify current agent's instructions (same context, same tools minus restrictions)
- **Subagents:** Spawn new agent instance (fresh context, separate execution)

**Recommendation:** Add comparison section in documentation.

---

## 4. Permissions

### Current Implementation

Comprehensive permission system with modes, rules, hooks, and callbacks aligned with Anthropic's Claude Agent SDK.

**Core Files:**

- `permission.go` - Core types (PermissionMode, ToolHookAction, hooks)
- `permission_config.go` - PermissionConfig, PermissionManager
- `permission_rules.go` - Declarative rule system
- `confirmer.go` - User confirmation implementations

### What Works Well

| Feature               | Location                | Description                                   |
| --------------------- | ----------------------- | --------------------------------------------- |
| Four permission modes | `permission.go:44-73`   | Default, Plan, AcceptEdits, BypassPermissions |
| Declarative rules     | `permission_rules.go`   | DenyRule, AllowRule, AskRule with patterns    |
| Pre/Post hooks        | `permission.go:144-172` | PreToolUseHook, PostToolUseHook               |
| CanUseTool callback   | `permission.go:174-188` | Runtime permission decisions                  |
| Tool annotations      | `tool.go`               | ReadOnlyHint, DestructiveHint, EditHint       |

### Gaps

#### Gap 4.1: No "Allow for Session" Option

**Problem:** No visible implementation of session-scoped permission allowlists.

**Location:** `cmd/dive/app.go:122-125`

```go
type ConfirmResult struct {
    Approved     bool
    AllowSession bool   // Field exists but usage unclear
    Feedback     string
}
```

**Impact:** Users must approve each similar operation individually.

**Claude Code Behavior:** "Allow all X operations this session" option reduces friction.

**Recommendation:** Implement session-scoped allowlist in PermissionManager:

```go
type PermissionManager struct {
    sessionAllowed map[string]bool // tool patterns allowed for session
    // ...
}
```

---

#### Gap 4.2: Limited Command-Level Patterns for Bash

**Problem:** Rule matching for Bash commands is limited.

**Location:** `permission_rules.go:58-75`

```go
type PermissionRule struct {
    Tool         string                      // Tool name pattern
    Command      string                      // Command pattern (Bash only)
    InputMatch   func(input any) bool        // Custom matcher
    // ...
}
```

**Current Support:**

- `AllowCommandRule("bash", "ls *", "")` - Basic glob matching
- No regex support
- No argument extraction

**Claude Code Behavior:** More sophisticated command parsing with specific patterns:

```
"Bash(go build:*), Bash(./dive:*), Bash(git -C /Users/curtis/git/dive show e150e85)"
```

**Recommendation:** Add regex support and argument position matching.

---

#### Gap 4.3: No Sandbox/Isolation

**Problem:** No filesystem sandbox, network isolation, or resource limits.

**References in Docs:** `dangerouslyDisableSandbox` mentioned but no actual sandbox implementation.

**Location:** No sandbox implementation found in codebase.

**Impact:** Agents can access entire filesystem, network, and system resources.

**Claude Code Behavior:** Has sandboxing with explicit bypass flag.

**Recommendation:** Implement sandbox options:

- Filesystem: chroot or path allowlist
- Network: proxy or allowlist
- Resources: timeout, memory limits

---

#### Gap 4.4: Permission Check Ordering Clarity

**Problem:** Documentation says deny→allow→ask→mode, but actual ordering may differ.

**Location:** `permission_config.go` (evaluation order)

**Documentation:** `permission.go:24`

```go
// Permission Flow:
//
//  PreToolUse Hook → Deny Rules → Allow Rules → Ask Rules → Mode Check → CanUseTool → Execute → PostToolUse Hook
```

**Concern:** If user expects mode to take precedence, rules might override unexpectedly.

**Recommendation:** Add explicit documentation examples showing precedence.

---

### Bugs

#### Bug 4.1: Hook Error Handling Unclear

**Problem:** What happens when PreToolUse hook returns an error?

**Location:** `agent.go` (hook evaluation)

**Questions:**

- Is error returned to LLM as tool error?
- Does it stop execution entirely?
- Is it logged and ignored?

**Recommendation:** Document error handling behavior explicitly.

---

## 5. Todo Lists

### Current Implementation

Task tracking with three states and real-time UI updates via event emission.

**Core Files:**

- `toolkit/todo.go` - TodoWriteTool implementation
- `response.go:74-91` - TodoItem, TodoStatus types
- `todo_tracker.go` - Helper for consuming events
- `cmd/dive/app.go` - CLI rendering and state

### What Works Well

| Feature            | Location                     | Description                                    |
| ------------------ | ---------------------------- | ---------------------------------------------- |
| Three states       | `response.go:75-80`          | pending, in_progress, completed                |
| Dual forms         | `response.go:83-88`          | content (imperative) + activeForm (continuous) |
| Event emission     | `agent.go:873-886`           | `ResponseItemTypeTodo` for real-time updates   |
| TodoTracker helper | `todo_tracker.go`            | Thread-safe consumption of events              |
| CLI visualization  | `cmd/dive/render.go:405-458` | Animated progress indicators                   |

### Gaps

#### Gap 5.1: Tool Naming Inconsistency

**Problem:** Tool is named differently in different places.

**Locations:**

- `toolkit/todo.go:62`: `func (t *TodoWriteTool) Name() string { return "todo_write" }`
- `cmd/dive/app.go:885`: `if call.Name == "todo_write"`
- Documentation references: `TodoWrite` (PascalCase)

**Impact:** Case-sensitive matching may fail. Confusion in documentation.

**Recommendation:** Standardize on one convention (recommend `todo_write` for consistency with other tools).

---

#### Gap 5.2: No Persistence

**Problem:** Todos exist only in memory, lost on process restart.

**Location:** `toolkit/todo.go:54-57`

```go
type TodoWriteTool struct {
    mu       sync.RWMutex
    todos    []TodoItem
    onUpdate func([]TodoItem)
}
```

**Impact:** Cannot resume work on a todo list after restart.

**Claude Code Behavior:** Persists todo state for session resumption.

**Recommendation:** Integrate with ThreadRepository or dedicated persistence.

---

#### Gap 5.3: No Read-Only Todo Query

**Problem:** No way to query current todos without overwriting.

**Current Behavior:** Each `TodoWrite` call replaces the entire list.

**Impact:** LLM must always provide full list even to check current state.

**Recommendation:** Add `TodoRead` tool:

```go
func (t *TodoReadTool) Call(ctx context.Context, input *TodoReadInput) (*dive.ToolResult, error) {
    return dive.NewToolResultText(formatTodos(t.todos)), nil
}
```

---

#### Gap 5.4: No Todo Dependencies or Ordering

**Problem:** Flat list with no relationships between items.

**Location:** `response.go:83-88`

```go
type TodoItem struct {
    Content    string
    Status     TodoStatus
    ActiveForm string
    // No: ParentID, DependsOn, Priority, Order
}
```

**Impact:** Cannot express "blocked by" or priority relationships.

**Claude Code Behavior:** Simple flat list (same as Dive - not a gap vs Claude Code, but a potential enhancement).

---

### Bugs

#### Bug 5.1: CLI Parsing Error Handling

**Problem:** `parseTodoWriteInput()` has minimal error handling.

**Location:** `cmd/dive/app.go:1154-1185`

**Risk:** Malformed JSON could cause issues.

**Recommendation:** Add defensive parsing with error logging.

---

## 6. Additional Gaps vs Claude Code

### Missing Core Tools

#### Gap 6.1: EnterPlanMode / ExitPlanMode Tools

**Problem:** No tools for dynamic plan mode entry/exit.

**Current State:** `PermissionModePlan` exists but can only be set at agent creation or per-request.

**Claude Code Behavior:**

- `EnterPlanMode` tool transitions agent to planning
- `ExitPlanMode` signals plan completion and user approval
- Plan written to file for review

**Recommendation:** Implement as tools:

```go
type EnterPlanModeTool struct{}
type ExitPlanModeTool struct{}
```

---

#### Gap 6.2: AskUserQuestion Tool Schema Mismatch

**Problem:** Dive's `ask_user_question` tool has different schema than Claude Code's `AskUserQuestion`.

**Dive Schema:** `toolkit/ask_user.go:73-130`

```go
Properties: map[string]*schema.Property{
    "question": {...},
    "type": {...},      // "confirm", "select", "multiselect", "input"
    "options": {...},
    // ...
}
```

**Claude Code Schema:**

```go
Properties: {
    "questions": [...],  // Array of questions with headers, multiSelect
    "answers": {...},    // For collecting answers
}
```

**Impact:** Prompt compatibility issues when migrating from Claude Code.

**Recommendation:** Add Claude Code-compatible schema or adapter.

---

#### Gap 6.3: WebSearch / WebFetch Tools

**Problem:** No built-in web search or URL fetching tools.

**Current State:**

- `toolkit/web_search.go` exists but provider integration unclear
- No `WebFetch` equivalent for direct URL fetching

**Claude Code Behavior:**

- `WebSearch` - Search with domain filtering
- `WebFetch` - Fetch URL content with AI processing

**Recommendation:** Complete web tool implementations with provider configuration.

---

#### Gap 6.4: LSP Integration

**Problem:** No Language Server Protocol support.

**Claude Code LSP Operations:**

- `goToDefinition`
- `findReferences`
- `hover`
- `documentSymbol`
- `workspaceSymbol`
- `goToImplementation`
- `prepareCallHierarchy`
- `incomingCalls`
- `outgoingCalls`

**Impact:** Significant code navigation capability missing.

**Recommendation:** Add LSP client integration for Go (gopls) at minimum.

---

#### Gap 6.5: NotebookEdit Tool

**Problem:** No Jupyter notebook support.

**Claude Code Behavior:** Can edit notebook cells by ID with `edit_mode` (replace, insert, delete).

**Recommendation:** Add notebook parsing and editing support.

---

#### Gap 6.6: KillShell Tool

**Problem:** Cannot terminate background bash processes.

**Current State:** `TaskRegistry` tracks tasks but no cancellation mechanism.

**Location:** `toolkit/task.go:25-36` - No cancel channel or method.

**Recommendation:** Add cancellation:

```go
type TaskRecord struct {
    // ...
    cancel context.CancelFunc
}
```

---

### CLI Gaps

#### Gap 6.7: No `/tasks` Command

**Problem:** TaskOutputTool description mentions `/tasks` but it's not implemented.

**Location:** `toolkit/task.go:373`

```go
"- Task IDs can be found using the /tasks command"
```

**But:** `cmd/dive/app.go:1040-1061` doesn't include `/tasks`.

**Recommendation:** Add `/tasks` command:

```go
case "/tasks":
    a.listTasks()
    return true
```

---

#### Gap 6.8: No Context Management Commands

**Missing Commands:**

- `/compact` - Force context compaction
- `/fork` - Create conversation branch
- `/resume` - Resume from previous session
- `/thread` - Show/switch threads

**Recommendation:** Add context management commands to CLI.

---

#### Gap 6.9: No Configuration Persistence

**Problem:** Model, temperature, etc. set via flags only.

**Current:** All configuration via command-line flags.

**Recommendation:** Add configuration file support (`.diverc`, `dive.yaml`).

---

## 7. Code Quality Issues

### Naming Inconsistencies

#### Issue 7.1: Tool Naming Convention

**Problem:** Mixed naming conventions across tools.

| Convention | Examples                                            |
| ---------- | --------------------------------------------------- |
| snake_case | `read_file`, `write_file`, `ask_user_question`, `todo_write` |
| PascalCase | `TodoWrite` (in docs), `Skill`                      |
| lowercase  | `task`, `glob`, `grep`, `bash`                      |

**Locations:**

- `toolkit/read_file.go`: `"read_file"`
- `toolkit/skill.go`: `"skill"` (lowercase)
- `toolkit/todo.go`: `"todo_write"`

**Recommendation:** Standardize on `snake_case` for all tools:

- `read_file`, `write_file`, `edit_file`
- `todo_write`, `todo_read`
- `ask_user_question`
- `skill` (already lowercase, keep as is)

---

#### Issue 7.2: Response Type Complexity

**Problem:** `ToolResult` has multiple construction patterns.

**Patterns in Use:**

```go
// Pattern 1: Content array
result := &ToolResult{
    Content: []*ToolResultContent{{Text: "..."}},
}

// Pattern 2: Helper functions
result := dive.NewToolResultText("...")
result := dive.NewToolResultError("...")

// Pattern 3: With display
result := dive.NewToolResultText("...").WithDisplay("...")
```

**Recommendation:** Document preferred patterns and deprecate others.

---

### Documentation Gaps

#### Issue 7.3: No Migration Guide for Permission System

**Problem:** `interactor.go` deprecated for tools but no migration guide.

**Location:** `interactor.go` - Contains `PermissionConfigFromInteractionMode()` but not documented.

**Recommendation:** Add migration guide in docs:

```markdown
## Migrating from InteractionMode to PermissionConfig

Before:
agent.Interactor = &TerminalInteractor{Mode: InteractIfDestructive}

After:
agent.Permission = &PermissionConfig{
Mode: PermissionModeDefault,
Rules: PermissionRules{
dive.AskRule("\*"),
},
}
```

---

#### Issue 7.4: Subagent File Format Undocumented

**Problem:** YAML frontmatter format for `.dive/agents/` files not in main docs.

**Actual Format:** (from `subagent_loader.go`)

```yaml
---
description: Expert code reviewer
model: sonnet
tools:
  - Read
  - Grep
  - Glob
---
You are a code review specialist.
```

**Recommendation:** Add to documentation with examples.

---

## 8. Recommendations

### Priority 1: Critical for Claude Code Parity

| #   | Recommendation                                 | Effort | Impact                        |
| --- | ---------------------------------------------- | ------ | ----------------------------- |
| 1   | Add slash command skill invocation             | Medium | High - Expected UX            |
| 2   | Implement session-scoped permission allowlists | Medium | High - Reduces friction       |
| 3   | Add `/tasks` command (documented but missing)  | Low    | Medium - Consistency          |
| 4   | Fix tool naming consistency                    | Low    | Medium - Developer experience |

### Priority 2: Important Functionality

| #   | Recommendation                           | Effort | Impact                               |
| --- | ---------------------------------------- | ------ | ------------------------------------ |
| 5   | Add parent context sharing for subagents | Medium | High - Matches Claude Code           |
| 6   | Implement persistent TaskRegistry        | Medium | High - Cross-session resume          |
| 7   | Add EnterPlanMode/ExitPlanMode tools     | Medium | Medium - Dynamic planning            |
| 8   | Implement rolling compaction             | High   | Medium - Better context preservation |

### Priority 3: Enhanced Capabilities

| #   | Recommendation               | Effort | Impact                         |
| --- | ---------------------------- | ------ | ------------------------------ |
| 9   | Add LSP integration (gopls)  | High   | High - Major productivity gain |
| 10  | Implement sandbox support    | High   | High - Security requirement    |
| 11  | Add WebSearch/WebFetch tools | Medium | Medium - Research capability   |
| 12  | Add todo persistence         | Low    | Medium - Session continuity    |

### Priority 4: Polish

| #   | Recommendation                     | Effort | Impact                     |
| --- | ---------------------------------- | ------ | -------------------------- |
| 13  | Add TodoRead tool                  | Low    | Low - Convenience          |
| 14  | Document subagent file format      | Low    | Low - Developer experience |
| 15  | Add configuration file support     | Medium | Low - Convenience          |
| 16  | Standardize response type patterns | Low    | Low - Code quality         |

---

## Appendix: File Reference Index

| File                   | Key Contents                                                                        |
| ---------------------- | ----------------------------------------------------------------------------------- |
| `compaction.go`        | CompactionConfig, CompactionEvent, CompactionRecord, DefaultCompactionSummaryPrompt |
| `agent.go`             | StandardAgent, performCompaction(), executeToolCalls(), isToolAllowed()             |
| `thread.go`            | Thread, CompactionHistory, ForkThread()                                             |
| `subagent.go`          | SubagentDefinition, SubagentRegistry, FilterTools(), GeneralPurposeSubagent         |
| `subagent_loader.go`   | FileSubagentLoader, CompositeSubagentLoader                                         |
| `toolkit/task.go`      | TaskTool, TaskOutputTool, TaskRegistry, TaskRecord                                  |
| `skill/skill.go`       | Skill, SkillConfig, IsToolAllowed()                                                 |
| `skill/loader.go`      | Loader, LoaderOptions, discovery paths                                              |
| `toolkit/skill.go`     | SkillTool, SkillToolInput, GetActiveSkill(), ClearActiveSkill()                     |
| `permission.go`        | PermissionMode, ToolHookAction, PreToolUseHook, PostToolUseHook, CanUseToolFunc     |
| `permission_config.go` | PermissionConfig, PermissionManager                                                 |
| `permission_rules.go`  | PermissionRule, DenyRule(), AllowRule(), AskRule()                                  |
| `toolkit/todo.go`      | TodoWriteTool, TodoItem (toolkit version)                                           |
| `response.go`          | TodoItem, TodoStatus, TodoEvent, ResponseItemTypeTodo                               |
| `todo_tracker.go`      | TodoTracker, HandleEvent(), Progress(), DisplayProgress()                           |
| `interactor.go`        | UserInteractor, TerminalInteractor, InteractionMode                                 |
| `toolkit/ask_user.go`  | AskUserTool, AskUserInput, AskUserOutput                                            |
| `cmd/dive/app.go`      | App, handleCommand(), parseTodoWriteInput()                                         |
| `cmd/dive/render.go`   | todoListView(), toolCallView()                                                      |
