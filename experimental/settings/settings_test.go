package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/deepnoodle-ai/dive/permission"
	"github.com/deepnoodle-ai/wonton/assert"
)

func useTestUserSettingsRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	restore := SetUserSettingsRootForTesting(root)
	t.Cleanup(restore)
	return root
}

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

	t.Run("settings.local.json merges with settings.json instead of shadowing it", func(t *testing.T) {
		tmpDir := t.TempDir()
		diveDir := filepath.Join(tmpDir, ".dive")
		assert.NoError(t, os.Mkdir(diveDir, 0755))

		settingsJSON := `{
  "permissions": {
    "allow": ["WebSearch", "Bash(go build:*)"],
    "deny": ["Bash(rm -rf:*)"]
  },
  "sandbox": {
    "enabled": true,
    "environment": {"BASE_VAR": "base", "SHARED_VAR": "from-base"},
    "docker": {"image": "ubuntu:22.04"}
  }
}`
		localJSON := `{
  "permissions": {
    "allow": ["Bash(go test:*)"]
  },
  "sandbox": {
    "environment": {"SHARED_VAR": "from-local", "LOCAL_VAR": "local"},
    "docker": {"command": "podman"}
  }
}`
		assert.NoError(t, os.WriteFile(filepath.Join(diveDir, "settings.json"), []byte(settingsJSON), 0644))
		assert.NoError(t, os.WriteFile(filepath.Join(diveDir, "settings.local.json"), []byte(localJSON), 0644))

		settings, err := LoadSettings(tmpDir)
		assert.NoError(t, err)
		assert.NotNil(t, settings)

		// Slice field: the local allow list replaces the base list wholesale.
		assert.Equal(t, []string{"Bash(go test:*)"}, settings.Permissions.Allow)
		// Slice field absent from local: the base deny list is preserved
		// (previously the entire settings.json — deny rules included — was
		// silently discarded when settings.local.json existed).
		assert.Equal(t, []string{"Bash(rm -rf:*)"}, settings.Permissions.Deny)

		// Map field: merged per key, local winning on conflict.
		assert.NotNil(t, settings.Sandbox)
		assert.Equal(t, map[string]string{
			"BASE_VAR":   "base",
			"SHARED_VAR": "from-local",
			"LOCAL_VAR":  "local",
		}, settings.Sandbox.Environment)

		// Scalar fields: base values absent from local are kept; local
		// values win where present.
		assert.True(t, settings.Sandbox.Enabled)
		assert.Equal(t, "ubuntu:22.04", settings.Sandbox.Docker.Image)
		assert.Equal(t, "podman", settings.Sandbox.Docker.Command)
	})

	t.Run("only settings.local.json present", func(t *testing.T) {
		tmpDir := t.TempDir()
		diveDir := filepath.Join(tmpDir, ".dive")
		assert.NoError(t, os.Mkdir(diveDir, 0755))

		localJSON := `{"permissions": {"allow": ["WebSearch"]}}`
		assert.NoError(t, os.WriteFile(filepath.Join(diveDir, "settings.local.json"), []byte(localJSON), 0644))

		settings, err := LoadSettings(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, []string{"WebSearch"}, settings.Permissions.Allow)
	})
}

func TestParsePermissionPattern(t *testing.T) {
	tests := []struct {
		name          string
		pattern       string
		ruleType      permission.RuleType
		wantTool      string
		wantSpecifier string
	}{
		{
			name:     "simple tool name",
			pattern:  "WebSearch",
			ruleType: permission.RuleAllow,
			wantTool: "WebSearch",
		},
		{
			name:          "bash command pattern",
			pattern:       "Bash(go test:*)",
			ruleType:      permission.RuleAllow,
			wantTool:      "Bash",
			wantSpecifier: "go test*",
		},
		{
			name:          "bash exact command",
			pattern:       "Bash(ls -la)",
			ruleType:      permission.RuleAllow,
			wantTool:      "Bash",
			wantSpecifier: "ls -la",
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
			if tt.wantSpecifier != "" {
				assert.Equal(t, rule.Specifier, tt.wantSpecifier)
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
	assert.Equal(t, rules[0].Tool, "Bash")
	assert.Equal(t, rules[0].Specifier, "rm -rf*")

	// Allow rules come after
	assert.Equal(t, rules[1].Type, permission.RuleAllow)
	assert.Equal(t, rules[1].Tool, "WebSearch")

	assert.Equal(t, rules[2].Type, permission.RuleAllow)
	assert.Equal(t, rules[2].Tool, "Bash")
	assert.Equal(t, rules[2].Specifier, "go build*")
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
			got := permission.MatchDomain(tt.url, tt.domain)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestMatchPath(t *testing.T) {
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
			got := permission.MatchPath(tt.pattern, tt.path)
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestSaveUserSettingsRoundTrip(t *testing.T) {
	root := useTestUserSettingsRoot(t)

	assert.NoError(t, SaveUserSettings(&Settings{
		Model:             strPtr("claude-opus-4-6"),
		ThinkingEffort:    strPtr("high"),
		ShowThinking:      boolPtr(true),
		ShowDetailedUsage: boolPtr(false),
	}))

	eff, err := LoadEffectiveSettings("")
	assert.NoError(t, err)
	assert.NotNil(t, eff.Model)
	assert.Equal(t, "claude-opus-4-6", *eff.Model)
	assert.NotNil(t, eff.ThinkingEffort)
	assert.Equal(t, "high", *eff.ThinkingEffort)
	assert.NotNil(t, eff.ShowThinking)
	assert.True(t, *eff.ShowThinking)
	assert.NotNil(t, eff.ShowDetailedUsage)
	assert.False(t, *eff.ShowDetailedUsage)

	path, err := UserSettingsPath()
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(root, ".dive", "settings.json"), path)
	info, err := os.Stat(path)
	assert.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestSaveUserSettingsRejectsNilUpdate(t *testing.T) {
	assert.Error(t, SaveUserSettings(nil))
}

func TestSaveUserSettingsPreservesUnrelatedKeys(t *testing.T) {
	useTestUserSettingsRoot(t)

	path, err := UserSettingsPath()
	assert.NoError(t, err)
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	assert.NoError(t, os.WriteFile(path, []byte(`{"model":"old","custom":{"keep":true}}`), 0o644))

	assert.NoError(t, SaveUserSettings(&Settings{ThinkingEffort: strPtr("low")}))

	eff, err := LoadEffectiveSettings("")
	assert.NoError(t, err)
	assert.Equal(t, "old", *eff.Model)
	assert.Equal(t, "low", *eff.ThinkingEffort)

	// Unknown keys survive the rewrite.
	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"keep"`)
}

func TestLoadEffectiveSettingsProjectOverridesUser(t *testing.T) {
	useTestUserSettingsRoot(t)
	assert.NoError(t, SaveUserSettings(&Settings{
		Model:          strPtr("user-model"),
		ThinkingEffort: strPtr("low"),
	}))

	project := t.TempDir()
	diveDir := filepath.Join(project, ".dive")
	assert.NoError(t, os.Mkdir(diveDir, 0o755))
	assert.NoError(t, os.WriteFile(
		filepath.Join(diveDir, "settings.json"),
		[]byte(`{"model":"project-model"}`), 0o644))

	eff, err := LoadEffectiveSettings(project)
	assert.NoError(t, err)
	assert.Equal(t, "project-model", *eff.Model)
	assert.Equal(t, "low", *eff.ThinkingEffort)
}

func TestLoadEffectiveSettingsKeepsSecurityConfigurationProjectScoped(t *testing.T) {
	useTestUserSettingsRoot(t)
	userPath, err := UserSettingsPath()
	assert.NoError(t, err)
	assert.NoError(t, os.MkdirAll(filepath.Dir(userPath), 0o700))
	assert.NoError(t, os.WriteFile(userPath, []byte(`{
  "model": "user-model",
  "permissions": {"allow": ["Bash(*)"]},
  "sandbox": {"enabled": false}
}`), 0o600))

	project := t.TempDir()
	diveDir := filepath.Join(project, ".dive")
	assert.NoError(t, os.Mkdir(diveDir, 0o755))
	assert.NoError(t, os.WriteFile(
		filepath.Join(diveDir, "settings.json"),
		[]byte(`{"permissions":{"deny":["Bash(rm:*)"]},"sandbox":{"enabled":true}}`),
		0o644,
	))

	eff, err := LoadEffectiveSettings(project)
	assert.NoError(t, err)
	assert.Equal(t, "user-model", *eff.Model)
	assert.Empty(t, eff.Permissions.Allow)
	assert.Equal(t, []string{"Bash(rm:*)"}, eff.Permissions.Deny)
	assert.NotNil(t, eff.Sandbox)
	assert.True(t, eff.Sandbox.Enabled)
}

func strPtr(s string) *string { return &s }

func boolPtr(b bool) *bool { return &b }
