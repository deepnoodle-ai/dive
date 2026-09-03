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

var userSettingsRoot = os.UserHomeDir

// Settings represents Dive settings.
//
// Three tiers merge into one effective value, later tiers winning per key:
// ~/.dive/settings.json (user) < .dive/settings.json (project base) <
// .dive/settings.local.json (local overrides). This mirrors Claude Code's
// settings semantics. Only Model, ThinkingEffort, ShowThinking, and
// ShowDetailedUsage are honored from the user tier today; Permissions and
// Sandbox continue to come from the project tiers.
// This mirrors the format used by Claude Code's .claude/settings.local.json.
type Settings struct {
	// Permissions contains allow and deny lists for tool operations.
	Permissions SettingsPermissions `json:"permissions"`
	// Sandbox contains sandboxing configuration.
	Sandbox *sandbox.Config `json:"sandbox,omitempty"`
	// Model is the default model ID (e.g. "claude-opus-4-6").
	Model *string `json:"model,omitempty"`
	// ThinkingEffort is the default reasoning effort (none, minimal, low,
	// medium, high, xhigh, max, or a provider-specific value).
	ThinkingEffort *string `json:"thinking_effort,omitempty"`
	// ShowThinking requests summarized model thinking when supported.
	ShowThinking *bool `json:"show_thinking,omitempty"`
	// ShowDetailedUsage renders the full token breakdown under the status
	// line. By default only the session total cost is shown inline.
	ShowDetailedUsage *bool `json:"show_detailed_usage,omitempty"`
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
// Project tiers only (.dive/settings.json + .dive/settings.local.json); see
// LoadEffectiveSettings for the user tier as well. Kept hermetic so tests
// never touch the real home directory.
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
	return unmarshalSettings(mergeSettingsMaps(base, local))
}

func unmarshalSettings(merged map[string]any) (*Settings, error) {
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

// LoadEffectiveSettings merges the user tier (~/.dive/settings.json) with the
// project tiers (.dive/settings.json, .dive/settings.local.json). Later tiers
// win per key using mergeSettingsMaps. A project dir of "" skips the project
// tiers, returning the user settings alone.
func LoadEffectiveSettings(dir string) (*Settings, error) {
	userPath, err := UserSettingsPath()
	if err != nil {
		return nil, err
	}
	user, err := readSettingsMap(userPath)
	if err != nil {
		return nil, err
	}
	// The global file currently owns only CLI presentation/model defaults.
	// Keep project security settings project-scoped until the permission and
	// sandbox loaders deliberately support a global trust tier.
	user = filterUserSettings(user)
	var base, local map[string]any
	if dir != "" {
		diveDir := filepath.Join(dir, ".dive")
		base, err = readSettingsMap(filepath.Join(diveDir, "settings.json"))
		if err != nil {
			return nil, err
		}
		local, err = readSettingsMap(filepath.Join(diveDir, "settings.local.json"))
		if err != nil {
			return nil, err
		}
	}

	merged := mergeSettingsMaps(user, base)
	merged = mergeSettingsMaps(merged, local)
	return unmarshalSettings(merged)
}

func filterUserSettings(user map[string]any) map[string]any {
	if user == nil {
		return nil
	}
	filtered := make(map[string]any, 4)
	for _, key := range []string{"model", "thinking_effort", "show_thinking", "show_detailed_usage"} {
		if value, ok := user[key]; ok {
			filtered[key] = value
		}
	}
	return filtered
}

// UserSettingsPath returns the global user settings path (~/.dive/settings.json).
func UserSettingsPath() (string, error) {
	home, err := userSettingsRoot()
	if err != nil {
		return "", fmt.Errorf("settings: find home directory: %w", err)
	}
	return filepath.Join(home, ".dive", "settings.json"), nil
}

// SetUserSettingsRootForTesting overrides the root used by UserSettingsPath
// and returns a function that restores the previous resolver. It exists so
// tests in importing packages can isolate settings writes without depending on
// whether the host platform uses HOME, USERPROFILE, or another home variable.
// Tests using this process-wide hook must not run in parallel.
func SetUserSettingsRootForTesting(root string) func() {
	previous := userSettingsRoot
	userSettingsRoot = func() (string, error) { return root, nil }
	return func() { userSettingsRoot = previous }
}

// SaveUserSettings persists user-tier defaults (model, thinking_effort,
// show_thinking, show_detailed_usage) to ~/.dive/settings.json. Nil fields are
// left untouched; non-nil fields overwrite. Unknown keys already in the file
// are preserved.
func SaveUserSettings(update *Settings) error {
	if update == nil {
		return fmt.Errorf("settings: update must not be nil")
	}
	path, err := UserSettingsPath()
	if err != nil {
		return err
	}
	existing, err := readSettingsMap(path)
	if err != nil {
		return err
	}
	if existing == nil {
		existing = make(map[string]any)
	}
	if update.Model != nil {
		existing["model"] = *update.Model
	}
	if update.ThinkingEffort != nil {
		existing["thinking_effort"] = *update.ThinkingEffort
	}
	if update.ShowThinking != nil {
		existing["show_thinking"] = *update.ShowThinking
	}
	if update.ShowDetailedUsage != nil {
		existing["show_detailed_usage"] = *update.ShowDetailedUsage
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: encode %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("settings: create directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "settings.json.tmp-*")
	if err != nil {
		return fmt.Errorf("settings: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("settings: write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("settings: close %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("settings: save %s: %w", path, err)
	}
	committed = true
	return nil
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
