package dive

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

// Permission Configuration and Manager
//
// This file implements the PermissionConfig and PermissionManager types that
// orchestrate the permission evaluation flow for tool calls.
//
// The PermissionManager implements Anthropic's permission flow:
//
//	PreToolUse Hook → Deny Rules → Allow Rules → Ask Rules → Mode Check → CanUseTool → Confirm
//
// Each step can terminate the flow early by returning allow, deny, or ask.
// Only ToolHookContinue passes control to the next step.

// PermissionConfig contains all permission-related configuration for an agent.
// It combines the permission mode, declarative rules, and programmatic callbacks
// into a single configuration object.
//
// Example - Basic mode configuration:
//
//	config := &dive.PermissionConfig{
//	    Mode: dive.PermissionModeAcceptEdits,
//	}
//
// Example - Full configuration with rules and hooks:
//
//	config := &dive.PermissionConfig{
//	    Mode: dive.PermissionModeDefault,
//	    Rules: dive.PermissionRules{
//	        dive.DenyRule("dangerous_*", "Blocked"),
//	        dive.AllowRule("read_*"),
//	    },
//	    PreToolUse: []dive.PreToolUseHook{auditLogger},
//	    PostToolUse: []dive.PostToolUseHook{metricsRecorder},
//	    CanUseTool: customPermissionCheck,
//	}
type PermissionConfig struct {
	// Mode determines the overall permission behavior when no rules match.
	// See PermissionMode constants for available modes.
	// Default: PermissionModeDefault (standard permission checks).
	Mode PermissionMode `json:"mode" yaml:"mode"`

	// Rules are declarative permission rules evaluated in order.
	// During evaluation, rules are separated by type and checked in this order:
	//   1. Deny rules (first matching deny rule blocks the tool)
	//   2. Allow rules (first matching allow rule permits the tool)
	//   3. Ask rules (first matching ask rule prompts the user)
	// If no rules match, evaluation continues to the Mode check.
	Rules PermissionRules `json:"rules" yaml:"rules"`

	// CanUseTool is a callback invoked after rules and mode checks.
	// Use this for dynamic permission logic that can't be expressed as static rules.
	// If nil, and no other checks have made a decision, the flow defaults to ask.
	// This field is not serializable.
	CanUseTool CanUseToolFunc `json:"-" yaml:"-"`

	// PreToolUse hooks are called before tool execution, in order.
	// Each hook can return allow/deny/ask to short-circuit the flow,
	// or continue to pass to the next hook.
	// Use for logging, auditing, rate limiting, or custom permission checks.
	// This field is not serializable.
	PreToolUse []PreToolUseHook `json:"-" yaml:"-"`

	// PostToolUse hooks are called after tool execution, in order.
	// These are purely observational and cannot affect the tool result.
	// Use for logging, metrics, cleanup, or notifications.
	// This field is not serializable.
	PostToolUse []PostToolUseHook `json:"-" yaml:"-"`
}

// PermissionManager orchestrates the permission evaluation flow for tool calls.
// It evaluates PreToolUse hooks, permission rules, mode checks, and the CanUseTool
// callback in order, returning the first definitive action (allow, deny, or ask).
//
// The manager is created with a PermissionConfig and a confirmer function.
// The confirmer is invoked when the flow results in a ToolHookAsk action.
//
// Thread Safety: PermissionManager is safe for concurrent use. The Mode can be
// changed dynamically using SetMode, which affects subsequent evaluations.
type PermissionManager struct {
	config    *PermissionConfig
	confirmer ConfirmToolFunc
}

// NewPermissionManager creates a new permission manager with the given configuration.
//
// Parameters:
//   - config: The permission configuration. If nil, defaults to PermissionModeDefault.
//   - confirmer: A callback function invoked when user confirmation is needed.
//     If nil, all ask actions will default to allow.
//
// Example:
//
//	pm := dive.NewPermissionManager(
//	    &dive.PermissionConfig{Mode: dive.PermissionModeAcceptEdits},
//	    func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, msg string) (bool, error) {
//	        return promptUser(msg), nil
//	    },
//	)
func NewPermissionManager(config *PermissionConfig, confirmer ConfirmToolFunc) *PermissionManager {
	if config == nil {
		config = &PermissionConfig{Mode: PermissionModeDefault}
	}
	return &PermissionManager{
		config:    config,
		confirmer: confirmer,
	}
}

// EvaluateToolUse runs the full permission evaluation flow and returns a decision.
//
// The evaluation proceeds through these steps in order:
//
//  1. PreToolUse Hooks - Each hook can return allow/deny/ask to short-circuit
//  2. Deny Rules - First matching deny rule blocks the tool
//  3. Allow Rules - First matching allow rule permits the tool
//  4. Ask Rules - First matching ask rule prompts the user
//  5. Permission Mode Check - Mode-specific logic (bypass, plan, acceptEdits)
//  6. CanUseTool Callback - Final programmatic decision point
//  7. Default to Ask - If nothing else decided, prompt the user
//
// At each step, returning ToolHookAllow, ToolHookDeny, or ToolHookAsk terminates
// the flow. Only ToolHookContinue (or nil) passes control to the next step.
//
// Returns:
//   - *ToolHookResult: The permission decision (allow, deny, or ask)
//   - error: Any error from hooks or callbacks (terminates evaluation)
func (pm *PermissionManager) EvaluateToolUse(
	ctx context.Context,
	tool Tool,
	call *llm.ToolUseContent,
	agent Agent,
) (*ToolHookResult, error) {
	// Step 1: Run PreToolUse hooks
	hookCtx := &PreToolUseContext{
		Tool:  tool,
		Call:  call,
		Agent: agent,
	}
	for _, hook := range pm.config.PreToolUse {
		result, err := hook(ctx, hookCtx)
		if err != nil {
			return nil, err
		}
		if result != nil && result.Action != ToolHookContinue {
			return result, nil
		}
	}

	// Step 2-4: Evaluate rules (deny → allow → ask)
	result := pm.evaluateRules(tool, call)
	if result != nil {
		return result, nil
	}

	// Step 5: Check permission mode
	result = pm.evaluateMode(tool, call)
	if result != nil && result.Action != ToolHookContinue {
		return result, nil
	}

	// Step 6: Call CanUseTool callback
	if pm.config.CanUseTool != nil {
		result, err := pm.config.CanUseTool(ctx, tool, call)
		if err != nil {
			return nil, err
		}
		if result != nil && result.Action != ToolHookContinue {
			return result, nil
		}
	}

	// Step 7: Default to ask
	return AskResult(""), nil
}

// evaluateRules checks deny, allow, and ask rules in order.
func (pm *PermissionManager) evaluateRules(tool Tool, call *llm.ToolUseContent) *ToolHookResult {
	// Separate rules by type and evaluate in order: deny → allow → ask
	var denyRules, allowRules, askRules PermissionRules
	for _, rule := range pm.config.Rules {
		switch rule.Type {
		case PermissionRuleDeny:
			denyRules = append(denyRules, rule)
		case PermissionRuleAllow:
			allowRules = append(allowRules, rule)
		case PermissionRuleAsk:
			askRules = append(askRules, rule)
		}
	}

	// Check deny rules first
	if result := denyRules.Evaluate(tool, call); result != nil {
		return result
	}

	// Check allow rules
	if result := allowRules.Evaluate(tool, call); result != nil {
		return result
	}

	// Check ask rules
	if result := askRules.Evaluate(tool, call); result != nil {
		return result
	}

	return nil
}

// evaluateMode applies permission mode logic.
func (pm *PermissionManager) evaluateMode(tool Tool, call *llm.ToolUseContent) *ToolHookResult {
	switch pm.config.Mode {
	case PermissionModeBypassPermissions:
		return AllowResult()

	case PermissionModePlan:
		// Only allow read-only tools
		if tool != nil {
			annotations := tool.Annotations()
			if annotations != nil && annotations.ReadOnlyHint {
				return AllowResult()
			}
		}
		return DenyResult("Only read-only tools are allowed in plan mode")

	case PermissionModeAcceptEdits:
		// Auto-accept edit operations
		if pm.isEditOperation(tool, call) {
			return AllowResult()
		}
		return ContinueResult()

	default:
		return ContinueResult()
	}
}

// isEditOperation checks if a tool call is an edit operation.
// This matches Anthropic's acceptEdits mode behavior.
func (pm *PermissionManager) isEditOperation(tool Tool, call *llm.ToolUseContent) bool {
	if tool == nil {
		return false
	}

	// Check EditHint annotation
	annotations := tool.Annotations()
	if annotations != nil && annotations.EditHint {
		return true
	}

	// Check tool name patterns
	toolName := strings.ToLower(tool.Name())
	editPatterns := []string{"edit", "write", "create", "mkdir", "touch", "mv", "cp", "rm"}
	for _, pattern := range editPatterns {
		if strings.Contains(toolName, pattern) {
			return true
		}
	}

	// For bash-like tools, check command content
	if pm.isBashTool(toolName) {
		return pm.isEditCommand(call)
	}

	return false
}

// isBashTool checks if a tool is a bash/command execution tool.
func (pm *PermissionManager) isBashTool(toolName string) bool {
	bashNames := []string{"bash", "command", "shell", "exec", "run"}
	for _, name := range bashNames {
		if strings.Contains(toolName, name) {
			return true
		}
	}
	return false
}

// isEditCommand checks if a bash command is an edit operation.
func (pm *PermissionManager) isEditCommand(call *llm.ToolUseContent) bool {
	if call == nil || call.Input == nil {
		return false
	}

	// Extract command from input
	command := extractCommand(call.Input)
	if command == "" {
		return false
	}

	// Check for filesystem modification commands
	editCommands := []string{
		"mkdir", "touch", "rm", "rmdir", "mv", "cp",
		"cat >", "echo >", "tee", "sed -i", "chmod", "chown",
	}
	commandLower := strings.ToLower(command)
	for _, editCmd := range editCommands {
		if strings.HasPrefix(commandLower, editCmd) ||
			strings.Contains(commandLower, " "+editCmd) {
			return true
		}
	}

	return false
}

// RunPostToolUseHooks runs all configured PostToolUse hooks in order.
// These hooks are called after tool execution and are purely observational -
// they cannot modify the tool result.
//
// If any hook returns an error, execution stops and the error is returned.
// However, in the agent's tool execution flow, PostToolUse errors are logged
// but don't affect the response sent to the LLM.
//
// Use cases:
//   - Logging tool calls for audit trails
//   - Recording metrics and analytics
//   - Sending notifications
//   - Cleanup operations
func (pm *PermissionManager) RunPostToolUseHooks(
	ctx context.Context,
	hookCtx *PostToolUseContext,
) error {
	for _, hook := range pm.config.PostToolUse {
		if err := hook(ctx, hookCtx); err != nil {
			return err
		}
	}
	return nil
}

// Confirm prompts the user for tool confirmation using the configured confirmer.
// This is called when the permission flow results in a ToolHookAsk action.
//
// Returns:
//   - true: User approved the tool execution
//   - false: User denied the tool execution
//
// If no confirmer was configured in NewPermissionManager, this defaults to
// returning true (auto-approve).
func (pm *PermissionManager) Confirm(
	ctx context.Context,
	tool Tool,
	call *llm.ToolUseContent,
	message string,
) (bool, error) {
	if pm.confirmer == nil {
		// No confirmer configured - default to allow
		return true, nil
	}
	return pm.confirmer(ctx, tool, call, message)
}

// Mode returns the current permission mode.
func (pm *PermissionManager) Mode() PermissionMode {
	return pm.config.Mode
}

// SetMode updates the permission mode dynamically.
// This allows changing permission behavior during a conversation, for example:
//   - Start in PermissionModeDefault for careful review
//   - Switch to PermissionModeAcceptEdits after establishing trust
//   - Use PermissionModePlan to restrict to read-only operations
//
// The new mode takes effect for all subsequent EvaluateToolUse calls.
func (pm *PermissionManager) SetMode(mode PermissionMode) {
	pm.config.Mode = mode
}

// AddRules appends additional permission rules to the configuration.
// Rules are evaluated in order, with deny rules checked first across all rules,
// then allow rules, then ask rules.
//
// This is useful for adding rules from a settings file or dynamically
// during a session (e.g., when user selects "allow all X this session").
//
// Example:
//
//	pm.AddRules(dive.PermissionRules{
//	    dive.AllowRule("read_*"),
//	    dive.AllowCommandRule("bash", "go test*"),
//	})
func (pm *PermissionManager) AddRules(rules PermissionRules) {
	pm.config.Rules = append(pm.config.Rules, rules...)
}

// Rules returns the current permission rules.
func (pm *PermissionManager) Rules() PermissionRules {
	return pm.config.Rules
}

// extractCommand extracts the command string from tool input.
func extractCommand(input []byte) string {
	var inputMap map[string]any
	if err := json.Unmarshal(input, &inputMap); err != nil {
		return ""
	}

	// Look for command in common field names
	commandFields := []string{"command", "cmd", "script", "code"}
	for _, field := range commandFields {
		if cmd, ok := inputMap[field].(string); ok {
			return cmd
		}
	}
	return ""
}
