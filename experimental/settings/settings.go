package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive/experimental/sandbox"
	"github.com/deepnoodle-ai/dive/permission"
)

// Settings represents the Dive project settings loaded from .dive/settings.json.
// This mirrors the format used by Claude Code's .claude/settings.local.json.
type Settings struct {
	// Permissions contains allow and deny lists for tool operations.
	Permissions SettingsPermissions `json:"permissions"`
	// Sandbox contains sandboxing configuration.
	Sandbox *sandbox.Config `json:"sandbox,omitempty"`
}

// SettingsPermissions contains permission rules in Claude Code format.
type SettingsPermissions struct {
	// Allow contains patterns for tools that should be auto-approved.
	// Patterns can be simple tool names or parameterized patterns like:
	//   - "WebSearch" - simple tool name
	//   - "Bash(go build:*)" - bash command pattern
	//   - "Read(/path/to/file/**)" - read with path pattern
	//   - "WebFetch(domain:example.com)" - web fetch with domain pattern
	Allow []string `json:"allow"`

	// Deny contains patterns for tools that should be blocked.
	// Uses the same pattern format as Allow.
	Deny []string `json:"deny"`
}

// LoadSettings loads settings from the .dive directory in the given directory.
// It checks for settings.local.json first (user-specific), then settings.json.
// If neither file exists, returns an empty Settings with no error.
func LoadSettings(dir string) (*Settings, error) {
	diveDir := filepath.Join(dir, ".dive")

	// Try settings.local.json first (takes precedence, like Claude Code)
	// Then fall back to settings.json
	filenames := []string{"settings.local.json", "settings.json"}

	for _, filename := range filenames {
		settingsPath := filepath.Join(diveDir, filename)
		data, err := os.ReadFile(settingsPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}

		var settings Settings
		if err := json.Unmarshal(data, &settings); err != nil {
			return nil, err
		}

		return &settings, nil
	}

	return &Settings{}, nil
}

// ToPermissionRules converts the settings permissions to permission.Rules.
// This parses Claude Code-style patterns into the internal rule format.
func (s *Settings) ToPermissionRules() permission.Rules {
	var rules permission.Rules

	// Process deny rules first (they take precedence in evaluation)
	for _, pattern := range s.Permissions.Deny {
		rule := parsePermissionPattern(pattern, permission.RuleDeny)
		if rule != nil {
			rules = append(rules, *rule)
		}
	}

	// Process allow rules
	for _, pattern := range s.Permissions.Allow {
		rule := parsePermissionPattern(pattern, permission.RuleAllow)
		if rule != nil {
			rules = append(rules, *rule)
		}
	}

	return rules
}

// parsePermissionPattern parses a Claude Code-style permission pattern into a Rule.
// Supports patterns like:
//   - "WebSearch" - simple tool name match
//   - "Bash(go build:*)" - bash with command pattern
//   - "Read(/path/to/file/**)" - read with file path pattern
//   - "WebFetch(domain:example.com)" - web fetch with domain constraint
//   - "mcp__ide__getDiagnostics" - MCP tool name
func parsePermissionPattern(pattern string, ruleType permission.RuleType) *permission.Rule {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}

	// Check for parameterized pattern: ToolName(args)
	if idx := strings.Index(pattern, "("); idx > 0 && strings.HasSuffix(pattern, ")") {
		toolName := pattern[:idx]
		args := pattern[idx+1 : len(pattern)-1]
		return parseParameterizedPattern(toolName, args, ruleType)
	}

	// Simple tool name pattern — delegate to permission.ParseRule
	rule, err := permission.ParseRule(ruleType, pattern)
	if err != nil {
		return nil
	}
	return &rule
}

// parseParameterizedPattern handles patterns like Bash(go build:*) or Read(/path/**)
func parseParameterizedPattern(toolName, args string, ruleType permission.RuleType) *permission.Rule {
	toolNameLower := strings.ToLower(toolName)

	switch {
	case toolNameLower == "bash" || toolNameLower == "shell" || toolNameLower == "command":
		return parseBashPattern("Bash", args, ruleType)

	case toolNameLower == "read" || toolNameLower == "read_file":
		return parsePathPattern("Read", args, ruleType)

	case toolNameLower == "write" || toolNameLower == "write_file":
		return parsePathPattern("Write", args, ruleType)

	case toolNameLower == "edit":
		return parsePathPattern("Edit", args, ruleType)

	case toolNameLower == "webfetch" || toolNameLower == "web_fetch":
		return parseWebFetchPattern(args, ruleType)

	default:
		// Generic tool with arguments - treat args as input pattern
		rule := permission.Rule{
			Type: ruleType,
			Tool: toolName,
			InputMatch: func(input any) bool {
				inputBytes, _ := json.Marshal(input)
				return strings.Contains(string(inputBytes), args)
			},
		}
		return &rule
	}
}

// parseBashPattern parses bash command patterns like "go build:*"
func parseBashPattern(toolName, args string, ruleType permission.RuleType) *permission.Rule {
	// Convert Claude Code pattern to specifier glob
	// "go build:*" -> "go build*"
	// "ls" -> "ls" (exact match)
	specifier := args
	if strings.HasSuffix(specifier, ":*") {
		specifier = strings.TrimSuffix(specifier, ":*") + "*"
	}

	rule := permission.ParseRuleWithSpecifier(ruleType, toolName, specifier)
	return &rule
}

// parsePathPattern parses file path patterns for read/write tools
func parsePathPattern(toolName, pathPattern string, ruleType permission.RuleType) *permission.Rule {
	// Convert Claude Code path patterns to our format
	// "//Users/path/**" -> "/Users/path/**" (remove leading double slash)
	if strings.HasPrefix(pathPattern, "//") {
		pathPattern = pathPattern[1:]
	}

	rule := permission.Rule{
		Type: ruleType,
		Tool: toolName,
		InputMatch: func(input any) bool {
			inputMap, ok := input.(map[string]any)
			if !ok {
				return false
			}

			var filePath string
			for _, field := range []string{"file_path", "filePath", "path"} {
				if p, ok := inputMap[field].(string); ok {
					filePath = p
					break
				}
			}

			if filePath == "" {
				return false
			}

			return permission.MatchPath(pathPattern, filePath)
		},
	}
	return &rule
}

// parseWebFetchPattern parses WebFetch patterns like "domain:example.com"
func parseWebFetchPattern(args string, ruleType permission.RuleType) *permission.Rule {
	if strings.HasPrefix(args, "domain:") {
		domain := strings.TrimPrefix(args, "domain:")
		rule := permission.Rule{
			Type: ruleType,
			Tool: "WebFetch",
			InputMatch: func(input any) bool {
				inputMap, ok := input.(map[string]any)
				if !ok {
					return false
				}

				url, ok := inputMap["url"].(string)
				if !ok {
					return false
				}

				return permission.MatchDomain(url, domain)
			},
		}
		return &rule
	}

	// Generic URL pattern
	rule := permission.Rule{
		Type: ruleType,
		Tool: "WebFetch",
		InputMatch: func(input any) bool {
			inputMap, ok := input.(map[string]any)
			if !ok {
				return false
			}

			url, ok := inputMap["url"].(string)
			if !ok {
				return false
			}

			return permission.MatchPath(args, url)
		},
	}
	return &rule
}
