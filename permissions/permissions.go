package permissions

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/deepnoodle-ai/dive/settings"
)

// PermissionDecision represents a permission decision
type PermissionDecision int

const (
	Allow PermissionDecision = iota
	Deny
	Ask
)

// PermissionMode represents the current permission mode
type PermissionMode string

const (
	NormalMode          PermissionMode = "normal"
	AcceptEditsMode     PermissionMode = "acceptEdits"
	BypassMode          PermissionMode = "bypassPermissions"
)

// PermissionManager manages tool permissions
type PermissionManager struct {
	settings *settings.PermissionSettings
	mode     PermissionMode
	workDir  string
}

// NewPermissionManager creates a new permission manager
func NewPermissionManager(s *settings.PermissionSettings, workDir string) *PermissionManager {
	pm := &PermissionManager{
		settings: s,
		mode:     NormalMode,
		workDir:  workDir,
	}

	// Set default mode from settings
	if s != nil && s.DefaultMode != "" {
		pm.mode = PermissionMode(s.DefaultMode)
	}

	return pm
}

// CheckToolPermission checks if a tool use is allowed
func (pm *PermissionManager) CheckToolPermission(toolName string, params map[string]interface{}) PermissionDecision {
	// Bypass mode allows everything
	if pm.mode == BypassMode && !pm.isDisabled() {
		return Allow
	}

	// Check deny rules first (highest priority)
	if pm.matchesRules(toolName, params, pm.settings.Deny) {
		return Deny
	}

	// Check allow rules
	if pm.matchesRules(toolName, params, pm.settings.Allow) {
		return Allow
	}

	// Check ask rules
	if pm.matchesRules(toolName, params, pm.settings.Ask) {
		return Ask
	}

	// Default behavior based on mode
	switch pm.mode {
	case AcceptEditsMode:
		// Auto-accept edit operations
		if isEditOperation(toolName) {
			return Allow
		}
		return Ask
	default:
		// Default to asking for permission
		return Ask
	}
}

// CheckFileAccess checks if a file path is allowed
func (pm *PermissionManager) CheckFileAccess(path string, operation string) PermissionDecision {
	// Normalize path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Deny
	}

	// Check if path is within working directory or allowed directories
	if !pm.isPathAllowed(absPath) {
		return Deny
	}

	// Check deny patterns for file operations
	for _, pattern := range pm.settings.Deny {
		if pm.matchesFilePattern(pattern, absPath, operation) {
			return Deny
		}
	}

	// Check allow patterns
	for _, pattern := range pm.settings.Allow {
		if pm.matchesFilePattern(pattern, absPath, operation) {
			return Allow
		}
	}

	// Default to asking
	return Ask
}

// SetMode sets the permission mode
func (pm *PermissionManager) SetMode(mode PermissionMode) error {
	if mode == BypassMode && pm.isDisabled() {
		return fmt.Errorf("bypass mode is disabled by policy")
	}
	pm.mode = mode
	return nil
}

// GetMode returns the current permission mode
func (pm *PermissionManager) GetMode() PermissionMode {
	return pm.mode
}

// isDisabled checks if bypass mode is disabled
func (pm *PermissionManager) isDisabled() bool {
	return pm.settings != nil && pm.settings.DisableBypassPermissions == "disable"
}

// matchesRules checks if a tool use matches any permission rules
func (pm *PermissionManager) matchesRules(toolName string, params map[string]interface{}, rules []string) bool {
	for _, rule := range rules {
		if pm.matchesRule(toolName, params, rule) {
			return true
		}
	}
	return false
}

// matchesRule checks if a tool use matches a single rule
func (pm *PermissionManager) matchesRule(toolName string, params map[string]interface{}, rule string) bool {
	// Parse rule format: Tool(params)
	if !strings.Contains(rule, "(") {
		// Simple tool name match
		return strings.EqualFold(rule, toolName)
	}

	// Extract tool and pattern
	parts := strings.SplitN(rule, "(", 2)
	if len(parts) != 2 {
		return false
	}

	ruleTool := strings.TrimSpace(parts[0])
	if !strings.EqualFold(ruleTool, toolName) {
		return false
	}

	// Extract pattern
	pattern := strings.TrimSuffix(parts[1], ")")

	// Handle specific tool patterns
	switch toolName {
	case "Bash":
		// For Bash, pattern is a command prefix
		if cmd, ok := params["command"].(string); ok {
			return pm.matchesBashCommand(cmd, pattern)
		}
	case "Read", "Write", "Edit":
		// For file operations, pattern is a file path pattern
		if path, ok := params["file_path"].(string); ok {
			return pm.matchesFilePattern(pattern, path, toolName)
		}
	case "WebFetch":
		// For web operations, pattern is a URL pattern
		if url, ok := params["url"].(string); ok {
			return pm.matchesURLPattern(url, pattern)
		}
	}

	return false
}

// matchesBashCommand checks if a command matches a bash pattern
func (pm *PermissionManager) matchesBashCommand(command, pattern string) bool {
	// Handle wildcard patterns
	if strings.HasSuffix(pattern, ":*") {
		prefix := strings.TrimSuffix(pattern, ":*")
		return strings.HasPrefix(command, prefix)
	}

	// Exact match
	return command == pattern
}

// matchesFilePattern checks if a file path matches a pattern
func (pm *PermissionManager) matchesFilePattern(pattern, path, operation string) bool {
	// First check if pattern includes operation
	if strings.Contains(pattern, "(") {
		parts := strings.Split(pattern, "(")
		if len(parts) == 2 {
			op := strings.TrimSpace(parts[0])
			if !strings.EqualFold(op, operation) {
				return false
			}
			pattern = strings.TrimSuffix(parts[1], ")")
		}
	}

	// Handle glob patterns
	if strings.Contains(pattern, "*") {
		matched, _ := filepath.Match(pattern, path)
		return matched
	}

	// Handle directory patterns
	if strings.HasSuffix(pattern, "/**") {
		dir := strings.TrimSuffix(pattern, "/**")
		absDir, _ := filepath.Abs(dir)
		return strings.HasPrefix(path, absDir)
	}

	// Exact match
	absPattern, _ := filepath.Abs(pattern)
	return path == absPattern
}

// matchesURLPattern checks if a URL matches a pattern
func (pm *PermissionManager) matchesURLPattern(url, pattern string) bool {
	// Handle wildcard patterns
	if pattern == "*" {
		return true
	}

	// Handle domain patterns
	if strings.HasPrefix(pattern, "*.") {
		domain := strings.TrimPrefix(pattern, "*.")
		return strings.Contains(url, domain)
	}

	// Handle regex patterns
	if strings.HasPrefix(pattern, "^") {
		if regex, err := regexp.Compile(pattern); err == nil {
			return regex.MatchString(url)
		}
	}

	// Exact match or prefix match
	return strings.HasPrefix(url, pattern)
}

// isPathAllowed checks if a path is within allowed directories
func (pm *PermissionManager) isPathAllowed(path string) bool {
	// Check if within working directory
	if strings.HasPrefix(path, pm.workDir) {
		return true
	}

	// Check additional directories
	if pm.settings != nil {
		for _, dir := range pm.settings.AdditionalDirectories {
			absDir, _ := filepath.Abs(dir)
			if strings.HasPrefix(path, absDir) {
				return true
			}
		}
	}

	return false
}

// isEditOperation checks if a tool is an edit operation
func isEditOperation(toolName string) bool {
	editTools := []string{"Edit", "Write", "MultiEdit", "NotebookEdit"}
	for _, tool := range editTools {
		if strings.EqualFold(toolName, tool) {
			return true
		}
	}
	return false
}

// FormatPermissionRule formats a permission rule for display
func FormatPermissionRule(rule string) string {
	if !strings.Contains(rule, "(") {
		return fmt.Sprintf("Tool: %s", rule)
	}

	parts := strings.SplitN(rule, "(", 2)
	tool := parts[0]
	pattern := strings.TrimSuffix(parts[1], ")")

	switch tool {
	case "Bash":
		if strings.HasSuffix(pattern, ":*") {
			return fmt.Sprintf("Bash commands starting with: %s", strings.TrimSuffix(pattern, ":*"))
		}
		return fmt.Sprintf("Bash command: %s", pattern)
	case "Read", "Write", "Edit":
		return fmt.Sprintf("%s files matching: %s", tool, pattern)
	case "WebFetch":
		return fmt.Sprintf("Web fetch from: %s", pattern)
	default:
		return fmt.Sprintf("%s(%s)", tool, pattern)
	}
}

// ValidatePermissionRules validates a set of permission rules
func ValidatePermissionRules(rules []string) error {
	for _, rule := range rules {
		if err := validateRule(rule); err != nil {
			return fmt.Errorf("invalid rule '%s': %w", rule, err)
		}
	}
	return nil
}

// validateRule validates a single permission rule
func validateRule(rule string) error {
	if rule == "" {
		return fmt.Errorf("empty rule")
	}

	// Check for valid tool names
	validTools := []string{
		"Bash", "Edit", "Glob", "Grep", "MultiEdit",
		"NotebookEdit", "Read", "Task", "TodoWrite",
		"WebFetch", "WebSearch", "Write",
	}

	// Extract tool name
	toolName := rule
	if idx := strings.Index(rule, "("); idx > 0 {
		toolName = rule[:idx]
	}

	// Check if tool is valid
	isValid := false
	for _, valid := range validTools {
		if strings.EqualFold(toolName, valid) {
			isValid = true
			break
		}
	}

	// Also allow MCP tool patterns
	if strings.HasPrefix(toolName, "mcp__") {
		isValid = true
	}

	if !isValid {
		return fmt.Errorf("unknown tool: %s", toolName)
	}

	// Validate pattern if present
	if strings.Contains(rule, "(") && !strings.HasSuffix(rule, ")") {
		return fmt.Errorf("unclosed parenthesis")
	}

	return nil
}

// InteractivePrompt represents an interactive permission prompt
type InteractivePrompt struct {
	Tool        string
	Description string
	Params      map[string]interface{}
}

// ShowPrompt displays an interactive permission prompt
func (ip *InteractivePrompt) ShowPrompt() (PermissionDecision, error) {
	fmt.Printf("\n⚠️  Permission required for %s\n", ip.Tool)
	fmt.Printf("Description: %s\n", ip.Description)

	if len(ip.Params) > 0 {
		fmt.Println("Parameters:")
		for key, value := range ip.Params {
			// Mask sensitive values
			displayValue := fmt.Sprintf("%v", value)
			if strings.Contains(strings.ToLower(key), "key") || strings.Contains(strings.ToLower(key), "token") {
				displayValue = "***"
			}
			fmt.Printf("  %s: %s\n", key, displayValue)
		}
	}

	fmt.Print("\nAllow this operation? (y/n/always): ")

	var response string
	fmt.Scanln(&response)

	switch strings.ToLower(response) {
	case "y", "yes":
		return Allow, nil
	case "n", "no":
		return Deny, nil
	case "always", "a":
		// TODO: Add to allow rules for this session
		return Allow, nil
	default:
		return Deny, fmt.Errorf("invalid response")
	}
}