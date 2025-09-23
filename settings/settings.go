package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// Settings represents hierarchical configuration for Dive
// Inspired by Claude Code's settings system
type Settings struct {
	// Permission settings
	Permissions *PermissionSettings `yaml:"permissions,omitempty" json:"permissions,omitempty"`

	// Environment variables to apply
	Env map[string]string `yaml:"env,omitempty" json:"env,omitempty"`

	// Hooks configuration
	Hooks map[string][]HookConfig `yaml:"hooks,omitempty" json:"hooks,omitempty"`

	// MCP servers configuration
	MCPServers map[string]*MCPServerSettings `yaml:"mcpServers,omitempty" json:"mcpServers,omitempty"`

	// Model configuration
	Model string `yaml:"model,omitempty" json:"model,omitempty"`

	// Default provider
	DefaultProvider string `yaml:"defaultProvider,omitempty" json:"defaultProvider,omitempty"`

	// Status line configuration
	StatusLine *StatusLineConfig `yaml:"statusLine,omitempty" json:"statusLine,omitempty"`

	// Output style for responses
	OutputStyle string `yaml:"outputStyle,omitempty" json:"outputStyle,omitempty"`

	// Enable all project MCP servers automatically
	EnableAllProjectMCPServers bool `yaml:"enableAllProjectMcpServers,omitempty" json:"enableAllProjectMcpServers,omitempty"`

	// List of enabled MCP servers from .mcp.json
	EnabledMCPJSONServers []string `yaml:"enabledMcpjsonServers,omitempty" json:"enabledMcpjsonServers,omitempty"`

	// List of disabled MCP servers from .mcp.json
	DisabledMCPJSONServers []string `yaml:"disabledMcpjsonServers,omitempty" json:"disabledMcpjsonServers,omitempty"`

	// Disable all hooks
	DisableAllHooks bool `yaml:"disableAllHooks,omitempty" json:"disableAllHooks,omitempty"`

	// Cleanup period in days for old sessions
	CleanupPeriodDays int `yaml:"cleanupPeriodDays,omitempty" json:"cleanupPeriodDays,omitempty"`

	// Include co-authored-by in commits and PRs
	IncludeCoAuthoredBy *bool `yaml:"includeCoAuthoredBy,omitempty" json:"includeCoAuthoredBy,omitempty"`

	// API key helper script
	APIKeyHelper string `yaml:"apiKeyHelper,omitempty" json:"apiKeyHelper,omitempty"`

	// AWS auth refresh script
	AWSAuthRefresh string `yaml:"awsAuthRefresh,omitempty" json:"awsAuthRefresh,omitempty"`

	// AWS credential export script
	AWSCredentialExport string `yaml:"awsCredentialExport,omitempty" json:"awsCredentialExport,omitempty"`
}

// PermissionSettings controls tool access permissions
type PermissionSettings struct {
	Allow                   []string `yaml:"allow,omitempty" json:"allow,omitempty"`
	Ask                     []string `yaml:"ask,omitempty" json:"ask,omitempty"`
	Deny                    []string `yaml:"deny,omitempty" json:"deny,omitempty"`
	AdditionalDirectories   []string `yaml:"additionalDirectories,omitempty" json:"additionalDirectories,omitempty"`
	DefaultMode             string   `yaml:"defaultMode,omitempty" json:"defaultMode,omitempty"`
	DisableBypassPermissions string   `yaml:"disableBypassPermissionsMode,omitempty" json:"disableBypassPermissionsMode,omitempty"`
}

// HookConfig defines a hook execution configuration
type HookConfig struct {
	Matcher string       `yaml:"matcher,omitempty" json:"matcher,omitempty"`
	Hooks   []HookAction `yaml:"hooks,omitempty" json:"hooks,omitempty"`
}

// HookAction represents a single hook action
type HookAction struct {
	Type    string `yaml:"type" json:"type"`
	Command string `yaml:"command" json:"command"`
	Timeout int    `yaml:"timeout,omitempty" json:"timeout,omitempty"`
}

// MCPServerSettings defines MCP server configuration
type MCPServerSettings struct {
	Type               string            `yaml:"type,omitempty" json:"type,omitempty"`
	Command            string            `yaml:"command,omitempty" json:"command,omitempty"`
	Args               []string          `yaml:"args,omitempty" json:"args,omitempty"`
	URL                string            `yaml:"url,omitempty" json:"url,omitempty"`
	Env                map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Headers            map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
}

// StatusLineConfig defines status line configuration
type StatusLineConfig struct {
	Type    string `yaml:"type" json:"type"`
	Command string `yaml:"command,omitempty" json:"command,omitempty"`
}

// SettingsManager manages hierarchical settings loading
type SettingsManager struct {
	userSettings       *Settings
	projectSettings    *Settings
	localSettings      *Settings
	enterpriseSettings *Settings
	merged             *Settings
}

// NewSettingsManager creates a new settings manager
func NewSettingsManager() *SettingsManager {
	return &SettingsManager{}
}

// Load loads all settings from their respective locations
func (sm *SettingsManager) Load() error {
	// Load enterprise settings (highest priority)
	if enterprisePath := getEnterpriseSettingsPath(); enterprisePath != "" {
		if settings, err := loadSettingsFile(enterprisePath); err == nil {
			sm.enterpriseSettings = settings
		}
	}

	// Load user settings
	if userPath := getUserSettingsPath(); userPath != "" {
		if settings, err := loadSettingsFile(userPath); err == nil {
			sm.userSettings = settings
		}
	}

	// Load project settings (from current directory)
	if projectPath := getProjectSettingsPath(); projectPath != "" {
		if settings, err := loadSettingsFile(projectPath); err == nil {
			sm.projectSettings = settings
		}
	}

	// Load local project settings
	if localPath := getLocalSettingsPath(); localPath != "" {
		if settings, err := loadSettingsFile(localPath); err == nil {
			sm.localSettings = settings
		}
	}

	// Merge settings in precedence order
	sm.merged = sm.mergeSettings()

	return nil
}

// GetMerged returns the merged settings
func (sm *SettingsManager) GetMerged() *Settings {
	if sm.merged == nil {
		sm.merged = sm.mergeSettings()
	}
	return sm.merged
}

// mergeSettings merges all settings in precedence order
func (sm *SettingsManager) mergeSettings() *Settings {
	result := &Settings{}

	// Apply in order: user -> project -> local -> enterprise
	// Lower priority settings are applied first, then overridden
	if sm.userSettings != nil {
		result = mergeSettingsObjects(result, sm.userSettings)
	}
	if sm.projectSettings != nil {
		result = mergeSettingsObjects(result, sm.projectSettings)
	}
	if sm.localSettings != nil {
		result = mergeSettingsObjects(result, sm.localSettings)
	}
	if sm.enterpriseSettings != nil {
		result = mergeSettingsObjects(result, sm.enterpriseSettings)
	}

	return result
}

// mergeSettingsObjects merges two settings objects
func mergeSettingsObjects(base, override *Settings) *Settings {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	result := *base

	// Merge simple fields
	if override.Model != "" {
		result.Model = override.Model
	}
	if override.DefaultProvider != "" {
		result.DefaultProvider = override.DefaultProvider
	}
	if override.OutputStyle != "" {
		result.OutputStyle = override.OutputStyle
	}
	if override.APIKeyHelper != "" {
		result.APIKeyHelper = override.APIKeyHelper
	}
	if override.AWSAuthRefresh != "" {
		result.AWSAuthRefresh = override.AWSAuthRefresh
	}
	if override.AWSCredentialExport != "" {
		result.AWSCredentialExport = override.AWSCredentialExport
	}
	if override.CleanupPeriodDays > 0 {
		result.CleanupPeriodDays = override.CleanupPeriodDays
	}
	if override.IncludeCoAuthoredBy != nil {
		result.IncludeCoAuthoredBy = override.IncludeCoAuthoredBy
	}

	// Merge environment variables
	if len(override.Env) > 0 {
		if result.Env == nil {
			result.Env = make(map[string]string)
		}
		for k, v := range override.Env {
			result.Env[k] = v
		}
	}

	// Merge permissions
	if override.Permissions != nil {
		if result.Permissions == nil {
			result.Permissions = &PermissionSettings{}
		}
		if len(override.Permissions.Allow) > 0 {
			result.Permissions.Allow = override.Permissions.Allow
		}
		if len(override.Permissions.Ask) > 0 {
			result.Permissions.Ask = override.Permissions.Ask
		}
		if len(override.Permissions.Deny) > 0 {
			result.Permissions.Deny = override.Permissions.Deny
		}
		if len(override.Permissions.AdditionalDirectories) > 0 {
			result.Permissions.AdditionalDirectories = override.Permissions.AdditionalDirectories
		}
		if override.Permissions.DefaultMode != "" {
			result.Permissions.DefaultMode = override.Permissions.DefaultMode
		}
		if override.Permissions.DisableBypassPermissions != "" {
			result.Permissions.DisableBypassPermissions = override.Permissions.DisableBypassPermissions
		}
	}

	// Merge hooks
	if len(override.Hooks) > 0 {
		if result.Hooks == nil {
			result.Hooks = make(map[string][]HookConfig)
		}
		for event, configs := range override.Hooks {
			result.Hooks[event] = configs
		}
	}

	// Merge MCP servers
	if len(override.MCPServers) > 0 {
		if result.MCPServers == nil {
			result.MCPServers = make(map[string]*MCPServerSettings)
		}
		for name, config := range override.MCPServers {
			result.MCPServers[name] = config
		}
	}

	// Merge status line
	if override.StatusLine != nil {
		result.StatusLine = override.StatusLine
	}

	// Merge MCP server lists
	if len(override.EnabledMCPJSONServers) > 0 {
		result.EnabledMCPJSONServers = override.EnabledMCPJSONServers
	}
	if len(override.DisabledMCPJSONServers) > 0 {
		result.DisabledMCPJSONServers = override.DisabledMCPJSONServers
	}

	// Merge boolean flags
	if override.EnableAllProjectMCPServers {
		result.EnableAllProjectMCPServers = override.EnableAllProjectMCPServers
	}
	if override.DisableAllHooks {
		result.DisableAllHooks = override.DisableAllHooks
	}

	return &result
}

// Helper functions to get settings paths
func getEnterpriseSettingsPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/Dive/managed-settings.json"
	case "windows":
		return `C:\ProgramData\Dive\managed-settings.json`
	default: // Linux and others
		return "/etc/dive/managed-settings.json"
	}
}

func getUserSettingsPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".dive", "settings.json")
}

func getProjectSettingsPath() string {
	// Check both .dive/settings.json and .dive/settings.yaml
	for _, name := range []string{".dive/settings.json", ".dive/settings.yaml"} {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	return ""
}

func getLocalSettingsPath() string {
	// Check for local settings
	for _, name := range []string{".dive/settings.local.json", ".dive/settings.local.yaml"} {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	return ""
}

// loadSettingsFile loads a settings file (JSON or YAML)
func loadSettingsFile(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var settings Settings
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".json":
		err = json.Unmarshal(data, &settings)
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, &settings)
	default:
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}

	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", path, err)
	}

	return &settings, nil
}

// SaveUserSettings saves user settings
func SaveUserSettings(settings *Settings) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	settingsDir := filepath.Join(homeDir, ".dive")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return err
	}

	settingsPath := filepath.Join(settingsDir, "settings.json")
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0644)
}

// SaveProjectSettings saves project settings
func SaveProjectSettings(settings *Settings) error {
	settingsDir := ".dive"
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return err
	}

	settingsPath := filepath.Join(settingsDir, "settings.json")
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0644)
}