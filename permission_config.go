package dive

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/deepnoodle-ai/dive/llm"
)

// Permission Configuration and Manager
//
// This file implements the PermissionConfig and PermissionManager types that
// orchestrate the permission evaluation flow for tool calls.
//
// The PermissionManager implements the following permission flow:
//
//	PreToolUse Hooks → Session Allowlist → Deny Rules → Allow Rules → Ask Rules → Mode Check → CanUseTool → Confirm
//
// Each step can terminate the flow early by returning allow, deny, or ask.
// Only ToolHookContinue passes control to the next step.
//
// Key features:
//   - Session Allowlists: Users can approve "allow all X this session" to skip
//     future confirmations for a tool category (bash, edit, read, search, etc.)
//   - Tool Categories: Tools are automatically categorized based on their names,
//     enabling consistent grouping for session allowlists
//   - Category Tracking: The Category field in ToolHookResult indicates which
//     category matched, allowing UIs to offer "allow all" options

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

// ToolCategory represents a tool's category for session allowlist purposes.
// Categories group similar tools together (e.g., all bash-like tools, all file editors),
// enabling "allow all X this session" functionality in UIs.
//
// When a user approves a tool and selects "allow all [category] this session",
// the category Key is stored in the session allowlist. Subsequent tool calls
// with the same category are auto-approved.
//
// The Category field in [ToolHookResult] indicates which category matched,
// allowing UIs to display appropriate "allow all" options.
type ToolCategory struct {
	// Key is the internal category identifier used for matching.
	// Standard keys include: "bash", "edit", "read", "search".
	// For unrecognized tools, the tool name itself becomes the key.
	Key string

	// Label is the human-readable description displayed in UIs.
	// For example: "bash commands", "file edits", "file reads".
	Label string
}

// Common tool categories used by the permission system.
// These predefined categories cover the most common tool types.
// Use these when calling [PermissionManager.AllowCategoryForSession].
//
// Example:
//
//	pm.AllowCategoryForSession(dive.ToolCategoryBash)
//	pm.AllowCategoryForSession(dive.ToolCategoryEdit)
var (
	// ToolCategoryBash matches command execution tools (bash, shell, exec, run, command).
	ToolCategoryBash = ToolCategory{Key: "bash", Label: "bash commands"}

	// ToolCategoryEdit matches file modification tools (edit, write, create, mkdir, touch).
	ToolCategoryEdit = ToolCategory{Key: "edit", Label: "file edits"}

	// ToolCategoryRead matches file reading tools (read).
	ToolCategoryRead = ToolCategory{Key: "read", Label: "file reads"}

	// ToolCategorySearch matches search and discovery tools (glob, grep, search).
	ToolCategorySearch = ToolCategory{Key: "search", Label: "searches"}
)

// GetToolCategory determines the category of a tool based on its name.
// This centralizes tool categorization logic for consistent behavior across the library
// and CLI implementations.
//
// Categorization rules (checked in order):
//  1. Bash tools: name contains "bash", "command", "shell", "exec", or "run"
//  2. Edit tools: name contains "edit", "write", "create", "mkdir", or "touch"
//  3. Read tools: name contains "read"
//  4. Search tools: name contains "glob", "grep", or "search"
//  5. Default: tool name becomes its own category
//
// The matching is case-insensitive.
//
// Example:
//
//	cat := dive.GetToolCategory("Bash")     // Returns ToolCategoryBash
//	cat := dive.GetToolCategory("read_file") // Returns ToolCategoryRead
//	cat := dive.GetToolCategory("custom")    // Returns {Key: "custom", Label: "custom operations"}
func GetToolCategory(toolName string) ToolCategory {
	toolNameLower := strings.ToLower(toolName)

	// Bash/command execution tools
	bashPatterns := []string{"bash", "command", "shell", "exec", "run"}
	for _, pattern := range bashPatterns {
		if strings.Contains(toolNameLower, pattern) {
			return ToolCategoryBash
		}
	}

	// Edit/write tools
	editPatterns := []string{"edit", "write", "create", "mkdir", "touch"}
	for _, pattern := range editPatterns {
		if strings.Contains(toolNameLower, pattern) {
			return ToolCategoryEdit
		}
	}

	// Read tools
	if strings.Contains(toolNameLower, "read") {
		return ToolCategoryRead
	}

	// Glob/search tools
	if strings.Contains(toolNameLower, "glob") || strings.Contains(toolNameLower, "grep") || strings.Contains(toolNameLower, "search") {
		return ToolCategorySearch
	}

	// Default: use the tool name itself as the category
	return ToolCategory{Key: toolName, Label: toolName + " operations"}
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
// All methods use internal synchronization to protect concurrent access.
//
// Session Allowlists: The manager maintains session-scoped allowlists that persist
// for the lifetime of the manager. When a user approves "allow all X this session",
// subsequent calls for that category are automatically allowed.
type PermissionManager struct {
	mu             sync.RWMutex
	config         *PermissionConfig
	confirmer      ConfirmToolFunc
	sessionAllowed map[string]bool // Maps category keys to allowed status
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
		config:         config,
		confirmer:      confirmer,
		sessionAllowed: make(map[string]bool),
	}
}

// EvaluateToolUse runs the full permission evaluation flow and returns a decision.
//
// The evaluation proceeds through these steps in order:
//
//  1. PreToolUse Hooks - Each hook can return allow/deny/ask to short-circuit
//  2. Session Allowlist - Check if category is allowed for this session
//  3. Deny Rules - First matching deny rule blocks the tool
//  4. Allow Rules - First matching allow rule permits the tool
//  5. Ask Rules - First matching ask rule prompts the user
//  6. Permission Mode Check - Mode-specific logic (bypass, plan, acceptEdits)
//  7. CanUseTool Callback - Final programmatic decision point
//  8. Default to Ask - If nothing else decided, prompt the user
//
// At each step, returning ToolHookAllow, ToolHookDeny, or ToolHookAsk terminates
// the flow. Only ToolHookContinue (or nil) passes control to the next step.
//
// Returns:
//   - *ToolHookResult: The permission decision (allow, deny, or ask).
//     The Category field is populated when the decision was based on
//     session allowlists or category-based rules.
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

	// Step 2: Check session allowlist
	if tool != nil {
		category := GetToolCategory(tool.Name())
		pm.mu.RLock()
		allowed := pm.sessionAllowed[category.Key]
		pm.mu.RUnlock()
		if allowed {
			result := AllowResult()
			result.Category = &category
			return result, nil
		}
	}

	// Step 3-5: Evaluate rules (deny → allow → ask)
	result := pm.evaluateRules(tool, call)
	if result != nil {
		return result, nil
	}

	// Step 6: Check permission mode
	result = pm.evaluateMode(tool, call)
	if result != nil && result.Action != ToolHookContinue {
		return result, nil
	}

	// Step 7: Call CanUseTool callback
	if pm.config.CanUseTool != nil {
		result, err := pm.config.CanUseTool(ctx, tool, call)
		if err != nil {
			return nil, err
		}
		if result != nil && result.Action != ToolHookContinue {
			return result, nil
		}
	}

	// Step 8: Default to ask with category info
	askResult := AskResult("")
	if tool != nil {
		category := GetToolCategory(tool.Name())
		askResult.Category = &category
	}
	return askResult, nil
}

// evaluateRules checks deny, allow, and ask rules in order.
func (pm *PermissionManager) evaluateRules(tool Tool, call *llm.ToolUseContent) *ToolHookResult {
	// Separate rules by type and evaluate in order: deny → allow → ask
	// Copy rules under lock to avoid race with AddRules
	pm.mu.RLock()
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
	pm.mu.RUnlock()

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
	pm.mu.RLock()
	mode := pm.config.Mode
	pm.mu.RUnlock()

	switch mode {
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
	toolNameLower := strings.ToLower(toolName)
	for _, name := range bashNames {
		if strings.Contains(toolNameLower, name) {
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
	pm.mu.Lock()
	defer pm.mu.Unlock()
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
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.config.Rules = append(pm.config.Rules, rules...)
}

// Rules returns a copy of the current permission rules.
func (pm *PermissionManager) Rules() PermissionRules {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	// Return a copy to prevent external mutation
	result := make(PermissionRules, len(pm.config.Rules))
	copy(result, pm.config.Rules)
	return result
}

// AllowForSession marks a tool category as allowed for the remainder of this session.
// When a category is allowed, all tools in that category will be auto-approved
// by [PermissionManager.EvaluateToolUse] without prompting the user.
//
// This is typically called when a user selects "allow all X this session" in a
// confirmation dialog. The category key should match the Key field from [ToolCategory].
//
// Session allowlists are checked early in the permission flow (step 2), after
// PreToolUse hooks but before rules. This means session allowlists can override
// deny rules that would otherwise block a tool.
//
// Thread-safe: can be called concurrently with [EvaluateToolUse].
//
// Example:
//
//	// Using category keys directly
//	pm.AllowForSession("bash")  // Allow all bash commands this session
//	pm.AllowForSession("edit")  // Allow all file edits this session
//
//	// From a tool call result
//	result, _ := pm.EvaluateToolUse(ctx, tool, call, agent)
//	if result.Action == dive.ToolHookAsk && userApprovedAll {
//	    pm.AllowForSession(result.Category.Key)
//	}
func (pm *PermissionManager) AllowForSession(categoryKey string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessionAllowed[categoryKey] = true
}

// AllowCategoryForSession is a convenience method that takes a [ToolCategory]
// and allows it for the session. This is equivalent to calling
// [PermissionManager.AllowForSession] with category.Key.
//
// Use this when you have a ToolCategory struct (e.g., from [GetToolCategory]
// or one of the predefined categories like [ToolCategoryBash]).
//
// Example:
//
//	pm.AllowCategoryForSession(dive.ToolCategoryBash)
//	pm.AllowCategoryForSession(dive.GetToolCategory(tool.Name()))
func (pm *PermissionManager) AllowCategoryForSession(category ToolCategory) {
	pm.AllowForSession(category.Key)
}

// IsSessionAllowed checks if a tool category is allowed for this session.
// Returns true if [AllowForSession] was previously called with this category key.
//
// Thread-safe: can be called concurrently with other methods.
//
// Example:
//
//	if pm.IsSessionAllowed("bash") {
//	    // Skip confirmation dialog
//	}
func (pm *PermissionManager) IsSessionAllowed(categoryKey string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.sessionAllowed[categoryKey]
}

// SessionAllowedCategories returns a list of all category keys currently allowed
// for this session. The returned slice is a copy and can be safely modified.
//
// This is useful for displaying which categories have been allowed in a UI,
// or for persisting session state.
//
// Example:
//
//	allowed := pm.SessionAllowedCategories()
//	for _, key := range allowed {
//	    fmt.Printf("Allowed: %s\n", key)
//	}
func (pm *PermissionManager) SessionAllowedCategories() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]string, 0, len(pm.sessionAllowed))
	for key, allowed := range pm.sessionAllowed {
		if allowed {
			result = append(result, key)
		}
	}
	return result
}

// ClearSessionAllowlist removes all session-scoped allowlist entries.
// After calling this, all tools will require their normal permission checks again.
//
// This is useful for resetting permissions at the start of a new task or when
// the user wants to re-enable confirmations for previously allowed categories.
//
// Thread-safe: can be called concurrently with other methods.
func (pm *PermissionManager) ClearSessionAllowlist() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessionAllowed = make(map[string]bool)
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
