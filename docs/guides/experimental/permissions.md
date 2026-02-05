# Permission System Guide

> **Experimental**: This package is in `experimental/permission/`. The API may change.

Dive's core permission system is built on **hooks** defined in the main `dive` package. The `experimental/permission` package provides a higher-level `PermissionManager` with modes, rules, and session allowlists.

## Core: Using PreToolUse Hooks

The simplest way to control tool execution is with `PreToolUseHook` on `AgentOptions`:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        model,
    Tools:        tools,
    PreToolUse: []dive.PreToolUseHook{
        func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
            // Allow read-only tools automatically
            if hookCtx.Tool.Annotations() != nil && hookCtx.Tool.Annotations().ReadOnlyHint {
                return dive.AllowResult(), nil
            }
            // Block dangerous operations
            if hookCtx.Tool.Annotations() != nil && hookCtx.Tool.Annotations().DestructiveHint {
                return dive.DenyResult("Destructive operations not allowed"), nil
            }
            // Ask for confirmation on everything else
            return dive.AskResult("Execute this tool?"), nil
        },
    },
    Confirmer: func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, message string) (bool, error) {
        // Implement your confirmation UI here
        fmt.Printf("Allow %s? (y/n): ", tool.Name())
        var answer string
        fmt.Scanln(&answer)
        return answer == "y", nil
    },
})
```

### Hook Actions

| Action             | Effect                              |
| ------------------ | ----------------------------------- |
| `AllowResult()`    | Execute the tool immediately        |
| `DenyResult(msg)`  | Block execution with a message      |
| `AskResult(msg)`   | Prompt user for confirmation        |
| `ContinueResult()` | Defer to the next hook in the chain |

## Experimental: PermissionManager

The `experimental/permission` package provides a `PermissionManager` with declarative rules and modes:

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

Declarative allow/deny/ask rules with pattern matching:

```go
rules := permission.Rules{
    permission.DenyCommandRule("bash", "rm -rf *", "Recursive deletion blocked"),
    permission.AllowRule("read_*"),
    permission.AllowRule("glob"),
    permission.AllowCommandPrefixRule("bash", "go test"),
    permission.AskRule("write_*", "Confirm file write"),
}
```

### Settings File

Permission rules can be loaded from `.dive/settings.json`:

```json
{
  "permissions": {
    "allow": [
      "WebSearch",
      "Bash(go build:*)",
      "Bash(go test:*)",
      "Read(/path/to/project/**)"
    ],
    "deny": ["Bash(rm -rf:*)", "Bash(sudo:*)"]
  }
}
```

### Permission Flow

When using the full `PermissionManager`:

```
PreToolUse Hooks -> Session Allowlist -> Deny Rules -> Allow Rules -> Ask Rules -> Mode Check -> Execute
```

### Session Allowlists

Users can approve "allow all X this session" for a tool category:

```go
pm.AllowForSession("bash")
pm.AllowCategoryForSession(permission.ToolCategoryEdit)
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
