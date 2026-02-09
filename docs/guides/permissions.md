# Permission System Guide

Dive's permission system is built on **PreToolUse hooks**. The `permission` package provides a higher-level permission manager with modes, rules, and session allowlists.

## Core: Using PreToolUse Hooks

The simplest way to control tool execution is with `PreToolUseHook` on `AgentOptions`. Hooks return `nil` to allow or `error` to deny:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a helpful assistant.",
    Model:        model,
    Tools:        tools,
    Hooks: dive.Hooks{
        PreToolUse: []dive.PreToolUseHook{
            func(ctx context.Context, hctx *dive.HookContext) error {
                // Allow read-only tools automatically
                if hctx.Tool.Annotations() != nil && hctx.Tool.Annotations().ReadOnlyHint {
                    return nil
                }
                // Block destructive operations
                if hctx.Tool.Annotations() != nil && hctx.Tool.Annotations().DestructiveHint {
                    return fmt.Errorf("destructive operations not allowed")
                }
                // Allow everything else
                return nil
            },
        },
    },
})
```

All hooks run in order. If any hook returns an error, the tool is denied and the error message is sent to the LLM. A `*dive.HookAbortError` aborts generation entirely.

## Permission Manager

The `permission` package provides a `Manager` with declarative rules and modes:

```go
import "github.com/deepnoodle-ai/dive/permission"
```

### Permission Modes

| Mode                | Behavior                                               |
| ------------------- | ------------------------------------------------------ |
| `Default`           | Standard rule-based checks                             |
| `Plan`              | Read-only mode (only `ReadOnlyHint=true` tools)        |
| `AcceptEdits`       | Auto-accept file edit operations                       |
| `BypassPermissions` | Allow all tools (use with caution)                     |
| `DontAsk`           | Auto-deny unless explicitly allowed (headless/CI mode) |

### Permission Rules

Declarative allow/deny/ask rules with glob tool name matching and specifier patterns:

```go
rules := permission.Rules{
    permission.DenySpecifierRule("Bash", "rm -rf*", "Recursive deletion blocked"),
    permission.AllowRule("Read"),
    permission.AllowRule("Glob"),
    permission.AllowSpecifierRule("Bash", "go test*"),
    permission.AllowRule("mcp__*"),  // glob pattern matches all MCP tools
    permission.AskRule("Write", "Confirm file write"),
}
```

Ask rules call the `dive.Dialog` to prompt the user. If no dialog is set, ask rules auto-allow.

### Specifier Patterns

The `Specifier` field on a rule matches against tool-specific values extracted from the tool call input. For example, Bash rules match against the command, Read/Write/Edit rules match against the file path, and WebFetch rules match against the URL.

Default specifier fields:

| Tool     | Input fields checked                         |
| -------- | --------------------------------------------- |
| Bash     | `command`, `cmd`, `script`, `code`            |
| Read     | `file_path`, `filePath`, `path`               |
| Write    | `file_path`, `filePath`, `path`               |
| Edit     | `file_path`, `filePath`, `path`               |
| WebFetch | `url`                                         |

Override with `Config.SpecifierFields` for custom tools.

### Parsing Rules from Strings

```go
rule, err := permission.ParseRule(permission.RuleAllow, "Bash(go test *)")
// rule.Tool = "Bash", rule.Specifier = "go test *"

rule, err = permission.ParseRule(permission.RuleDeny, "mcp__*")
// rule.Tool = "mcp__*", rule.Specifier = ""
```

### Using as a Hook

```go
config := &permission.Config{
    Mode: permission.ModeDefault,
    Rules: rules,
}

agent, _ := dive.NewAgent(dive.AgentOptions{
    Model: model,
    Tools: tools,
    Hooks: dive.Hooks{
        PreToolUse: []dive.PreToolUseHook{permission.Hook(config, &dive.AutoApproveDialog{})},
    },
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
manager := permission.NewManager(config, dialog)
hook := permission.HookFromManager(manager)

// Later, allow a category for the session
manager.AllowForSession("bash")
manager.AllowForSession(permission.CategoryEdit.Key)
```

### DontAsk Mode

For headless or automation use cases, `ModeDontAsk` auto-denies any tool that is not explicitly allowed by a rule:

```go
config := &permission.Config{
    Mode: permission.ModeDontAsk,
    Rules: permission.Rules{
        permission.AllowRule("Read"),
        permission.AllowRule("Glob"),
        permission.AllowSpecifierRule("Bash", "go test*"),
    },
}
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
3. Use `ModeDontAsk` for headless/CI pipelines
4. Use PreToolUse hooks for audit logging
5. Set appropriate annotations on custom tools
