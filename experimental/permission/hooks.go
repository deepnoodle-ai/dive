package permission

import (
	"context"

	"github.com/deepnoodle-ai/dive"
)

// Hook returns a PreToolUseHook that implements permission checking.
//
// The hook evaluates the permission config and resolves confirmations internally.
// Returns nil (allow) or error (deny).
//
// Example:
//
//	config := &permission.Config{
//	    Mode: permission.ModeDefault,
//	    Rules: permission.Rules{
//	        permission.AllowRule("Read"),
//	        permission.AllowRule("Glob"),
//	        permission.AskRule("Bash", "Execute command?"),
//	    },
//	}
//
//	confirmer := func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, msg string) (bool, error) {
//	    return promptUser(msg), nil
//	}
//
//	preToolHook := permission.Hook(config, confirmer)
func Hook(config *Config, confirmer ConfirmFunc) dive.PreToolUseHook {
	manager := NewManager(config, confirmer)
	return HookFromManager(manager)
}

// HookFromManager returns a PreToolUseHook using an existing Manager.
//
// This is useful when you need access to the manager for session allowlist
// management or dynamic mode changes.
//
// Example:
//
//	manager := permission.NewManager(config, confirmer)
//	preToolHook := permission.HookFromManager(manager)
//
//	// Later, allow a category for the session
//	manager.AllowForSession("bash")
func HookFromManager(manager *Manager) dive.PreToolUseHook {
	return func(ctx context.Context, hookCtx *dive.PreToolUseContext) error {
		return manager.EvaluateToolUse(ctx, hookCtx.Tool, hookCtx.Call)
	}
}

// AuditHook returns a PreToolUseHook that logs all tool calls without making
// permission decisions.
//
// This is useful for monitoring and debugging. It always returns nil
// to let other hooks make the actual permission decision.
func AuditHook(logger func(toolName string, input []byte)) dive.PreToolUseHook {
	return func(ctx context.Context, hookCtx *dive.PreToolUseContext) error {
		toolName := "unknown"
		if hookCtx.Tool != nil {
			toolName = hookCtx.Tool.Name()
		}
		var input []byte
		if hookCtx.Call != nil {
			input = hookCtx.Call.Input
		}
		if logger != nil {
			logger(toolName, input)
		}
		return nil
	}
}
