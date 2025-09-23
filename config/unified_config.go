package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/enhanced"
	"github.com/deepnoodle-ai/dive/hooks"
	"github.com/deepnoodle-ai/dive/log"
	"github.com/deepnoodle-ai/dive/mcp"
	"github.com/deepnoodle-ai/dive/memory"
	"github.com/deepnoodle-ai/dive/permissions"
	"github.com/deepnoodle-ai/dive/settings"
	"github.com/deepnoodle-ai/dive/subagents"
	"github.com/goccy/go-yaml"
)

// UnifiedConfig represents the complete Dive configuration with all Claude Code-inspired features
type UnifiedConfig struct {
	// Base configuration (backward compatible)
	*Config

	// Settings hierarchy
	Settings *settings.Settings `yaml:"settings,omitempty" json:"settings,omitempty"`

	// Memory configuration
	Memory *MemoryConfig `yaml:"memory,omitempty" json:"memory,omitempty"`

	// Hooks configuration (can also be in settings)
	Hooks map[string][]HookConfig `yaml:"hooks,omitempty" json:"hooks,omitempty"`

	// Permission configuration (can also be in settings)
	Permissions *PermissionConfig `yaml:"permissions,omitempty" json:"permissions,omitempty"`

	// Subagent definitions
	Subagents map[string]*SubagentConfig `yaml:"subagents,omitempty" json:"subagents,omitempty"`

	// Enhanced MCP configuration
	MCPConfig *MCPConfiguration `yaml:"mcpConfig,omitempty" json:"mcpConfig,omitempty"`

	// Runtime state (not serialized)
	runtime *RuntimeState `yaml:"-" json:"-"`
}

// MemoryConfig defines memory system configuration
type MemoryConfig struct {
	Enabled           bool              `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	AutoLoad          bool              `yaml:"autoLoad,omitempty" json:"autoLoad,omitempty"`
	MaxImportDepth    int               `yaml:"maxImportDepth,omitempty" json:"maxImportDepth,omitempty"`
	CustomPaths       []string          `yaml:"customPaths,omitempty" json:"customPaths,omitempty"`
	ImportDirectories []string          `yaml:"importDirectories,omitempty" json:"importDirectories,omitempty"`
	ExcludePatterns   []string          `yaml:"excludePatterns,omitempty" json:"excludePatterns,omitempty"`
}

// HookConfig defines a hook execution configuration
type HookConfig struct {
	Event    string       `yaml:"event" json:"event"`
	Matcher  string       `yaml:"matcher,omitempty" json:"matcher,omitempty"`
	Actions  []HookAction `yaml:"actions" json:"actions"`
}

// HookAction represents a single hook action
type HookAction struct {
	Type    string `yaml:"type" json:"type"`
	Command string `yaml:"command" json:"command"`
	Timeout int    `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Async   bool   `yaml:"async,omitempty" json:"async,omitempty"`
}

// PermissionConfig defines permission system configuration
type PermissionConfig struct {
	DefaultMode             string   `yaml:"defaultMode,omitempty" json:"defaultMode,omitempty"`
	Allow                   []string `yaml:"allow,omitempty" json:"allow,omitempty"`
	Ask                     []string `yaml:"ask,omitempty" json:"ask,omitempty"`
	Deny                    []string `yaml:"deny,omitempty" json:"deny,omitempty"`
	AdditionalDirectories   []string `yaml:"additionalDirectories,omitempty" json:"additionalDirectories,omitempty"`
	DisableBypassMode       bool     `yaml:"disableBypassMode,omitempty" json:"disableBypassMode,omitempty"`
	AutoApproveTimeout      int      `yaml:"autoApproveTimeout,omitempty" json:"autoApproveTimeout,omitempty"`
}

// SubagentConfig defines a subagent configuration
type SubagentConfig struct {
	Name         string            `yaml:"name" json:"name"`
	Description  string            `yaml:"description" json:"description"`
	Model        string            `yaml:"model,omitempty" json:"model,omitempty"`
	Tools        []string          `yaml:"tools,omitempty" json:"tools,omitempty"`
	SystemPrompt string            `yaml:"systemPrompt" json:"systemPrompt"`
	AutoInvoke   []string          `yaml:"autoInvoke,omitempty" json:"autoInvoke,omitempty"`
	MaxTokens    int               `yaml:"maxTokens,omitempty" json:"maxTokens,omitempty"`
	Temperature  float64           `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	Permissions  *PermissionConfig `yaml:"permissions,omitempty" json:"permissions,omitempty"`
}

// MCPConfiguration defines enhanced MCP configuration
type MCPConfiguration struct {
	EnableAllProjectServers  bool                          `yaml:"enableAllProjectServers,omitempty" json:"enableAllProjectServers,omitempty"`
	EnabledServers          []string                      `yaml:"enabledServers,omitempty" json:"enabledServers,omitempty"`
	DisabledServers         []string                      `yaml:"disabledServers,omitempty" json:"disabledServers,omitempty"`
	ServerConfigs           map[string]*MCPServerEnhanced `yaml:"serverConfigs,omitempty" json:"serverConfigs,omitempty"`
	AutoDiscovery           bool                          `yaml:"autoDiscovery,omitempty" json:"autoDiscovery,omitempty"`
	DiscoveryPaths          []string                      `yaml:"discoveryPaths,omitempty" json:"discoveryPaths,omitempty"`
}

// MCPServerEnhanced extends MCPServer with additional features
type MCPServerEnhanced struct {
	*MCPServer
	AutoStart     bool              `yaml:"autoStart,omitempty" json:"autoStart,omitempty"`
	RestartOnFail bool              `yaml:"restartOnFail,omitempty" json:"restartOnFail,omitempty"`
	MaxRetries    int               `yaml:"maxRetries,omitempty" json:"maxRetries,omitempty"`
	Permissions   *PermissionConfig `yaml:"permissions,omitempty" json:"permissions,omitempty"`
}

// RuntimeState holds runtime state for the configuration
type RuntimeState struct {
	MemoryManager      *memory.Memory
	HookManager        *hooks.HookManager
	PermissionManager  *permissions.PermissionManager
	SubagentManager    *subagents.SubagentManager
	SettingsManager    *settings.SettingsManager
	MCPManager         *mcp.Manager
	Logger             log.Logger
	Context            context.Context
}

// LoadUnifiedConfig loads a unified configuration from multiple sources
func LoadUnifiedConfig(ctx context.Context, path string) (*UnifiedConfig, error) {
	uc := &UnifiedConfig{
		runtime: &RuntimeState{
			Context: ctx,
			Logger:  log.New(log.GetDefaultLevel()),
		},
	}

	// Load base configuration if path provided
	if path != "" {
		cfg, err := LoadDirectory(path)
		if err != nil {
			// Try loading as a single file
			cfg, err = ParseFile(path)
			if err != nil {
				return nil, fmt.Errorf("failed to load config: %w", err)
			}
		}
		uc.Config = cfg
	} else {
		// Create default config
		uc.Config = &Config{}
	}

	// Load settings hierarchy
	uc.runtime.SettingsManager = settings.NewSettingsManager()
	if err := uc.runtime.SettingsManager.Load(); err != nil {
		// Settings are optional, log warning but continue
		uc.runtime.Logger.Warn("Failed to load settings: %v", err)
	}
	uc.Settings = uc.runtime.SettingsManager.GetMerged()

	// Merge settings into config if present
	if uc.Settings != nil {
		uc.mergeSettingsIntoConfig()
	}

	// Initialize memory system
	if uc.Memory == nil {
		uc.Memory = &MemoryConfig{
			Enabled:  true,
			AutoLoad: true,
		}
	}

	if uc.Memory.Enabled {
		uc.runtime.MemoryManager = memory.NewMemory()
		if uc.Memory.AutoLoad {
			if err := uc.runtime.MemoryManager.Load(); err != nil {
				uc.runtime.Logger.Warn("Failed to load memory: %v", err)
			}
		}
	}

	// Initialize hooks
	if uc.Settings != nil && uc.Settings.Hooks != nil {
		// Convert settings hooks to our hook format
		uc.Hooks = convertSettingsHooks(uc.Settings.Hooks)
	}

	if uc.Hooks != nil {
		uc.runtime.HookManager = hooks.NewHookManager(uc.Settings)
	}

	// Initialize permissions
	if uc.Permissions == nil && uc.Settings != nil && uc.Settings.Permissions != nil {
		uc.Permissions = convertSettingsPermissions(uc.Settings.Permissions)
	}

	if uc.Permissions != nil {
		workDir, _ := os.Getwd()
		uc.runtime.PermissionManager = permissions.NewPermissionManager(
			convertToPermissionSettings(uc.Permissions),
			workDir,
		)
	}

	// Initialize subagents
	uc.runtime.SubagentManager = subagents.NewSubagentManager()
	if err := uc.runtime.SubagentManager.Load(); err != nil {
		uc.runtime.Logger.Warn("Failed to load subagents: %v", err)
	}

	// Initialize MCP
	if uc.MCPConfig == nil {
		uc.MCPConfig = &MCPConfiguration{}
	}

	if uc.Settings != nil && len(uc.Settings.MCPServers) > 0 {
		// Merge settings MCP servers
		if uc.MCPConfig.ServerConfigs == nil {
			uc.MCPConfig.ServerConfigs = make(map[string]*MCPServerEnhanced)
		}
		for name, server := range uc.Settings.MCPServers {
			uc.MCPConfig.ServerConfigs[name] = &MCPServerEnhanced{
				MCPServer: convertSettingsMCPServer(server),
				AutoStart: true,
			}
		}
	}

	return uc, nil
}

// mergeSettingsIntoConfig merges settings into the main config
func (uc *UnifiedConfig) mergeSettingsIntoConfig() {
	if uc.Settings == nil {
		return
	}

	// Merge model settings
	if uc.Settings.Model != "" && uc.Config.Config.DefaultModel == "" {
		uc.Config.Config.DefaultModel = uc.Settings.Model
	}

	if uc.Settings.DefaultProvider != "" && uc.Config.Config.DefaultProvider == "" {
		uc.Config.Config.DefaultProvider = uc.Settings.DefaultProvider
	}

	// Merge environment variables
	if len(uc.Settings.Env) > 0 {
		for key, value := range uc.Settings.Env {
			os.Setenv(key, value)
		}
	}
}

// CreateEnhancedEnvironment creates an enhanced environment with all features
func (uc *UnifiedConfig) CreateEnhancedEnvironment(ctx context.Context, opts EnvironmentOpts) (*enhanced.Environment, error) {
	// Create base environment
	baseEnv, err := NewEnvironment(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Enhance with our features
	enhancedEnv := &enhanced.Environment{
		BaseEnvironment:   &environmentAdapter{baseEnv},
		MemoryManager:     uc.runtime.MemoryManager,
		HookManager:       uc.runtime.HookManager,
		PermissionManager: uc.runtime.PermissionManager,
		SubagentManager:   uc.runtime.SubagentManager,
		SettingsManager:   uc.runtime.SettingsManager,
		UnifiedConfig:     uc,
		Agents:            baseEnv.Agents,
		Tools:             baseEnv.Tools,
	}

	// Enhance agents with new capabilities
	for i, agent := range baseEnv.Agents {
		baseEnv.Agents[i] = enhancedEnv.EnhanceAgent(agent)
	}

	return enhancedEnv, nil
}


// environmentAdapter wraps Environment to implement enhanced.BaseEnvironment
type environmentAdapter struct {
	*Environment
}

func (ea *environmentAdapter) GetAgents() []dive.Agent {
	return ea.Agents
}

func (ea *environmentAdapter) GetTools() map[string]dive.Tool {
	return ea.Tools
}

func (ea *environmentAdapter) GetMCPManager() *mcp.Manager {
	return ea.MCPManager
}

func (ea *environmentAdapter) GetLogger() interface{} {
	return ea.Logger
}

func (ea *environmentAdapter) GetThreads() dive.ThreadRepository {
	return ea.Threads
}

func (ea *environmentAdapter) GetConfirmer() dive.Confirmer {
	return ea.Confirmer
}

func (ea *environmentAdapter) GetDirectory() string {
	return ea.Directory
}

// Helper conversion functions
func convertSettingsHooks(settingsHooks map[string][]settings.HookConfig) map[string][]HookConfig {
	result := make(map[string][]HookConfig)
	for event, configs := range settingsHooks {
		var hookConfigs []HookConfig
		for _, cfg := range configs {
			actions := make([]HookAction, len(cfg.Hooks))
			for i, hook := range cfg.Hooks {
				actions[i] = HookAction{
					Type:    hook.Type,
					Command: hook.Command,
					Timeout: hook.Timeout,
				}
			}
			hookConfigs = append(hookConfigs, HookConfig{
				Event:   event,
				Matcher: cfg.Matcher,
				Actions: actions,
			})
		}
		result[event] = hookConfigs
	}
	return result
}

func convertSettingsPermissions(sp *settings.PermissionSettings) *PermissionConfig {
	return &PermissionConfig{
		DefaultMode:           sp.DefaultMode,
		Allow:                 sp.Allow,
		Ask:                   sp.Ask,
		Deny:                  sp.Deny,
		AdditionalDirectories: sp.AdditionalDirectories,
		DisableBypassMode:     sp.DisableBypassPermissions == "disable",
	}
}

func convertToPermissionSettings(pc *PermissionConfig) *settings.PermissionSettings {
	disableBypass := ""
	if pc.DisableBypassMode {
		disableBypass = "disable"
	}
	return &settings.PermissionSettings{
		DefaultMode:              pc.DefaultMode,
		Allow:                    pc.Allow,
		Ask:                      pc.Ask,
		Deny:                     pc.Deny,
		AdditionalDirectories:    pc.AdditionalDirectories,
		DisableBypassPermissions: disableBypass,
	}
}

func convertSettingsMCPServer(server *settings.MCPServerSettings) *MCPServer {
	return &MCPServer{
		Type:    server.Type,
		Name:    server.Type, // Use type as name if not specified
		Command: server.Command,
		URL:     server.URL,
		Env:     server.Env,
		Args:    server.Args,
		Headers: server.Headers,
	}
}

// Save saves the unified configuration
func (uc *UnifiedConfig) Save(path string) error {
	ext := strings.ToLower(filepath.Ext(path))

	data, err := uc.Marshal(ext)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Marshal marshals the configuration to bytes
func (uc *UnifiedConfig) Marshal(format string) ([]byte, error) {
	switch format {
	case ".json":
		return json.MarshalIndent(uc, "", "  ")
	case ".yml", ".yaml":
		return yaml.Marshal(uc)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}