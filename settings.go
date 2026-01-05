package dive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Settings represents the Dive project settings loaded from .dive/settings.json.
// This mirrors the format used by Claude Code's .claude/settings.local.json.
type Settings struct {
	// Permissions contains allow and deny lists for tool operations.
	Permissions SettingsPermissions `json:"permissions"`
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

// ToPermissionRules converts the settings permissions to PermissionRules.
// This parses Claude Code-style patterns into the internal rule format.
func (s *Settings) ToPermissionRules() PermissionRules {
	var rules PermissionRules

	// Process deny rules first (they take precedence in evaluation)
	for _, pattern := range s.Permissions.Deny {
		rule := parsePermissionPattern(pattern, PermissionRuleDeny)
		if rule != nil {
			rules = append(rules, *rule)
		}
	}

	// Process allow rules
	for _, pattern := range s.Permissions.Allow {
		rule := parsePermissionPattern(pattern, PermissionRuleAllow)
		if rule != nil {
			rules = append(rules, *rule)
		}
	}

	return rules
}

// parsePermissionPattern parses a Claude Code-style permission pattern into a PermissionRule.
// Supports patterns like:
//   - "WebSearch" - simple tool name match
//   - "Bash(go build:*)" - bash with command pattern
//   - "Read(/path/to/file/**)" - read with file path pattern
//   - "WebFetch(domain:example.com)" - web fetch with domain constraint
//   - "mcp__ide__getDiagnostics" - MCP tool name
func parsePermissionPattern(pattern string, ruleType PermissionRuleType) *PermissionRule {
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

	// Simple tool name pattern
	return &PermissionRule{
		Type: ruleType,
		Tool: pattern,
	}
}

// parseParameterizedPattern handles patterns like Bash(go build:*) or Read(/path/**)
func parseParameterizedPattern(toolName, args string, ruleType PermissionRuleType) *PermissionRule {
	toolNameLower := strings.ToLower(toolName)

	switch {
	case toolNameLower == "bash" || toolNameLower == "shell" || toolNameLower == "command":
		// Bash command pattern: "go build:*" means command starts with "go build"
		// The colon separates the prefix from a wildcard suffix
		// Use lowercase "bash" to match actual tool name
		return parseBashPattern("bash", args, ruleType)

	case toolNameLower == "read" || toolNameLower == "read_file":
		// Read file pattern: path glob
		// Use "read_file" to match actual tool name
		return parsePathPattern("read_file", args, ruleType)

	case toolNameLower == "write" || toolNameLower == "write_file":
		// Write file pattern: path glob
		return parsePathPattern("write_file", args, ruleType)

	case toolNameLower == "edit":
		// Edit file pattern: path glob
		return parsePathPattern("edit", args, ruleType)

	case toolNameLower == "webfetch" || toolNameLower == "web_fetch":
		// WebFetch pattern: domain:example.com
		return parseWebFetchPattern(args, ruleType)

	default:
		// Generic tool with arguments - treat args as input pattern
		return &PermissionRule{
			Type: ruleType,
			Tool: toolName,
			InputMatch: func(input any) bool {
				// Simple substring match on serialized input
				inputBytes, _ := json.Marshal(input)
				return strings.Contains(string(inputBytes), args)
			},
		}
	}
}

// parseBashPattern parses bash command patterns like "go build:*"
func parseBashPattern(toolName, args string, ruleType PermissionRuleType) *PermissionRule {
	// Handle patterns like "go build:*" or "ls -la /path:*"
	// The :* suffix indicates a prefix match

	// Convert pattern to command glob
	// "go build:*" -> "go build*"
	// "ls" -> "ls" (exact match)
	commandPattern := args
	if strings.HasSuffix(commandPattern, ":*") {
		commandPattern = strings.TrimSuffix(commandPattern, ":*") + "*"
	}

	return &PermissionRule{
		Type:    ruleType,
		Tool:    toolName,
		Command: commandPattern,
	}
}

// parsePathPattern parses file path patterns for read/write tools
func parsePathPattern(toolName, pathPattern string, ruleType PermissionRuleType) *PermissionRule {
	// Convert Claude Code path patterns to our format
	// "//Users/path/**" -> "/Users/path/**" (remove leading double slash)
	// "/path/to/file" -> exact match
	// "/path/**" -> glob match

	if strings.HasPrefix(pathPattern, "//") {
		pathPattern = pathPattern[1:] // Remove one leading slash
	}

	return &PermissionRule{
		Type: ruleType,
		Tool: toolName,
		InputMatch: func(input any) bool {
			inputMap, ok := input.(map[string]any)
			if !ok {
				return false
			}

			// Look for path in common field names
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

			return matchPathGlob(pathPattern, filePath)
		},
	}
}

// parseWebFetchPattern parses WebFetch patterns like "domain:example.com"
func parseWebFetchPattern(args string, ruleType PermissionRuleType) *PermissionRule {
	// Handle "domain:example.com" format
	if strings.HasPrefix(args, "domain:") {
		domain := strings.TrimPrefix(args, "domain:")
		return &PermissionRule{
			Type: ruleType,
			Tool: "fetch", // Actual tool name is "fetch"
			InputMatch: func(input any) bool {
				inputMap, ok := input.(map[string]any)
				if !ok {
					return false
				}

				url, ok := inputMap["url"].(string)
				if !ok {
					return false
				}

				// Match domain properly: check for domain at host position
				// This handles http://example.com, https://sub.example.com, etc.
				return matchDomain(url, domain)
			},
		}
	}

	// Generic URL pattern
	return &PermissionRule{
		Type: ruleType,
		Tool: "fetch",
		InputMatch: func(input any) bool {
			inputMap, ok := input.(map[string]any)
			if !ok {
				return false
			}

			url, ok := inputMap["url"].(string)
			if !ok {
				return false
			}

			return matchPathGlob(args, url)
		},
	}
}

// matchDomain checks if a URL's host matches or is a subdomain of the given domain.
func matchDomain(urlStr, domain string) bool {
	// Extract host from URL
	// Handle both http://example.com and //example.com formats
	host := urlStr

	// Remove protocol
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	} else if strings.HasPrefix(host, "//") {
		host = host[2:]
	}

	// Remove path
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}

	// Remove port
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Check exact match or subdomain match
	// example.com matches example.com
	// sub.example.com matches example.com
	// notexample.com does NOT match example.com
	if host == domain {
		return true
	}
	return strings.HasSuffix(host, "."+domain)
}

// matchPathGlob performs glob-style matching on file paths.
// Supports * (single segment) and ** (multiple segments).
func matchPathGlob(pattern, path string) bool {
	// Handle exact match
	if pattern == path {
		return true
	}

	// Convert glob pattern to regex
	regexPattern := globToRegex(pattern)
	matched, err := regexp.MatchString(regexPattern, path)
	if err != nil {
		return false
	}
	return matched
}

// globToRegex converts a glob pattern to a regex pattern.
func globToRegex(glob string) string {
	// Escape special regex characters except * and ?
	var result strings.Builder
	result.WriteString("^")

	i := 0
	for i < len(glob) {
		c := glob[i]
		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				// ** matches any path including separators
				result.WriteString(".*")
				i++ // Skip the second *
			} else {
				// * matches anything except path separators
				result.WriteString("[^/]*")
			}
		case '?':
			result.WriteString("[^/]")
		case '.', '+', '^', '$', '|', '(', ')', '[', ']', '{', '}', '\\':
			result.WriteByte('\\')
			result.WriteByte(c)
		default:
			result.WriteByte(c)
		}
		i++
	}

	result.WriteString("$")
	return result.String()
}
