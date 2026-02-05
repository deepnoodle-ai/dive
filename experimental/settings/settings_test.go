package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/dive/experimental/permission"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestLoadSettings(t *testing.T) {
	t.Run("returns empty settings when file doesn't exist", func(t *testing.T) {
		settings, err := LoadSettings("/nonexistent/path")
		assert.NoError(t, err)
		assert.NotNil(t, settings)
		assert.Empty(t, settings.Permissions.Allow)
		assert.Empty(t, settings.Permissions.Deny)
	})

	t.Run("loads settings from .dive/settings.json", func(t *testing.T) {
		// Create temp directory
		tmpDir := t.TempDir()
		diveDir := filepath.Join(tmpDir, ".dive")
		assert.NoError(t, os.Mkdir(diveDir, 0755))

		// Write settings file
		settingsJSON := `{
  "permissions": {
    "allow": [
      "WebSearch",
      "Bash(go test:*)",
      "Read(/path/to/files/**)"
    ],
    "deny": [
      "Bash(rm -rf:*)"
    ]
  }
}`
		assert.NoError(t, os.WriteFile(filepath.Join(diveDir, "settings.json"), []byte(settingsJSON), 0644))

		// Load settings
		settings, err := LoadSettings(tmpDir)
		assert.NoError(t, err)
		assert.NotNil(t, settings)
		assert.Len(t, settings.Permissions.Allow, 3)
		assert.Len(t, settings.Permissions.Deny, 1)
	})

	t.Run("settings.local.json takes precedence over settings.json", func(t *testing.T) {
		// Create temp directory
		tmpDir := t.TempDir()
		diveDir := filepath.Join(tmpDir, ".dive")
		assert.NoError(t, os.Mkdir(diveDir, 0755))

		// Write both settings files
		settingsJSON := `{"permissions": {"allow": ["WebSearch"]}}`
		localSettingsJSON := `{"permissions": {"allow": ["WebSearch", "Bash(go test:*)"]}}`

		assert.NoError(t, os.WriteFile(filepath.Join(diveDir, "settings.json"), []byte(settingsJSON), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(diveDir, "settings.local.json"), []byte(localSettingsJSON), 0644))

		// Load settings - should get local version
		settings, err := LoadSettings(tmpDir)
		assert.NoError(t, err)
		assert.NotNil(t, settings)
		assert.Len(t, settings.Permissions.Allow, 2) // local has 2, regular has 1
	})
}

func TestParsePermissionPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		ruleType permission.RuleType
		wantTool string
		wantCmd  string
	}{
		{
			name:     "simple tool name",
			pattern:  "WebSearch",
			ruleType: permission.RuleAllow,
			wantTool: "WebSearch",
		},
		{
			name:     "bash command pattern",
			pattern:  "Bash(go test:*)",
			ruleType: permission.RuleAllow,
			wantTool: "Bash", // PascalCase tool name
			wantCmd:  "go test*",
		},
		{
			name:     "bash exact command",
			pattern:  "Bash(ls -la)",
			ruleType: permission.RuleAllow,
			wantTool: "Bash", // PascalCase tool name
			wantCmd:  "ls -la",
		},
		{
			name:     "MCP tool pattern",
			pattern:  "mcp__ide__getDiagnostics",
			ruleType: permission.RuleAllow,
			wantTool: "mcp__ide__getDiagnostics",
		},
		{
			name:     "read file pattern normalizes tool name",
			pattern:  "Read(/path/to/file)",
			ruleType: permission.RuleAllow,
			wantTool: "Read",
		},
		{
			name:     "write file pattern normalizes tool name",
			pattern:  "Write(/path/to/file)",
			ruleType: permission.RuleAllow,
			wantTool: "Write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := parsePermissionPattern(tt.pattern, tt.ruleType)
			assert.NotNil(t, rule)
			assert.Equal(t, rule.Type, tt.ruleType)
			assert.Equal(t, rule.Tool, tt.wantTool)
			if tt.wantCmd != "" {
				assert.Equal(t, rule.Command, tt.wantCmd)
			}
		})
	}
}

func TestToPermissionRules(t *testing.T) {
	settings := &Settings{
		Permissions: SettingsPermissions{
			Allow: []string{
				"WebSearch",
				"Bash(go build:*)",
			},
			Deny: []string{
				"Bash(rm -rf:*)",
			},
		},
	}

	rules := settings.ToPermissionRules()

	// Deny rules come first
	assert.Len(t, rules, 3)

	// First rule should be deny
	assert.Equal(t, rules[0].Type, permission.RuleDeny)
	assert.Equal(t, rules[0].Tool, "Bash") // PascalCase tool name
	assert.Equal(t, rules[0].Command, "rm -rf*")

	// Allow rules come after
	assert.Equal(t, rules[1].Type, permission.RuleAllow)
	assert.Equal(t, rules[1].Tool, "WebSearch")

	assert.Equal(t, rules[2].Type, permission.RuleAllow)
	assert.Equal(t, rules[2].Tool, "Bash") // PascalCase tool name
	assert.Equal(t, rules[2].Command, "go build*")
}

func TestMatchDomain(t *testing.T) {
	tests := []struct {
		url    string
		domain string
		want   bool
	}{
		// Exact matches
		{"https://example.com/path", "example.com", true},
		{"http://example.com", "example.com", true},

		// Subdomain matches
		{"https://sub.example.com/path", "example.com", true},
		{"https://deep.sub.example.com", "example.com", true},

		// Non-matches (substring but not subdomain)
		{"https://notexample.com", "example.com", false},
		{"https://example.com.evil.com", "example.com", false},

		// Different domains
		{"https://other.com", "example.com", false},

		// With ports
		{"https://example.com:8080/path", "example.com", true},
		{"https://sub.example.com:443", "example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.url+"_"+tt.domain, func(t *testing.T) {
			got := matchDomain(tt.url, tt.domain)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestMatchPathGlob(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/path/to/file", "/path/to/file", true},
		{"/path/to/*", "/path/to/file", true},
		{"/path/to/*", "/path/to/file.go", true},
		{"/path/**", "/path/to/file", true},
		{"/path/**", "/path/to/deep/nested/file", true},
		{"/path/to/*", "/other/path", false},
		{"*.go", "file.go", true},
		{"*.go", "file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			got := matchPathGlob(tt.pattern, tt.path)
			assert.Equal(t, got, tt.want)
		})
	}
}
