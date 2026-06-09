// Package permission provides tool permission management for Dive agents.
//
// This package implements permission checking as a PreToolUse hook,
// including rule-based evaluation, session allowlists, and user confirmation.
//
// Evaluation order: deny rules, session allowlist, allow rules, ask rules,
// permission mode, then the default confirmation dialog. Deny rules are
// absolute: they cannot be bypassed by session grants or by any mode,
// including ModeBypassPermissions.
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
	"net/url"
	"path/filepath"
	"strings"
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

// SpecifierMatcherFunc matches a rule's specifier pattern against the value
// extracted from a tool call. The rule type is provided because safe matching
// is direction-dependent: deny matching should be generous (catch evasions)
// while allow matching should be strict (reject anything not clearly covered).
type SpecifierMatcherFunc func(ruleType RuleType, pattern, value string) bool

// Config contains all permission-related configuration.
type Config struct {
	Mode  Mode
	Rules Rules

	// SpecifierFields maps tool names to functions that extract the specifier
	// value from tool call input. If not set, DefaultSpecifierFields is used.
	SpecifierFields map[string]SpecifierFieldFunc

	// SpecifierMatchers maps tool names to functions that match a rule's
	// specifier pattern against the extracted value. If not set,
	// DefaultSpecifierMatchers is used (command-aware matching for Bash,
	// segment-aware path matching for Read/Write/Edit, domain-aware matching
	// for WebFetch). Tools with no matcher fall back to MatchGlob.
	SpecifierMatchers map[string]SpecifierMatcherFunc
}

// sessionGrant is a session-scoped approval for a specific tool, optionally
// narrowed to a specifier. Grants created from dialog approvals are exact
// (the literal specifier value, compared by equality so glob characters in
// the approved input cannot widen the grant); grants created via
// AllowToolForSession are patterns matched with the tool's specifier matcher.
type sessionGrant struct {
	tool      string
	specifier string // empty means the entire tool
	exact     bool
}

// Manager orchestrates the permission evaluation flow.
type Manager struct {
	mu             sync.RWMutex
	config         *Config
	dialog         dive.Dialog
	sessionAllowed map[string]bool
	sessionGrants  []sessionGrant
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

// Internal decision type used between evaluateMode and EvaluateToolUse.
type decision int

const (
	noDecision decision = iota
	allow
	deny
)

// EvaluateToolUse runs the full permission evaluation flow.
// Returns nil if the tool is allowed, or an error if denied.
//
// Evaluation order: deny rules, session allowlist, allow rules, ask rules,
// permission mode, then the default confirmation dialog. Deny rules are
// absolute — they beat session grants and every mode, including
// ModeBypassPermissions.
func (pm *Manager) EvaluateToolUse(
	ctx context.Context,
	tool dive.Tool,
	call *llm.ToolUseContent,
) error {
	denyRules, allowRules, askRules := pm.partitionRules()

	var toolName string
	if tool != nil {
		toolName = tool.Name()
	}

	// Deny rules are evaluated first and are absolute.
	if tool != nil && call != nil {
		for _, rule := range denyRules {
			if pm.matchRule(rule, toolName, call) {
				msg := rule.Message
				if msg == "" {
					msg = "denied by rule " + rule.String()
				}
				return fmt.Errorf("%s", msg)
			}
		}
	}

	// Check session allowlist
	if tool != nil && pm.isSessionAllowed(toolName, call) {
		return nil
	}

	// Check allow and ask rules
	if tool != nil && call != nil {
		for _, rule := range allowRules {
			if pm.matchRule(rule, toolName, call) {
				return nil
			}
		}
		for _, rule := range askRules {
			if pm.matchRule(rule, toolName, call) {
				return pm.confirm(ctx, tool, call, rule.Message)
			}
		}
	}

	// Check permission mode
	d, msg := pm.evaluateMode(tool, call)
	switch d {
	case deny:
		return fmt.Errorf("%s", msg)
	case allow:
		return nil
	}

	// Default: ask for confirmation
	return pm.confirm(ctx, tool, call, "")
}

func (pm *Manager) partitionRules() (denyRules, allowRules, askRules Rules) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
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
	return denyRules, allowRules, askRules
}

func (pm *Manager) matchRule(rule Rule, toolName string, call *llm.ToolUseContent) bool {
	// Match tool pattern using glob
	if !MatchGlob(rule.Tool, toolName) {
		return false
	}

	// Match specifier pattern if specified
	if rule.Specifier != "" {
		specifier := pm.extractSpecifier(toolName, call.Input)
		if specifier == "" {
			// No specifier could be extracted. Deny rules fail closed: a
			// rule meant to block "rm -rf*" must not be evaded by an input
			// shape the extractor doesn't understand.
			return rule.Type == RuleDeny
		}
		if !pm.matchSpecifier(rule.Type, toolName, rule.Specifier, specifier) {
			return false
		}
	}

	// Match input if specified
	if rule.InputMatch != nil {
		var input any
		if err := json.Unmarshal(call.Input, &input); err != nil {
			// Unparsable input fails closed for deny rules.
			return rule.Type == RuleDeny
		}
		if !rule.InputMatch(input) {
			return false
		}
	}

	return true
}

// matchSpecifier matches a specifier pattern against an extracted value using
// the tool's configured matcher (custom, then default, then MatchGlob).
func (pm *Manager) matchSpecifier(ruleType RuleType, toolName, pattern, value string) bool {
	pm.mu.RLock()
	custom := pm.config.SpecifierMatchers
	pm.mu.RUnlock()

	if custom != nil {
		if fn, ok := custom[toolName]; ok {
			return fn(ruleType, pattern, value)
		}
	}
	if fn, ok := DefaultSpecifierMatchers[toolName]; ok {
		return fn(ruleType, pattern, value)
	}
	return MatchGlob(pattern, value)
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
		pm.grantSessionFromCall(tool, call)
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
//
// Deprecated: category grants are very broad (one approval covers every
// command-like tool, for example). Use AllowToolForSession to grant a
// specific tool, optionally narrowed to a specifier pattern. Category grants
// are still honored for backward compatibility, but dialog approvals no
// longer create them, and deny rules beat them.
func (pm *Manager) AllowForSession(categoryKey string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessionAllowed[categoryKey] = true
}

// AllowToolForSession marks a specific tool as allowed for this session.
// If specifierPattern is non-empty, the grant only covers calls whose
// extracted specifier matches the pattern (using the tool's specifier
// matcher with allow-side semantics, e.g. "git status*" for Bash). An empty
// specifierPattern grants every call to the tool. Deny rules always beat
// session grants.
func (pm *Manager) AllowToolForSession(toolName, specifierPattern string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessionGrants = append(pm.sessionGrants, sessionGrant{
		tool:      toolName,
		specifier: specifierPattern,
	})
}

// IsSessionAllowed checks if a tool category is allowed for this session.
//
// Deprecated: this reflects only legacy category grants created via
// AllowForSession, not the scoped grants created by dialog approvals or
// AllowToolForSession.
func (pm *Manager) IsSessionAllowed(categoryKey string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.sessionAllowed[categoryKey]
}

// ClearSessionAllowlist removes all session-scoped allowlist entries,
// including both scoped tool grants and legacy category grants.
func (pm *Manager) ClearSessionAllowlist() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessionAllowed = make(map[string]bool)
	pm.sessionGrants = nil
}

// isSessionAllowed reports whether a tool call is covered by a session grant.
func (pm *Manager) isSessionAllowed(toolName string, call *llm.ToolUseContent) bool {
	pm.mu.RLock()
	legacyAllowed := pm.sessionAllowed[GetToolCategory(toolName).Key]
	grants := make([]sessionGrant, len(pm.sessionGrants))
	copy(grants, pm.sessionGrants)
	pm.mu.RUnlock()

	if legacyAllowed {
		return true
	}
	if len(grants) == 0 {
		return false
	}

	var specifier string
	if call != nil {
		specifier = pm.extractSpecifier(toolName, call.Input)
	}
	for _, grant := range grants {
		if grant.tool != toolName {
			continue
		}
		if grant.specifier == "" {
			return true
		}
		if specifier == "" {
			continue
		}
		if grant.exact {
			if specifier == grant.specifier {
				return true
			}
			continue
		}
		if pm.matchSpecifier(RuleAllow, toolName, grant.specifier, specifier) {
			return true
		}
	}
	return false
}

// SessionGrantLabel returns a short human-readable description of the
// session grant that approving the given call with "allow for session" would
// create: the exact command or path, the URL's domain, or the tool name when
// no specifier can be extracted. Intended for dialog option labels like
// "Yes, allow all <label> during this session".
func (pm *Manager) SessionGrantLabel(tool dive.Tool, call *llm.ToolUseContent) string {
	if tool == nil {
		return "this action"
	}
	toolName := tool.Name()
	var specifier string
	if call != nil {
		specifier = pm.extractSpecifier(toolName, call.Input)
	}
	if specifier == "" {
		return toolName + " operations"
	}
	if host := urlHost(specifier); host != "" {
		return "fetches from " + host
	}
	return fmt.Sprintf("%q", specifier)
}

// grantSessionFromCall records a session grant scoped to the approved call.
// The grant covers the exact specifier value (command, file path) rather
// than a category, so approving "ls" does not approve "rm -rf /". URL
// specifiers are granted at domain scope, matching how users think about
// approving a fetch ("allow example.com this session").
func (pm *Manager) grantSessionFromCall(tool dive.Tool, call *llm.ToolUseContent) {
	toolName := tool.Name()
	var specifier string
	if call != nil {
		specifier = pm.extractSpecifier(toolName, call.Input)
	}

	grant := sessionGrant{tool: toolName}
	if specifier != "" {
		if host := urlHost(specifier); host != "" {
			grant.specifier = "domain:" + host
		} else {
			grant.specifier = specifier
			grant.exact = true
		}
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.sessionGrants = append(pm.sessionGrants, grant)
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

// DefaultSpecifierMatchers maps tool names to specifier matching functions.
// These are used when Config.SpecifierMatchers does not have an entry for the
// tool. Tools with no entry in either map fall back to MatchGlob.
//
//   - Bash: command-aware matching. Allow rules require every shell segment
//     to match and reject command substitution; deny rules match if the full
//     command or any segment matches. See MatchCommandAllow/MatchCommandDeny.
//   - Read/Write/Edit: paths are cleaned (so "/safe/../etc" cannot evade a
//     rule) and matched with MatchPath, where * stays within one path segment
//     and ** crosses segments. Deny rules also match the absolutized form of
//     relative paths.
//   - WebFetch: domain-aware matching via MatchURLSpecifier ("domain:x.com"
//     or a bare domain matches the host; other patterns glob the full URL).
var DefaultSpecifierMatchers = map[string]SpecifierMatcherFunc{
	"Bash":     matchCommandSpecifier,
	"Read":     matchPathSpecifier,
	"Write":    matchPathSpecifier,
	"Edit":     matchPathSpecifier,
	"WebFetch": matchURLSpecifier,
}

func matchCommandSpecifier(ruleType RuleType, pattern, command string) bool {
	if ruleType == RuleDeny {
		return MatchCommandDeny(pattern, command)
	}
	return MatchCommandAllow(pattern, command)
}

func matchPathSpecifier(ruleType RuleType, pattern, path string) bool {
	cleaned := filepath.Clean(path)
	if MatchPath(pattern, cleaned) {
		return true
	}
	// Deny rules also consider the absolute form of a relative path, so
	// "../../etc/shadow" is still caught by a deny on "/etc/**". (Allow rules
	// must not do this: relative paths may be resolved against a tool's
	// workspace dir, not the process working directory.)
	if ruleType == RuleDeny && !filepath.IsAbs(cleaned) {
		if abs, err := filepath.Abs(cleaned); err == nil && MatchPath(pattern, abs) {
			return true
		}
	}
	return false
}

func matchURLSpecifier(_ RuleType, pattern, urlStr string) bool {
	return MatchURLSpecifier(pattern, urlStr)
}

// urlHost returns the lowercased host of a specifier that is unambiguously a
// URL (http or https scheme), or "" otherwise.
func urlHost(specifier string) string {
	u, err := url.Parse(specifier)
	if err != nil {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
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
