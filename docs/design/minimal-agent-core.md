# Minimal Agent Core Design

## Problem Statement

The current `StandardAgent` implementation has accumulated responsibilities beyond its core purpose. It directly manages:

- **SubagentRegistry** - Knowledge of child agent spawning
- **UserInteractor** - User input/confirmation handling
- **SessionRepository** - Conversation persistence
- **PermissionManager** - Tool permission evaluation
- **System prompt templating** - Building prompts from Name/Goal/Instructions
- **IsSupervisor/Subordinates** - Obsolete delegation model

This creates tight coupling, makes the agent harder to understand, and forces all users to work within these abstractions even when they don't need them.

## Design Principles

1. **The agent is a generation loop** - Messages in, LLM calls, tool execution, messages out
2. **Extensibility via hooks** - All customization happens through composable hooks
3. **Tools own their dependencies** - Tools that need special resources receive them at construction
4. **Explicit over implicit** - No magic; callers see what's happening

## Proposed Architecture

### Core Agent

```go
type AgentOptions struct {
    // Required
    SystemPrompt string
    Model        llm.LLM

    // Tools
    Tools []Tool

    // Generation-level hooks
    PreGeneration  []PreGenerationHook
    PostGeneration []PostGenerationHook

    // Tool-level hooks
    PreToolUse  []PreToolUseHook
    PostToolUse []PostToolUseHook

    // Infrastructure
    Logger        llm.Logger
    ModelSettings *ModelSettings
    Hooks         llm.Hooks  // LLM-level hooks (existing)
}
```

### Removed from Agent

| Field                          | Reason                                  | New Location                       |
| ------------------------------ | --------------------------------------- | ---------------------------------- |
| `Name`, `Goal`, `Instructions` | Caller builds SystemPrompt              | Caller code                        |
| `IsSupervisor`, `Subordinates` | Obsolete; Task tool replaced this       | Removed entirely                   |
| `SessionRepository`            | Persistence is external concern         | PreGeneration/PostGeneration hooks |
| `Interactor`                   | Only needed by AskUserQuestion tool     | Tool constructor                   |
| `Subagents`, `SubagentLoader`  | Only needed by Task tool                | Tool constructor                   |
| `Permission`                   | Composed from PreToolUse hooks          | Hook helpers                       |
| `systemPromptTemplate`         | Over-engineered                         | Removed; use plain string          |
| `Context`                      | Can be prepended via PreGeneration hook | Hook                               |

## Hook System

### Generation Hooks

Generation hooks run before and after the entire generation loop (which may include multiple LLM calls for tool use).

```go
// GenerationState provides mutable access to the generation context
type GenerationState struct {
    // Identifiers
    SessionID string
    UserID    string

    // Input (mutable in PreGeneration)
    SystemPrompt string
    Messages     []*llm.Message

    // Output (populated after generation, available in PostGeneration)
    Response       *Response
    OutputMessages []*llm.Message
    Usage          *llm.Usage

    // Arbitrary storage for hooks to communicate
    Values map[string]any
}

// PreGenerationHook is called before the generation loop starts.
// Hooks can modify state.Messages, state.SystemPrompt, etc.
// Return an error to abort generation.
type PreGenerationHook func(ctx context.Context, state *GenerationState) error

// PostGenerationHook is called after generation completes.
// Hooks can read the response, save state, log metrics, etc.
// Errors are logged but do not affect the response.
type PostGenerationHook func(ctx context.Context, state *GenerationState) error
```

### Tool Hooks

Tool hooks run around individual tool executions. These already exist in the current design.

```go
type PreToolUseContext struct {
    Tool  Tool
    Call  *llm.ToolUseContent
    Agent Agent
}

type PostToolUseContext struct {
    Tool   Tool
    Call   *llm.ToolUseContent
    Result *ToolCallResult
    Agent  Agent
}

type PreToolUseHook func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error)
type PostToolUseHook func(ctx context.Context, hookCtx *PostToolUseContext) error
```

### Hook Execution Flow

```
CreateResponse(messages)
       │
       ▼
┌─────────────────────────────────────────┐
│  PreGeneration Hooks (in order)         │
│  - Load session messages                │
│  - Inject context/reminders             │
│  - Apply compaction if needed           │
└─────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────┐
│  Generation Loop                        │
│  ┌───────────────────────────────────┐  │
│  │ Call LLM                          │  │
│  └───────────────────────────────────┘  │
│       │                                 │
│       ▼ (if tool calls)                 │
│  ┌───────────────────────────────────┐  │
│  │ For each tool call:               │  │
│  │   PreToolUse Hooks                │  │
│  │   Execute Tool                    │  │
│  │   PostToolUse Hooks               │  │
│  └───────────────────────────────────┘  │
│       │                                 │
│       ▼ (loop until no tool calls)      │
└─────────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────────┐
│  PostGeneration Hooks (in order)        │
│  - Save session                         │
│  - Log usage metrics                    │
│  - Trigger async workflows              │
└─────────────────────────────────────────┘
       │
       ▼
    Response
```

## Common Patterns as Hooks

### Session Management

```go
// SessionHooks returns pre/post hooks for session persistence
func SessionHooks(repo SessionRepository) (PreGenerationHook, PostGenerationHook) {
    loader := func(ctx context.Context, state *GenerationState) error {
        if state.SessionID == "" {
            state.SessionID = newSessionID()
        }
        session, err := repo.GetSession(ctx, state.SessionID)
        if err == ErrSessionNotFound {
            return nil
        }
        if err != nil {
            return err
        }
        // Prepend history to input messages
        state.Messages = append(slices.Clone(session.Messages), state.Messages...)
        return nil
    }

    saver := func(ctx context.Context, state *GenerationState) error {
        session := &Session{
            ID:        state.SessionID,
            UserID:    state.UserID,
            Messages:  state.Messages,
            UpdatedAt: time.Now(),
        }
        return repo.PutSession(ctx, session)
    }

    return loader, saver
}

// Usage
loader, saver := dive.SessionHooks(myRepo)
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt:   "You are a helpful assistant.",
    Model:          model,
    PreGeneration:  []dive.PreGenerationHook{loader},
    PostGeneration: []dive.PostGenerationHook{saver},
})
```

### Permission Rules

```go
// PermissionHooks converts permission rules to PreToolUse hooks
func PermissionHooks(config PermissionConfig) PreToolUseHook {
    return func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
        // Evaluate deny rules
        for _, rule := range config.DenyRules {
            if rule.Matches(hookCtx.Tool, hookCtx.Call) {
                return DenyResult(rule.Message), nil
            }
        }
        // Evaluate allow rules
        for _, rule := range config.AllowRules {
            if rule.Matches(hookCtx.Tool, hookCtx.Call) {
                return AllowResult(), nil
            }
        }
        // Default behavior based on mode
        switch config.Mode {
        case PermissionModeBypass:
            return AllowResult(), nil
        case PermissionModePlan:
            if hookCtx.Tool.Annotations().ReadOnlyHint {
                return AllowResult(), nil
            }
            return DenyResult("Plan mode: write operations not allowed"), nil
        default:
            return AskResult(""), nil
        }
    }
}

// Usage
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: prompt,
    Model:        model,
    Tools:        tools,
    PreToolUse:   []dive.PreToolUseHook{
        dive.PermissionHooks(dive.PermissionConfig{
            Mode: dive.PermissionModeDefault,
            AllowRules: []dive.PermissionRule{
                dive.AllowRule("Read"),
                dive.AllowRule("Glob"),
            },
        }),
    },
})
```

### Context Injection

```go
// InjectContext adds content to every generation
func InjectContext(content ...llm.Content) PreGenerationHook {
    return func(ctx context.Context, state *GenerationState) error {
        contextMsg := llm.NewUserMessage(content...)
        state.Messages = append([]*llm.Message{contextMsg}, state.Messages...)
        return nil
    }
}

// Usage: inject codebase context
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt:  prompt,
    Model:         model,
    PreGeneration: []dive.PreGenerationHook{
        dive.InjectContext(llm.NewTextContent(claudeMDContents)),
    },
})
```

### Compaction

```go
// CompactionHook triggers summarization when context is too large
func CompactionHook(threshold int, summarizer func(context.Context, []*llm.Message) ([]*llm.Message, error)) PreGenerationHook {
    return func(ctx context.Context, state *GenerationState) error {
        tokens := estimateTokens(state.Messages)
        if tokens < threshold {
            return nil
        }
        compacted, err := summarizer(ctx, state.Messages)
        if err != nil {
            return err
        }
        state.Messages = compacted
        return nil
    }
}
```

### Usage Logging

```go
// UsageLogger logs token usage after each generation
func UsageLogger(logger *slog.Logger) PostGenerationHook {
    return func(ctx context.Context, state *GenerationState) error {
        if state.Usage != nil {
            logger.Info("generation complete",
                "session_id", state.SessionID,
                "input_tokens", state.Usage.InputTokens,
                "output_tokens", state.Usage.OutputTokens,
            )
        }
        return nil
    }
}
```

## Tool Dependency Injection

Tools that require external resources receive them at construction, not from the agent.

### Task Tool (Subagents)

```go
// Before: agent.SubagentRegistry held definitions
// After: Task tool receives registry at construction

registry := dive.NewSubagentRegistry()
registry.Register("code-reviewer", &dive.SubagentDefinition{
    Description: "Reviews code for issues",
    Prompt:      "You are a code reviewer...",
    Tools:       []string{"Read", "Glob", "Grep"},
})

taskTool := toolkit.NewTaskTool(toolkit.TaskToolOptions{
    Registry:    registry,
    ParentAgent: agent,  // or parent model/tools for spawning
})

agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: prompt,
    Model:        model,
    Tools:        []dive.Tool{readTool, taskTool},
})
```

### AskUserQuestion Tool

```go
// Before: agent.Interactor was used
// After: AskUserQuestion tool receives interactor at construction

interactor := dive.NewTerminalInteractor()
askTool := toolkit.NewAskUserTool(interactor)

agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: prompt,
    Model:        model,
    Tools:        []dive.Tool{askTool, otherTools...},
})
```

## Migration Path

### Phase 1: Add Hook Infrastructure

1. Add `PreGeneration` and `PostGeneration` hook types
2. Add hook execution to `CreateResponse`
3. Keep existing fields as deprecated

### Phase 2: Provide Hook-Based Alternatives

1. Create `SessionHooks()` helper
2. Create `PermissionHooks()` helper
3. Update toolkit constructors to accept dependencies

### Phase 3: Deprecate Embedded Components

1. Mark `SessionRepository`, `Permission`, `Interactor` as deprecated
2. Mark `IsSupervisor`, `Subordinates`, `Goal`, `Instructions` as deprecated
3. Update documentation and examples

### Phase 4: Remove Deprecated Fields

1. Remove deprecated fields from `AgentOptions`
2. Remove `systemPromptTemplate` logic
3. Simplify `StandardAgent` struct

## Example: Complete Agent Setup

```go
// Build system prompt explicitly
systemPrompt := `You are a helpful coding assistant.

You have access to tools for reading, writing, and searching code.
Always explain your reasoning before making changes.`

// Set up session persistence
repo := dive.NewFileSessionRepository("./sessions")
sessionLoader, sessionSaver := dive.SessionHooks(repo)

// Set up permissions
permissions := dive.PermissionHooks(dive.PermissionConfig{
    Mode: dive.PermissionModeDefault,
    AllowRules: []dive.PermissionRule{
        dive.AllowRule("Read"),
        dive.AllowRule("Glob"),
        dive.AllowRule("Grep"),
    },
})

// Set up user interaction for AskUserQuestion
interactor := NewMyCustomInteractor()

// Set up subagents for Task tool
registry := dive.NewSubagentRegistry()
registry.Register("researcher", &dive.SubagentDefinition{
    Description: "Researches codebases to answer questions",
    Prompt:      "You are a code researcher...",
    Tools:       []string{"Read", "Glob", "Grep"},
    Model:       "haiku",
})

// Build tools with their dependencies
tools := []dive.Tool{
    toolkit.NewReadTool(),
    toolkit.NewWriteTool(),
    toolkit.NewGlobTool(),
    toolkit.NewGrepTool(),
    toolkit.NewBashTool(),
    toolkit.NewAskUserTool(interactor),
    toolkit.NewTaskTool(toolkit.TaskToolOptions{
        Registry: registry,
        Model:    model,
    }),
}

// Create the agent
agent, err := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: systemPrompt,
    Model:        anthropic.New("claude-sonnet-4-20250514"),
    Tools:        tools,

    PreGeneration:  []dive.PreGenerationHook{sessionLoader},
    PostGeneration: []dive.PostGenerationHook{sessionSaver},
    PreToolUse:     []dive.PreToolUseHook{permissions},

    Logger: slog.Default(),
})
```

## Benefits

1. **Simpler core** - Agent only handles generation loop
2. **Composable** - Mix and match hooks for different behaviors
3. **Testable** - Hooks can be unit tested in isolation
4. **Explicit** - No hidden behavior; caller sees all configuration
5. **Flexible** - Custom session stores, permission systems, etc. without modifying agent

## Resolved Questions

1. **Hook error handling** - ✅ Resolved: PreGeneration errors abort generation (returns error to caller). PostGeneration errors are logged but don't affect the returned Response. This ensures generation results aren't lost due to post-processing failures.

2. **Hook ordering** - ✅ Resolved: Hooks run in the order they are provided in the slice. No dependency declaration mechanism is needed—callers control order by arranging hooks in their slices. This keeps the API simple and explicit. If complex ordering is needed, callers can compose hooks or use a helper that sorts them.

3. **State sharing** - ✅ Resolved: `Values map[string]any` is sufficient. Hooks store typed data with string keys (e.g., `state.Values["session"]`, `state.Values["compaction_event"]`). Type assertions are used when retrieving values. This approach is simple, flexible, and matches common patterns in Go middleware.

4. **Confirmation flow** - ✅ Resolved: PreToolUse hooks that return `AskResult` trigger the permission manager's `Confirm` method. For the experimental permission package, `HookFromManager` handles this automatically—it calls the confirmer function when the hook returns Ask, then converts the confirmation result to Allow or Deny. This keeps the hook return values simple (Allow/Deny/Ask/Continue) while allowing custom confirmation UI.

## Related Documents

- [Permissions Guide](../guides/permissions.md)
- [Tools Guide](../guides/tools.md)
- [Custom Tools Guide](../guides/custom-tools.md)
