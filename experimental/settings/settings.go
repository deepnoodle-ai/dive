package settings

import (
	"encoding/json"
	"fmt"
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
// Both settings.json (the project base) and settings.local.json (user-specific
// overrides) are read when present and merged, mirroring Claude Code
// semantics: settings.local.json overrides settings.json rather than
// replacing it wholesale.
//
// Merge rules, applied recursively to the raw JSON documents:
//   - Objects/maps merge per key, with the local value winning on conflict.
//     This applies at every nesting level (e.g. "permissions", "sandbox",
//     "sandbox.environment").
//   - Arrays/slices replace wholesale: a local "permissions.allow" list
//     replaces the entire base list rather than appending to it.
//   - Scalar keys present in the local file win, including explicit zero
//     values such as false or "" (presence in the file is the override
//     signal); keys absent from the local file keep the base value.
//
// If neither file exists, returns an empty Settings with no error.
func LoadSettings(dir string) (*Settings, error) {
	diveDir := filepath.Join(dir, ".dive")

	base, err := readSettingsMap(filepath.Join(diveDir, "settings.json"))
	if err != nil {
		return nil, err
	}
	local, err := readSettingsMap(filepath.Join(diveDir, "settings.local.json"))
	if err != nil {
		return nil, err
	}

	merged := mergeSettingsMaps(base, local)
	if merged == nil {
		return &Settings{}, nil
	}

	data, err := json.Marshal(merged)
	if err != nil {
		return nil, err
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

// readSettingsMap reads a settings file into a generic JSON map. Returns
// (nil, nil) if the file does not exist.
func readSettingsMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("settings: parse %s: %w", path, err)
	}
	return m, nil
}

// mergeSettingsMaps deep-merges override into base and returns the result.
// Objects merge per key recursively with override winning; any other value
// type (arrays, scalars, null) present in override replaces the base value.
// Either argument may be nil, in which case the other is returned.
func mergeSettingsMaps(base, override map[string]any) map[string]any {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}
	out := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		if baseMap, ok := out[k].(map[string]any); ok {
			if overrideMap, ok := v.(map[string]any); ok {
				out[k] = mergeSettingsMaps(baseMap, overrideMap)
				continue
			}
		}
		out[k] = v
	}
	return out
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
		// Generic tool with arguments - treat args as glob pattern against input
		rule := permission.Rule{
			Type: ruleType,
			Tool: toolName,
			InputMatch: func(input any) bool {
				inputBytes, _ := json.Marshal(input)
				return permission.MatchGlob(args, string(inputBytes))
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
