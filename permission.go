package dive

import (
	"context"
	"encoding/json"

	"github.com/deepnoodle-ai/dive/llm"
)

// Permission System
//
// This file defines the core permission types for Dive's tool permission system,
// which aligns with Anthropic's Claude Agent SDK permission specifications.
//
// The permission system provides four complementary ways to control tool usage:
//
//  1. Permission Modes - Global permission behavior settings (PermissionMode)
//  2. Tool Hooks - Pre/post execution hooks with allow/deny/ask/continue actions
//  3. Permission Rules - Declarative allow/deny/ask rules with pattern matching
//  4. CanUseTool Callback - Runtime permission handler for uncovered cases
//
// Permission Flow:
//
//	PreToolUse Hook → Deny Rules → Allow Rules → Ask Rules → Mode Check → CanUseTool → Execute → PostToolUse Hook
//
// Example usage:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Permission: &dive.PermissionConfig{
//	        Mode: dive.PermissionModeAcceptEdits,
//	        Rules: dive.PermissionRules{
//	            dive.DenyRule("dangerous_*", "Dangerous tools are blocked"),
//	            dive.AllowRule("read_*"),
//	        },
//	        PreToolUse: []dive.PreToolUseHook{
//	            func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
//	                log.Printf("Tool called: %s", hookCtx.Tool.Name())
//	                return dive.ContinueResult(), nil
//	            },
//	        },
//	    },
//	})

// PermissionMode determines the agent's global permission behavior.
// It affects how tools are evaluated when no explicit rules or hooks match.
type PermissionMode string

const (
	// PermissionModeDefault applies standard permission checks based on rules and hooks.
	// Tools not covered by explicit rules will fall through to the CanUseTool callback,
	// and if that's not set, will default to asking for user confirmation.
	// This is the recommended mode for most use cases.
	PermissionModeDefault PermissionMode = "default"

	// PermissionModePlan restricts the agent to read-only operations.
	// Tools with ReadOnlyHint=true are automatically allowed.
	// All other tools are denied unless explicitly allowed by rules or hooks.
	// Use this mode when you want the agent to explore and plan without making changes.
	PermissionModePlan PermissionMode = "plan"

	// PermissionModeAcceptEdits auto-accepts file edit operations without prompting.
	// This includes tools with EditHint=true, tools with names containing "edit", "write",
	// "create", and bash commands that modify the filesystem (mkdir, touch, rm, etc.).
	// Non-edit tools follow standard permission checks.
	// Use this mode for rapid development when you trust the agent's edit decisions.
	PermissionModeAcceptEdits PermissionMode = "acceptEdits"

	// PermissionModeBypassPermissions allows ALL tools to run without permission prompts.
	// No rules or hooks are evaluated - everything is automatically allowed.
	// WARNING: Use with extreme caution. This gives the agent full, unrestricted access
	// to all tools. Only use in controlled environments or for fully automated workflows.
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
)

// ToolHookAction represents the action to take after a hook or rule evaluates.
// These actions control the flow of the permission evaluation pipeline.
type ToolHookAction string

const (
	// ToolHookAllow permits the tool to execute immediately without further checks.
	// The permission flow stops here and the tool is executed.
	// Use AllowResult() or AllowResultWithInput() to create this action.
	ToolHookAllow ToolHookAction = "allow"

	// ToolHookDeny blocks the tool execution immediately.
	// The permission flow stops here and an error result is returned to the LLM.
	// Use DenyResult(message) to create this action with an explanation.
	ToolHookDeny ToolHookAction = "deny"

	// ToolHookAsk prompts the user for confirmation before executing.
	// The ConfirmToolFunc callback is invoked to get user approval.
	// Use AskResult(message) to create this action with a prompt message.
	ToolHookAsk ToolHookAction = "ask"

	// ToolHookContinue proceeds to the next step in the permission flow.
	// Use this when your hook doesn't want to make a decision and wants
	// to defer to subsequent rules, modes, or callbacks.
	// Use ContinueResult() to create this action.
	ToolHookContinue ToolHookAction = "continue"
)

// ToolHookResult is returned by tool hooks to control execution flow.
// It contains the action to take (allow, deny, ask, continue), an optional
// message, and metadata about the decision.
type ToolHookResult struct {
	// Action determines what happens next in the permission flow.
	// See [ToolHookAllow], [ToolHookDeny], [ToolHookAsk], [ToolHookContinue].
	Action ToolHookAction `json:"action"`

	// Message provides context for deny/ask actions.
	// For deny: explains why the tool was blocked (returned to the LLM).
	// For ask: displayed to the user when prompting for confirmation.
	Message string `json:"message,omitempty"`

	// UpdatedInput optionally provides modified input for allow actions.
	// If set, the tool will be called with this input instead of the original.
	// Use [AllowResultWithInput] to create a result with modified input.
	UpdatedInput json.RawMessage `json:"updatedInput,omitempty"`

	// Category contains the tool category that triggered this result.
	// This is populated in two cases:
	//   1. When the result comes from a session allowlist check (the category
	//      that was previously allowed via [PermissionManager.AllowForSession])
	//   2. When the result is a default "ask" action, providing the category
	//      so UIs can offer "allow all [category] this session" options
	//
	// The category enables "allow all X this session" functionality in UIs.
	// When a user approves a tool call, the UI can use Category.Key to call
	// [PermissionManager.AllowForSession] for future auto-approval.
	//
	// Example usage in a confirmation dialog:
	//
	//	if result.Action == ToolHookAsk && result.Category != nil {
	//	    // Show "Allow all [category.Label] this session" checkbox
	//	    if userSelectedAllowAll {
	//	        pm.AllowForSession(result.Category.Key)
	//	    }
	//	}
	Category *ToolCategory `json:"category,omitempty"`
}

// PreToolUseContext contains information passed to PreToolUse hooks.
type PreToolUseContext struct {
	// Tool is the tool being called.
	Tool Tool

	// Call contains the tool call details including ID, name, and input.
	Call *llm.ToolUseContent

	// Agent is the agent executing the tool.
	Agent Agent
}

// PostToolUseContext contains information passed to PostToolUse hooks.
type PostToolUseContext struct {
	// Tool is the tool that was called.
	Tool Tool

	// Call contains the tool call details.
	Call *llm.ToolUseContent

	// Result contains the result of the tool execution.
	Result *ToolCallResult

	// Agent is the agent that executed the tool.
	Agent Agent
}

// PreToolUseHook is called before a tool is executed.
// Hooks are called in order and can control the permission flow by returning
// different ToolHookAction values:
//
//   - ToolHookAllow: Immediately allow the tool to execute
//   - ToolHookDeny: Immediately block the tool execution
//   - ToolHookAsk: Prompt the user for confirmation
//   - ToolHookContinue: Proceed to the next hook or permission check
//
// Example:
//
//	func auditHook(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
//	    log.Printf("Tool %s called with input: %s", hookCtx.Tool.Name(), hookCtx.Call.Input)
//	    return dive.ContinueResult(), nil // Let other rules decide
//	}
type PreToolUseHook func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error)

// PostToolUseHook is called after a tool has executed.
// Use for logging, auditing, metrics collection, or cleanup.
// PostToolUse hooks cannot modify the tool result - they are purely observational.
// If an error is returned, it is logged but does not affect the response.
//
// Example:
//
//	func metricsHook(ctx context.Context, hookCtx *dive.PostToolUseContext) error {
//	    metrics.RecordToolCall(hookCtx.Tool.Name(), hookCtx.Result.Error == nil)
//	    return nil
//	}
type PostToolUseHook func(ctx context.Context, hookCtx *PostToolUseContext) error

// CanUseToolFunc is a callback for runtime permission decisions.
// It's invoked when no PreToolUse hooks or permission rules have made a determination.
// This is the final programmatic chance to allow, deny, or prompt for the tool
// before falling back to the default "ask" behavior.
//
// Example:
//
//	func customPermissions(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent) (*dive.ToolHookResult, error) {
//	    // Check against an external permission service
//	    if permissionService.IsAllowed(ctx, tool.Name()) {
//	        return dive.AllowResult(), nil
//	    }
//	    return dive.AskResult("This tool requires approval"), nil
//	}
type CanUseToolFunc func(ctx context.Context, tool Tool, call *llm.ToolUseContent) (*ToolHookResult, error)

// ConfirmToolFunc is a callback for prompting the user for tool confirmation.
// It's invoked when the permission flow determines the user should be asked (ToolHookAsk).
// Return true to allow the tool execution, false to deny it.
//
// The message parameter contains context about what's being confirmed, which may come
// from the hook/rule that triggered the ask, or from the tool's preview.
type ConfirmToolFunc func(ctx context.Context, tool Tool, call *llm.ToolUseContent, message string) (bool, error)

// AllowResult returns a ToolHookResult that allows tool execution.
func AllowResult() *ToolHookResult {
	return &ToolHookResult{Action: ToolHookAllow}
}

// AllowResultWithInput returns a ToolHookResult that allows tool execution
// with modified input.
func AllowResultWithInput(input json.RawMessage) *ToolHookResult {
	return &ToolHookResult{Action: ToolHookAllow, UpdatedInput: input}
}

// DenyResult returns a ToolHookResult that denies tool execution.
func DenyResult(message string) *ToolHookResult {
	return &ToolHookResult{Action: ToolHookDeny, Message: message}
}

// AskResult returns a ToolHookResult that prompts the user for confirmation.
func AskResult(message string) *ToolHookResult {
	return &ToolHookResult{Action: ToolHookAsk, Message: message}
}

// ContinueResult returns a ToolHookResult that continues to the next step.
func ContinueResult() *ToolHookResult {
	return &ToolHookResult{Action: ToolHookContinue}
}
