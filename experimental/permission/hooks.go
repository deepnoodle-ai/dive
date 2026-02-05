package permission

import (
	"context"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

// Hook returns a PreToolUseHook that implements permission checking.
//
// The hook evaluates the permission config and returns allow/deny/ask results.
// When the result is "ask", the confirmer function is called to get user approval.
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
func Hook(config *Config, confirmer dive.ConfirmToolFunc) dive.PreToolUseHook {
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
	return func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
		result, err := manager.EvaluateToolUse(ctx, hookCtx.Tool, hookCtx.Call)
		if err != nil {
			return nil, err
		}

		// If the result is "ask", invoke the confirmer
		if result.Action == dive.ToolHookAsk {
			confirmed, err := manager.Confirm(ctx, hookCtx.Tool, hookCtx.Call, result.Message)
			if err != nil {
				// Check if this is user feedback
				if feedback, ok := dive.IsUserFeedback(err); ok {
					return dive.DenyResult(feedback), nil
				}
				return nil, err
			}
			if confirmed {
				return dive.AllowResult(), nil
			}
			return dive.DenyResult("User denied tool call"), nil
		}

		return result, nil
	}
}

// HookWithOptions provides additional configuration for the permission hook.
type HookWithOptions struct {
	// Config is the permission configuration.
	Config *Config

	// Confirmer is called when user confirmation is needed.
	Confirmer dive.ConfirmToolFunc

	// OnAllow is called when a tool is allowed.
	OnAllow func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent)

	// OnDeny is called when a tool is denied.
	OnDeny func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, reason string)

	// OnAsk is called when user confirmation is requested.
	OnAsk func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, message string)
}

// Build returns a PreToolUseHook with the configured options.
func (o HookWithOptions) Build() dive.PreToolUseHook {
	manager := NewManager(o.Config, o.Confirmer)

	return func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
		result, err := manager.EvaluateToolUse(ctx, hookCtx.Tool, hookCtx.Call)
		if err != nil {
			return nil, err
		}

		switch result.Action {
		case dive.ToolHookAllow:
			if o.OnAllow != nil {
				o.OnAllow(ctx, hookCtx.Tool, hookCtx.Call)
			}
			return result, nil

		case dive.ToolHookDeny:
			if o.OnDeny != nil {
				o.OnDeny(ctx, hookCtx.Tool, hookCtx.Call, result.Message)
			}
			return result, nil

		case dive.ToolHookAsk:
			if o.OnAsk != nil {
				o.OnAsk(ctx, hookCtx.Tool, hookCtx.Call, result.Message)
			}
			confirmed, err := manager.Confirm(ctx, hookCtx.Tool, hookCtx.Call, result.Message)
			if err != nil {
				if feedback, ok := dive.IsUserFeedback(err); ok {
					if o.OnDeny != nil {
						o.OnDeny(ctx, hookCtx.Tool, hookCtx.Call, feedback)
					}
					return dive.DenyResult(feedback), nil
				}
				return nil, err
			}
			if confirmed {
				if o.OnAllow != nil {
					o.OnAllow(ctx, hookCtx.Tool, hookCtx.Call)
				}
				return dive.AllowResult(), nil
			}
			if o.OnDeny != nil {
				o.OnDeny(ctx, hookCtx.Tool, hookCtx.Call, "User denied tool call")
			}
			return dive.DenyResult("User denied tool call"), nil
		}

		return result, nil
	}
}

// AuditHook returns a PreToolUseHook that logs all tool calls without making
// permission decisions.
//
// This is useful for monitoring and debugging. It always returns ContinueResult()
// to let other hooks make the actual permission decision.
func AuditHook(logger func(toolName string, input []byte)) dive.PreToolUseHook {
	return func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
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
		return dive.ContinueResult(), nil
	}
}
