# Permission System Guide

> **Experimental**: This package is in `experimental/permission/`. The API may change.

Dive's permission system is built on **PreToolUse hooks**. The `experimental/permission` package provides a higher-level permission manager with modes, rules, and session allowlists.

## Core: Using PreToolUse Hooks

The simplest way to control tool execution is with `PreToolUseHook` on `AgentOptions`. Hooks return `nil` to allow or `error` to deny:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        model,
    Tools:        tools,
    PreToolUse: []dive.PreToolUseHook{
        func(ctx context.Context, hookCtx *dive.PreToolUseContext) error {
            // Allow read-only tools automatically
            if hookCtx.Tool.Annotations() != nil && hookCtx.Tool.Annotations().ReadOnlyHint {
                return nil
            }
            // Block destructive operations
            if hookCtx.Tool.Annotations() != nil && hookCtx.Tool.Annotations().DestructiveHint {
                return fmt.Errorf("destructive operations not allowed")
            }
            // Allow everything else
            return nil
        },
    },
})
```

All hooks run in order. If any hook returns an error, the tool is denied and the error message is sent to the LLM. A `*dive.HookAbortError` aborts generation entirely.

## Experimental: PermissionManager

The `experimental/permission` package provides a `Manager` with declarative rules and modes:

```go
import "github.com/deepnoodle-ai/dive/experimental/permission"
```

### Permission Modes

| Mode                | Behavior                                        |
| ------------------- | ----------------------------------------------- |
| `Default`           | Standard rule-based checks                      |
| `Plan`              | Read-only mode (only `ReadOnlyHint=true` tools) |
| `AcceptEdits`       | Auto-accept file edit operations                |
| `BypassPermissions` | Allow all tools (use with caution)              |

### Permission Rules

Declarative allow/deny/ask rules with exact tool name matching (or `"*"` for all tools):

```go
rules := permission.Rules{
    permission.DenyCommandRule("Bash", "rm -rf", "Recursive deletion blocked"),
    permission.AllowRule("Read"),
    permission.AllowRule("Glob"),
    permission.AllowCommandRule("Bash", "go test"),
    permission.AskRule("Write", "Confirm file write"),
}
```

Ask rules call the `ConfirmFunc` to prompt the user. If no confirmer is set, ask rules auto-allow.

### Using as a Hook

```go
config := &permission.Config{
    Mode: permission.ModeDefault,
    Rules: rules,
}
confirmer := func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, msg string) (bool, error) {
    fmt.Printf("Allow %s? (y/n): ", tool.Name())
    var answer string
    fmt.Scanln(&answer)
    return answer == "y", nil
}

agent, _ := dive.NewAgent(dive.AgentOptions{
    Model:      model,
    Tools:      tools,
    PreToolUse: []dive.PreToolUseHook{permission.Hook(config, confirmer)},
})
```

### Permission Flow

When using the full `Manager`:

```text
Session Allowlist -> Deny Rules -> Allow Rules -> Ask Rules -> Mode Check -> Default (confirm)
```

### Session Allowlists

Users can approve "allow all X this session" for a tool category:

```go
manager := permission.NewManager(config, confirmer)
hook := permission.HookFromManager(manager)

// Later, allow a category for the session
manager.AllowForSession("bash")
manager.AllowForSession(permission.CategoryEdit.Key)
```

## Tool Annotations

Annotations on tools influence permission decisions:

| Annotation        | Description                                    |
| ----------------- | ---------------------------------------------- |
| `ReadOnlyHint`    | Tool only reads data (safe for Plan mode)      |
| `DestructiveHint` | Tool may cause irreversible changes            |
| `IdempotentHint`  | Tool can be safely retried                     |
| `EditHint`        | Tool modifies files (used by AcceptEdits mode) |

## Best Practices

1. Start restrictive and add explicit allow rules
2. Use deny rules for dangerous operations
3. Use PreToolUse hooks for audit logging
4. Set appropriate annotations on custom tools
