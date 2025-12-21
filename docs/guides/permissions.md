# Permission System Guide

Dive's permission system controls when and how tools can be executed, providing fine-grained control over agent behavior. The system aligns with Anthropic's Claude Agent SDK permission specifications.

## Overview

The permission system provides four complementary ways to control tool usage:

1. **Permission Modes** - Global permission behavior settings
2. **Permission Rules** - Declarative allow/deny/ask rules with pattern matching
3. **Tool Hooks** - Pre/post execution hooks for custom logic
4. **CanUseTool Callback** - Runtime permission handler for dynamic decisions

## Permission Flow

When a tool is called, Dive evaluates permissions in this order:

```
PreToolUse Hooks → Deny Rules → Allow Rules → Ask Rules → Mode Check → CanUseTool → Execute → PostToolUse Hooks
```

Each step can terminate the flow early by returning `allow`, `deny`, or `ask`. Only `continue` passes control to the next step.

## Permission Modes

Set the global permission behavior with `PermissionMode`:

| Mode                              | Behavior                                                    |
| --------------------------------- | ----------------------------------------------------------- |
| `PermissionModeDefault`           | Standard checks - falls through to CanUseTool or asks user  |
| `PermissionModePlan`              | Read-only mode - only allows tools with `ReadOnlyHint=true` |
| `PermissionModeAcceptEdits`       | Auto-accepts file edit operations without prompting         |
| `PermissionModeBypassPermissions` | Allows ALL tools without any prompts (use with caution)     |

### Basic Mode Configuration

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:  "Assistant",
    Model: anthropic.New(),
    Permission: &dive.PermissionConfig{
        Mode: dive.PermissionModePlan,
    },
})
```

### AcceptEdits Mode

The `PermissionModeAcceptEdits` mode automatically allows edit operations, detected by:

1. Tools with `EditHint: true` annotation
2. Tool names containing: `edit`, `write`, `create`, `mkdir`, `touch`, `mv`, `cp`, `rm`
3. Bash commands that modify the filesystem: `mkdir`, `touch`, `rm`, `rmdir`, `mv`, `cp`, `cat >`, `echo >`, `tee`, `sed -i`, `chmod`, `chown`

```go
Permission: &dive.PermissionConfig{
    Mode: dive.PermissionModeAcceptEdits,
}
```

## Permission Rules

Declarative rules provide static, configuration-driven access control. Rules are evaluated in order within each category: deny rules first, then allow rules, then ask rules.

### Rule Types

- **Deny Rules** - Block tool execution immediately
- **Allow Rules** - Permit execution without prompting
- **Ask Rules** - Prompt user for confirmation

### Pattern Matching

Tool patterns use glob-style matching:

- `"bash"` - matches exactly "bash"
- `"read_*"` - matches "read_file", "read_config", etc.
- `"*"` - matches any tool

Command patterns (for bash-like tools) also support wildcards:

- `"rm -rf *"` - matches any rm -rf command
- `"git push *"` - matches any git push command

### Rule Examples

```go
Permission: &dive.PermissionConfig{
    Mode: dive.PermissionModeDefault,
    Rules: dive.PermissionRules{
        // Block dangerous operations
        dive.DenyRule("dangerous_*", "Dangerous tools are blocked"),
        dive.DenyCommandRule("bash", "rm -rf *", "Recursive deletion not allowed"),
        dive.DenyCommandRule("bash", "sudo *", "Sudo commands are blocked"),

        // Allow safe read operations
        dive.AllowRule("read_*"),
        dive.AllowRule("glob"),
        dive.AllowRule("grep"),
        dive.AllowCommandRule("bash", "ls *"),
        dive.AllowCommandRule("bash", "git status"),

        // Prompt for writes
        dive.AskRule("write_*", "Confirm file write operation"),
        dive.AskCommandRule("bash", "git push *", "Confirm push to remote"),
    },
}
```

### Rule Helper Functions

| Function                                  | Description                       |
| ----------------------------------------- | --------------------------------- |
| `DenyRule(pattern, message)`              | Block tools matching pattern      |
| `DenyCommandRule(tool, command, message)` | Block specific bash commands      |
| `AllowRule(pattern)`                      | Allow tools matching pattern      |
| `AllowCommandRule(tool, command)`         | Allow specific bash commands      |
| `AskRule(pattern, message)`               | Prompt for tools matching pattern |
| `AskCommandRule(tool, command, message)`  | Prompt for specific bash commands |

### Custom Input Matching

For complex validation beyond pattern matching, use `InputMatch`:

```go
rule := dive.PermissionRule{
    Type: dive.PermissionRuleDeny,
    Tool: "write_file",
    InputMatch: func(input any) bool {
        m, ok := input.(map[string]any)
        if !ok {
            return false
        }
        path, _ := m["path"].(string)
        return strings.HasPrefix(path, "/etc/")  // Block writes to /etc/
    },
    Message: "Cannot write to system directories",
}
```

## Tool Hooks

Hooks provide programmatic control over tool execution with custom logic.

### PreToolUse Hooks

Called before tool execution. Can allow, deny, ask, or continue the flow.

```go
Permission: &dive.PermissionConfig{
    PreToolUse: []dive.PreToolUseHook{
        // Audit logging
        func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
            log.Printf("Tool %s called with input: %s",
                hookCtx.Tool.Name(), hookCtx.Call.Input)
            return dive.ContinueResult(), nil
        },

        // Rate limiting
        func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
            if rateLimiter.IsExceeded(hookCtx.Tool.Name()) {
                return dive.DenyResult("Rate limit exceeded"), nil
            }
            return dive.ContinueResult(), nil
        },
    },
}
```

### Hook Actions

| Action             | Effect                       |
| ------------------ | ---------------------------- |
| `ToolHookAllow`    | Immediately allow execution  |
| `ToolHookDeny`     | Block execution with message |
| `ToolHookAsk`      | Prompt user for confirmation |
| `ToolHookContinue` | Proceed to next step in flow |

### Helper Functions

```go
dive.AllowResult()              // Allow the tool
dive.AllowResultWithInput(json) // Allow with modified input
dive.DenyResult("reason")       // Deny with message
dive.AskResult("prompt")        // Ask user with message
dive.ContinueResult()           // Continue to next step
```

### PostToolUse Hooks

Called after tool execution. These are observational only and cannot modify the result.

```go
Permission: &dive.PermissionConfig{
    PostToolUse: []dive.PostToolUseHook{
        func(ctx context.Context, hookCtx *dive.PostToolUseContext) error {
            metrics.RecordToolCall(
                hookCtx.Tool.Name(),
                hookCtx.Result.Error == nil,
            )
            return nil
        },
    },
}
```

### Modifying Tool Input

PreToolUse hooks can modify tool input before execution:

```go
func sanitizeInput(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
    if hookCtx.Tool.Name() == "bash" {
        var input map[string]any
        json.Unmarshal(hookCtx.Call.Input, &input)

        // Sanitize the command
        cmd := input["command"].(string)
        input["command"] = sanitize(cmd)

        modified, _ := json.Marshal(input)
        return dive.AllowResultWithInput(modified), nil
    }
    return dive.ContinueResult(), nil
}
```

## CanUseTool Callback

The `CanUseTool` callback provides runtime permission decisions for cases not covered by rules. It's called after rules and mode checks.

```go
Permission: &dive.PermissionConfig{
    Mode: dive.PermissionModeDefault,
    CanUseTool: func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent) (*dive.ToolHookResult, error) {
        // Check against external permission service
        if permissionService.IsAllowed(ctx, tool.Name()) {
            return dive.AllowResult(), nil
        }

        // Check tool annotations
        if tool.Annotations().DestructiveHint {
            return dive.AskResult("This is a destructive operation"), nil
        }

        return dive.ContinueResult(), nil  // Fall through to default ask
    },
}
```

## Complete Example

Here's a comprehensive permission configuration:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    Name:         "Secure Assistant",
    Instructions: "You are a helpful coding assistant.",
    Model:        anthropic.New(),
    Permission: &dive.PermissionConfig{
        Mode: dive.PermissionModeDefault,

        Rules: dive.PermissionRules{
            // Security: block dangerous operations
            dive.DenyCommandRule("bash", "rm -rf *", "Recursive deletion blocked"),
            dive.DenyCommandRule("bash", "sudo *", "Sudo not allowed"),
            dive.DenyRule("execute_code", "Code execution disabled"),

            // Allow safe read operations
            dive.AllowRule("read_*"),
            dive.AllowRule("glob"),
            dive.AllowRule("grep"),
            dive.AllowCommandRule("bash", "ls *"),
            dive.AllowCommandRule("bash", "cat *"),
            dive.AllowCommandRule("bash", "git status"),
            dive.AllowCommandRule("bash", "git diff *"),

            // Prompt for file modifications
            dive.AskRule("write_*", "Confirm file write"),
            dive.AskRule("edit_*", "Confirm file edit"),
            dive.AskCommandRule("bash", "git commit *", "Confirm commit"),
            dive.AskCommandRule("bash", "git push *", "Confirm push"),
        },

        PreToolUse: []dive.PreToolUseHook{
            // Audit logging
            func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
                auditLog.Record(hookCtx.Tool.Name(), hookCtx.Call.Input)
                return dive.ContinueResult(), nil
            },
        },

        PostToolUse: []dive.PostToolUseHook{
            // Metrics collection
            func(ctx context.Context, hookCtx *dive.PostToolUseContext) error {
                metrics.RecordToolExecution(hookCtx.Tool.Name(), hookCtx.Result)
                return nil
            },
        },

        CanUseTool: func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent) (*dive.ToolHookResult, error) {
            // Default: ask for confirmation on unknown tools
            return dive.AskResult("Confirm tool execution"), nil
        },
    },
})
```

## Tool Annotations

Tools can declare hints that affect permission evaluation:

| Annotation        | Description                                    |
| ----------------- | ---------------------------------------------- |
| `ReadOnlyHint`    | Tool only reads data, doesn't modify state     |
| `DestructiveHint` | Tool may cause irreversible changes            |
| `IdempotentHint`  | Tool can be safely retried                     |
| `EditHint`        | Tool modifies files (used by AcceptEdits mode) |

When creating custom tools, set appropriate annotations:

```go
func (t *MyTool) Annotations() *dive.ToolAnnotations {
    return &dive.ToolAnnotations{
        ReadOnlyHint: true,  // Safe for plan mode
    }
}
```

## Backward Compatibility

If you're migrating from the older `Interactor` pattern, Dive provides automatic conversion:

| InteractionMode         | Equivalent PermissionConfig       |
| ----------------------- | --------------------------------- |
| `InteractNever`         | `PermissionModeBypassPermissions` |
| `InteractAlways`        | Ask rule for all tools            |
| `InteractIfDestructive` | Ask if `DestructiveHint` is true  |
| `InteractIfNotReadOnly` | Ask if `ReadOnlyHint` is false    |

The `Interactor` field in `AgentOptions` still works but is deprecated for tool confirmations. Use `Permission` instead for new code.

## Dynamic Mode Changes

You can change the permission mode at runtime:

```go
// Get the permission manager from agent internals
// Then call SetMode to change behavior
permissionManager.SetMode(dive.PermissionModeBypassPermissions)
```

This is useful for workflows that start with careful review and switch to faster execution after establishing trust.

## Best Practices

1. **Start restrictive** - Begin with `PermissionModeDefault` and explicit allow rules
2. **Use deny rules for security** - Block dangerous operations explicitly
3. **Log tool calls** - Use PreToolUse hooks for audit trails
4. **Set appropriate annotations** - Mark tools with ReadOnlyHint, EditHint, etc.
5. **Test permission flows** - Verify rules work as expected before deployment
6. **Use CanUseTool for edge cases** - Handle dynamic decisions programmatically
