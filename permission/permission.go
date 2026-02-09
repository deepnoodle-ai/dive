// Package permission provides tool permission management for Dive agents.
//
// This package implements permission checking as a PreToolUse hook,
// including rule-based evaluation, session allowlists, and user confirmation.
//
// Example:
//
//	config := &permission.Config{
//	    Mode: permission.ModeDefault,
//	    Rules: permission.Rules{
//	        permission.AllowRule("Read"),
//	        permission.AskRule("Bash", "Execute command?"),
//	    },
//	}
//	preToolHook := permission.Hook(config, &dive.AutoApproveDialog{})
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    Model: model,
//	    Hooks: dive.Hooks{
//	        PreToolUse: []dive.PreToolUseHook{preToolHook},
//	    },
//	})
package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

// Mode determines the global permission behavior.
type Mode string

const (
	// ModeDefault applies standard permission checks based on rules.
	ModeDefault Mode = "default"

	// ModePlan restricts the agent to read-only operations.
	ModePlan Mode = "plan"

	// ModeAcceptEdits auto-accepts file edit operations.
	ModeAcceptEdits Mode = "acceptEdits"

	// ModeBypassPermissions allows ALL tools without prompts.
	ModeBypassPermissions Mode = "bypassPermissions"

	// ModeDontAsk auto-denies any tool call that is not explicitly allowed
	// by a rule. This is useful for headless/automation use cases.
	ModeDontAsk Mode = "dontAsk"
)

// RuleType indicates what action a rule takes when it matches.
type RuleType string

const (
	RuleDeny  RuleType = "deny"
	RuleAllow RuleType = "allow"
	RuleAsk   RuleType = "ask"
)

// Rule defines a declarative permission rule.
type Rule struct {
	Type      RuleType
	Tool      string
	Specifier string
	Message   string

	// InputMatch is an optional custom matcher for tool input.
	InputMatch func(input any) bool
}

// String returns a human-readable representation like "allow:Bash(go test *)".
func (r Rule) String() string {
	s := string(r.Type) + ":" + r.Tool
	if r.Specifier != "" {
		s += "(" + r.Specifier + ")"
	}
	return s
}

// Rules is an ordered list of permission rules.
type Rules []Rule

// SpecifierFieldFunc extracts the specifier value from a tool call's input.
// The input is the raw JSON input from the tool call.
type SpecifierFieldFunc func(input json.RawMessage) string

// Config contains all permission-related configuration.
type Config struct {
	Mode  Mode
	Rules Rules

	// SpecifierFields maps tool names to functions that extract the specifier
	// value from tool call input. If not set, DefaultSpecifierFields is used.
	SpecifierFields map[string]SpecifierFieldFunc
}

// Manager orchestrates the permission evaluation flow.
type Manager struct {
	mu             sync.RWMutex
	config         *Config
	dialog         dive.Dialog
	sessionAllowed map[string]bool
}

// NewManager creates a new permission manager.
func NewManager(config *Config, dialog dive.Dialog) *Manager {
	if config == nil {
		config = &Config{Mode: ModeDefault}
	}
	return &Manager{
		config:         config,
		dialog:         dialog,
		sessionAllowed: make(map[string]bool),
	}
}

// Internal decision type used between evaluateRules/evaluateMode and EvaluateToolUse.
type decision int

const (
	noDecision decision = iota
	allow
	deny
	askUser
)

// EvaluateToolUse runs the full permission evaluation flow.
// Returns nil if the tool is allowed, or an error if denied.
func (pm *Manager) EvaluateToolUse(
	ctx context.Context,
	tool dive.Tool,
	call *llm.ToolUseContent,
) error {
	// Check session allowlist
	if tool != nil {
		category := GetToolCategory(tool.Name())
		pm.mu.RLock()
		allowed := pm.sessionAllowed[category.Key]
		pm.mu.RUnlock()
		if allowed {
			return nil
		}
	}

	// Evaluate rules
	d, msg := pm.evaluateRules(tool, call)
	switch d {
	case deny:
		return fmt.Errorf("%s", msg)
	case allow:
		return nil
	case askUser:
		return pm.confirm(ctx, tool, call, msg)
	}

	// Check permission mode
	d, msg = pm.evaluateMode(tool, call)
	switch d {
	case deny:
		return fmt.Errorf("%s", msg)
	case allow:
		return nil
	}

	// Default: ask for confirmation
	return pm.confirm(ctx, tool, call, "")
}

func (pm *Manager) evaluateRules(tool dive.Tool, call *llm.ToolUseContent) (decision, string) {
	if tool == nil || call == nil {
		return noDecision, ""
	}

	pm.mu.RLock()
	var denyRules, allowRules, askRules Rules
	for _, rule := range pm.config.Rules {
		switch rule.Type {
		case RuleDeny:
			denyRules = append(denyRules, rule)
		case RuleAllow:
			allowRules = append(allowRules, rule)
		case RuleAsk:
			askRules = append(askRules, rule)
		}
	}
	pm.mu.RUnlock()

	toolName := tool.Name()

	// Check deny rules first
	for _, rule := range denyRules {
		if pm.matchRule(rule, toolName, call) {
			return deny, rule.Message
		}
	}

	// Check allow rules
	for _, rule := range allowRules {
		if pm.matchRule(rule, toolName, call) {
			return allow, ""
		}
	}

	// Check ask rules
	for _, rule := range askRules {
		if pm.matchRule(rule, toolName, call) {
			return askUser, rule.Message
		}
	}

	return noDecision, ""
}

func (pm *Manager) matchRule(rule Rule, toolName string, call *llm.ToolUseContent) bool {
	// Match tool pattern using glob
	if !MatchGlob(rule.Tool, toolName) {
		return false
	}

	// Match specifier pattern if specified
	if rule.Specifier != "" {
		specifier := pm.extractSpecifier(toolName, call.Input)
		if specifier == "" || !MatchGlob(rule.Specifier, specifier) {
			return false
		}
	}

	// Match input if specified
	if rule.InputMatch != nil {
		var input any
		if err := json.Unmarshal(call.Input, &input); err != nil {
			return false
		}
		if !rule.InputMatch(input) {
			return false
		}
	}

	return true
}

func (pm *Manager) extractSpecifier(toolName string, input json.RawMessage) string {
	pm.mu.RLock()
	specFields := pm.config.SpecifierFields
	pm.mu.RUnlock()

	// Check user-configured specifier fields first
	if specFields != nil {
		if fn, ok := specFields[toolName]; ok {
			return fn(input)
		}
	}

	// Fall back to defaults
	if fn, ok := DefaultSpecifierFields[toolName]; ok {
		return fn(input)
	}
	return ""
}

func (pm *Manager) evaluateMode(tool dive.Tool, call *llm.ToolUseContent) (decision, string) {
	pm.mu.RLock()
	mode := pm.config.Mode
	pm.mu.RUnlock()

	switch mode {
	case ModeBypassPermissions:
		return allow, ""

	case ModePlan:
		if tool != nil {
			annotations := tool.Annotations()
			if annotations != nil && annotations.ReadOnlyHint {
				return allow, ""
			}
		}
		return deny, "only read-only tools are allowed in plan mode"

	case ModeAcceptEdits:
		if pm.isEditOperation(tool, call) {
			return allow, ""
		}
		return noDecision, ""

	case ModeDontAsk:
		return deny, "tool not explicitly allowed (dontAsk mode)"

	default:
		return noDecision, ""
	}
}

func (pm *Manager) isEditOperation(tool dive.Tool, _ *llm.ToolUseContent) bool {
	if tool == nil {
		return false
	}

	annotations := tool.Annotations()
	if annotations != nil && annotations.EditHint {
		return true
	}

	toolName := tool.Name()
	editNames := []string{"Edit", "Write", "Create", "Mkdir", "Touch"}
	for _, name := range editNames {
		if MatchGlob(name, toolName) || MatchGlob("*"+name+"*", toolName) {
			return true
		}
	}
	return false
}

// confirm prompts the user for tool confirmation.
// Returns nil if approved, error if denied.
func (pm *Manager) confirm(
	ctx context.Context,
	tool dive.Tool,
	call *llm.ToolUseContent,
	message string,
) error {
	if pm.dialog == nil {
		return nil // no dialog = auto-allow
	}
	output, err := pm.dialog.Show(ctx, &dive.DialogInput{
		Confirm: true,
		Title:   tool.Name(),
		Message: message,
		Tool:    tool,
		Call:    call,
	})
	if err != nil {
		return err
	}
	if output.AllowSession {
		category := GetToolCategory(tool.Name())
		pm.AllowForSession(category.Key)
		return nil
	}
	if output.Feedback != "" {
		return dive.NewUserFeedback(output.Feedback)
	}
	if output.Canceled || !output.Confirmed {
		return fmt.Errorf("user denied tool call")
	}
	return nil
}

// Mode returns the current permission mode.
func (pm *Manager) Mode() Mode {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.config.Mode
}

// SetMode updates the permission mode dynamically.
func (pm *Manager) SetMode(mode Mode) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.config.Mode = mode
}

// AllowForSession marks a tool category as allowed for this session.
func (pm *Manager) AllowForSession(categoryKey string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessionAllowed[categoryKey] = true
}

// IsSessionAllowed checks if a tool category is allowed for this session.
func (pm *Manager) IsSessionAllowed(categoryKey string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.sessionAllowed[categoryKey]
}

// ClearSessionAllowlist removes all session-scoped allowlist entries.
func (pm *Manager) ClearSessionAllowlist() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessionAllowed = make(map[string]bool)
}

// Category represents a tool's category for session allowlist purposes.
type Category struct {
	Key   string
	Label string
}

// Common tool categories.
var (
	CategoryBash   = Category{Key: "bash", Label: "bash commands"}
	CategoryEdit   = Category{Key: "edit", Label: "file edits"}
	CategoryRead   = Category{Key: "read", Label: "file reads"}
	CategorySearch = Category{Key: "search", Label: "searches"}
)

// GetToolCategory determines the category of a tool based on its name.
func GetToolCategory(toolName string) Category {
	if MatchGlob("*{Bash,Command,Shell,Exec,Run}*", toolName) {
		return CategoryBash
	}
	if MatchGlob("*{Edit,Write,Create,Mkdir,Touch}*", toolName) {
		return CategoryEdit
	}
	if MatchGlob("*Read*", toolName) {
		return CategoryRead
	}
	if MatchGlob("*{Glob,Grep,Search}*", toolName) {
		return CategorySearch
	}
	return Category{Key: toolName, Label: toolName + " operations"}
}

// DefaultSpecifierFields maps tool names to functions that extract the
// specifier value from the tool call input. These are used when
// Config.SpecifierFields does not have an entry for the tool.
var DefaultSpecifierFields = map[string]SpecifierFieldFunc{
	"Bash": func(input json.RawMessage) string {
		return jsonStringField(input, "command", "cmd", "script", "code")
	},
	"Read": func(input json.RawMessage) string {
		return jsonStringField(input, "file_path", "filePath", "path")
	},
	"Write": func(input json.RawMessage) string {
		return jsonStringField(input, "file_path", "filePath", "path")
	},
	"Edit": func(input json.RawMessage) string {
		return jsonStringField(input, "file_path", "filePath", "path")
	},
	"WebFetch": func(input json.RawMessage) string {
		return jsonStringField(input, "url")
	},
}

// jsonStringField extracts the first non-empty string value from the given
// JSON object for the specified field names.
func jsonStringField(input json.RawMessage, fields ...string) string {
	var m map[string]any
	if err := json.Unmarshal(input, &m); err != nil {
		return ""
	}
	for _, field := range fields {
		if v, ok := m[field].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// Helper functions to create rules.

// DenyRule creates a deny rule for a tool pattern.
func DenyRule(toolPattern string, message string) Rule {
	return Rule{Type: RuleDeny, Tool: toolPattern, Message: message}
}

// AllowRule creates an allow rule for a tool pattern.
func AllowRule(toolPattern string) Rule {
	return Rule{Type: RuleAllow, Tool: toolPattern}
}

// AskRule creates an ask rule for a tool pattern.
func AskRule(toolPattern string, message string) Rule {
	return Rule{Type: RuleAsk, Tool: toolPattern, Message: message}
}

// DenySpecifierRule creates a deny rule for a tool with a specifier pattern.
func DenySpecifierRule(toolPattern, specifierPattern, message string) Rule {
	return Rule{Type: RuleDeny, Tool: toolPattern, Specifier: specifierPattern, Message: message}
}

// AllowSpecifierRule creates an allow rule for a tool with a specifier pattern.
func AllowSpecifierRule(toolPattern, specifierPattern string) Rule {
	return Rule{Type: RuleAllow, Tool: toolPattern, Specifier: specifierPattern}
}

// AskSpecifierRule creates an ask rule for a tool with a specifier pattern.
func AskSpecifierRule(toolPattern, specifierPattern, message string) Rule {
	return Rule{Type: RuleAsk, Tool: toolPattern, Specifier: specifierPattern, Message: message}
}
