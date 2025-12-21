package dive

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive/llm"
)

// Permission Rules
//
// This file implements declarative permission rules for the tool permission system.
// Rules provide a static, configuration-driven way to control tool access without
// writing custom hook functions.
//
// Rules are evaluated in a specific order during the permission flow:
//  1. Deny rules are checked first
//  2. Allow rules are checked second
//  3. Ask rules are checked third
//
// The first matching rule in each category determines the action. This ordering
// ensures that explicit denials take precedence over allows.
//
// Pattern Matching:
//
// Tool patterns use glob-style matching (via filepath.Match):
//   - "bash" matches exactly "bash"
//   - "read_*" matches "read_file", "read_config", etc.
//   - "*" matches any tool
//
// Command patterns (for bash-like tools) support wildcards:
//   - "rm -rf *" matches any rm -rf command
//   - "git push *" matches any git push command
//
// Example:
//
//	rules := dive.PermissionRules{
//	    dive.DenyRule("dangerous_*", "Dangerous tools are blocked"),
//	    dive.DenyCommandRule("bash", "rm -rf *", "Recursive deletion not allowed"),
//	    dive.AllowRule("read_*"),
//	    dive.AskRule("write_*", "Confirm file write"),
//	}

// PermissionRuleType indicates what action a rule takes when it matches.
type PermissionRuleType string

const (
	// PermissionRuleDeny blocks tool execution immediately.
	// Deny rules are evaluated first and take precedence over allow/ask rules.
	PermissionRuleDeny PermissionRuleType = "deny"

	// PermissionRuleAllow permits tool execution without prompting.
	// Allow rules are evaluated after deny rules.
	PermissionRuleAllow PermissionRuleType = "allow"

	// PermissionRuleAsk prompts the user for confirmation before executing.
	// Ask rules are evaluated after deny and allow rules.
	PermissionRuleAsk PermissionRuleType = "ask"
)

// PermissionRule defines a declarative permission rule.
// Rules can match based on tool name patterns, command patterns, or custom input matchers.
// When multiple matching criteria are specified, ALL must match for the rule to apply.
//
// Example with custom input matcher:
//
//	rule := dive.PermissionRule{
//	    Type: dive.PermissionRuleDeny,
//	    Tool: "write_file",
//	    InputMatch: func(input any) bool {
//	        m, ok := input.(map[string]any)
//	        if !ok {
//	            return false
//	        }
//	        path, _ := m["path"].(string)
//	        return strings.HasPrefix(path, "/etc/")  // Block writes to /etc/
//	    },
//	    Message: "Cannot write to system directories",
//	}
type PermissionRule struct {
	// Type is the action to take when this rule matches.
	// Must be one of: PermissionRuleDeny, PermissionRuleAllow, PermissionRuleAsk.
	Type PermissionRuleType `json:"type" yaml:"type"`

	// Tool is a glob pattern for matching tool names.
	// Uses filepath.Match semantics: "*" matches any sequence, "?" matches single char.
	// Examples: "bash", "read_*", "write_file", "*"
	Tool string `json:"tool" yaml:"tool"`

	// Command is an optional glob pattern for matching bash/shell command content.
	// Only evaluated when the Tool pattern matches a bash-like tool.
	// The command is extracted from common input fields: "command", "cmd", "script", "code".
	// Uses simple glob matching where "*" matches any sequence of characters.
	// Examples: "rm -rf *", "git push *", "curl *"
	Command string `json:"command,omitempty" yaml:"command,omitempty"`

	// Message provides context for deny/ask actions.
	// For deny: explains why the tool was blocked (shown to LLM).
	// For ask: displayed to the user when prompting for confirmation.
	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	// InputMatch is an optional programmatic matcher for complex input validation.
	// If set, the rule only matches when this function returns true.
	// The input parameter is the unmarshaled JSON input from the tool call.
	// This field is not serializable and must be set programmatically.
	InputMatch func(input any) bool `json:"-" yaml:"-"`
}

// PermissionRules is an ordered list of permission rules.
type PermissionRules []PermissionRule

// Evaluate checks the rules against a tool call and returns the first matching action.
// Returns nil if no rules match.
func (rules PermissionRules) Evaluate(tool Tool, call *llm.ToolUseContent) *ToolHookResult {
	if tool == nil || call == nil {
		return nil
	}

	toolName := tool.Name()

	for _, rule := range rules {
		if !matchToolPattern(rule.Tool, toolName) {
			continue
		}

		// If command pattern is specified, check it for bash-like tools
		if rule.Command != "" {
			if !matchCommandPattern(rule.Command, call.Input) {
				continue
			}
		}

		// If input matcher is specified, check it
		if rule.InputMatch != nil {
			var input any
			if err := json.Unmarshal(call.Input, &input); err != nil {
				continue
			}
			if !rule.InputMatch(input) {
				continue
			}
		}

		// Rule matches - return the action
		return &ToolHookResult{
			Action:  ruleTypeToAction(rule.Type),
			Message: rule.Message,
		}
	}

	return nil
}

// matchToolPattern checks if a tool name matches a glob pattern.
func matchToolPattern(pattern, toolName string) bool {
	if pattern == "*" {
		return true
	}

	// Use filepath.Match for glob-style matching
	matched, err := filepath.Match(pattern, toolName)
	if err != nil {
		// Invalid pattern - try exact match
		return pattern == toolName
	}
	return matched
}

// matchCommandPattern checks if the tool input contains a matching command.
// This extracts the command from bash-style tool inputs.
func matchCommandPattern(pattern string, input json.RawMessage) bool {
	// Try to extract command from common input formats
	var inputMap map[string]any
	if err := json.Unmarshal(input, &inputMap); err != nil {
		return false
	}

	// Look for command in common field names
	commandFields := []string{"command", "cmd", "script", "code"}
	var command string
	for _, field := range commandFields {
		if cmd, ok := inputMap[field].(string); ok {
			command = cmd
			break
		}
	}

	if command == "" {
		return false
	}

	// Match using glob pattern against the command
	// Support wildcards in the pattern
	if pattern == "*" {
		return true
	}

	// Simple glob matching for command patterns
	return matchCommandGlob(pattern, command)
}

// matchCommandGlob performs glob-like matching on a command string.
// Supports * for any characters.
func matchCommandGlob(pattern, command string) bool {
	// Normalize whitespace
	pattern = strings.TrimSpace(pattern)
	command = strings.TrimSpace(command)

	// Handle simple cases
	if pattern == command {
		return true
	}

	// Convert glob pattern to a simple matcher
	// Split on * and check if all parts appear in order
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == command
	}

	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}

		idx := strings.Index(command[pos:], part)
		if idx == -1 {
			return false
		}

		// First part must be at start if pattern doesn't start with *
		if i == 0 && idx != 0 {
			return false
		}

		pos += idx + len(part)
	}

	// Last part must be at end if pattern doesn't end with *
	if len(parts) > 0 && parts[len(parts)-1] != "" {
		return strings.HasSuffix(command, parts[len(parts)-1])
	}

	return true
}

// ruleTypeToAction converts a PermissionRuleType to a ToolHookAction.
func ruleTypeToAction(ruleType PermissionRuleType) ToolHookAction {
	switch ruleType {
	case PermissionRuleDeny:
		return ToolHookDeny
	case PermissionRuleAllow:
		return ToolHookAllow
	case PermissionRuleAsk:
		return ToolHookAsk
	default:
		return ToolHookContinue
	}
}

// DenyRule creates a deny rule that blocks any tool matching the pattern.
// The message is returned to the LLM explaining why the tool was blocked.
//
// Example:
//
//	DenyRule("dangerous_*", "Dangerous tools are not permitted")
//	DenyRule("*", "All tools blocked in read-only mode")
func DenyRule(toolPattern string, message string) PermissionRule {
	return PermissionRule{
		Type:    PermissionRuleDeny,
		Tool:    toolPattern,
		Message: message,
	}
}

// DenyCommandRule creates a deny rule for specific bash/shell commands.
// Both the tool pattern and command pattern must match for the rule to apply.
//
// Example:
//
//	DenyCommandRule("bash", "rm -rf *", "Recursive deletion is not allowed")
//	DenyCommandRule("bash", "sudo *", "Sudo commands are blocked")
func DenyCommandRule(toolPattern, commandPattern, message string) PermissionRule {
	return PermissionRule{
		Type:    PermissionRuleDeny,
		Tool:    toolPattern,
		Command: commandPattern,
		Message: message,
	}
}

// AllowRule creates an allow rule that permits any tool matching the pattern
// to execute without prompting for confirmation.
//
// Example:
//
//	AllowRule("read_*")    // Allow all read operations
//	AllowRule("glob")      // Allow the glob tool
func AllowRule(toolPattern string) PermissionRule {
	return PermissionRule{
		Type: PermissionRuleAllow,
		Tool: toolPattern,
	}
}

// AllowCommandRule creates an allow rule for specific bash/shell commands.
// Both the tool pattern and command pattern must match for the rule to apply.
//
// Example:
//
//	AllowCommandRule("bash", "ls *", "Directory listing is always allowed")
//	AllowCommandRule("bash", "git status", "Git status is safe")
func AllowCommandRule(toolPattern, commandPattern string) PermissionRule {
	return PermissionRule{
		Type:    PermissionRuleAllow,
		Tool:    toolPattern,
		Command: commandPattern,
	}
}

// AskRule creates a rule that prompts the user for confirmation when matched.
// The message is displayed to the user when asking for confirmation.
//
// Example:
//
//	AskRule("write_*", "Confirm file write operation")
//	AskRule("bash", "Confirm shell command execution")
func AskRule(toolPattern string, message string) PermissionRule {
	return PermissionRule{
		Type:    PermissionRuleAsk,
		Tool:    toolPattern,
		Message: message,
	}
}

// AskCommandRule creates an ask rule for specific bash/shell commands.
// The user is prompted for confirmation when both patterns match.
//
// Example:
//
//	AskCommandRule("bash", "git push *", "Confirm push to remote")
//	AskCommandRule("bash", "npm publish *", "Confirm package publication")
func AskCommandRule(toolPattern, commandPattern, message string) PermissionRule {
	return PermissionRule{
		Type:    PermissionRuleAsk,
		Tool:    toolPattern,
		Command: commandPattern,
		Message: message,
	}
}
