package sandbox

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path"
	"strings"

	"github.com/deepnoodle-ai/dive/sandbox/proxy"
)

// Config holds configuration for the sandbox manager.
type Config struct {
	// Enabled turns sandboxing on/off
	Enabled bool `json:"enabled"`

	// Mode controls how sandboxing interacts with approvals (regular or auto)
	Mode SandboxMode `json:"mode"`

	// WorkDir is the project directory (mounted read-write)
	WorkDir string `json:"work_dir"`

	// AllowNetwork permits outbound network access
	AllowNetwork bool `json:"allow_network"`

	// Network holds proxy and domain allowlist configuration
	Network NetworkConfig `json:"network"`

	// AllowedWritePaths are additional writable paths outside WorkDir
	AllowedWritePaths []string `json:"allowed_write_paths"`

	// AllowedUnixSockets are unix socket paths allowed for mounting (Docker only)
	AllowedUnixSockets []string `json:"allowed_unix_sockets"`

	// Environment variables to pass through to sandboxed process
	Environment map[string]string `json:"environment"`

	// ExcludedCommands are shell commands that must run outside the sandbox
	ExcludedCommands []string `json:"excluded_commands"`

	// AllowUnsandboxedCommands enables running excluded commands outside the sandbox
	AllowUnsandboxedCommands bool `json:"allow_unsandboxed_commands"`

	// MaxCommandDurationMs caps command execution time in milliseconds
	MaxCommandDurationMs int `json:"max_command_duration_ms"`

	// AuditLog enables sandbox audit logging
	AuditLog bool `json:"audit_log"`

	// MountCloudCredentials enables mounting of standard cloud credential paths (read-only)
	// e.g., ~/.config/gcloud, ~/.aws/credentials
	MountCloudCredentials bool `json:"mount_cloud_credentials"`

	// Docker-specific options
	Docker DockerConfig `json:"docker"`

	// Seatbelt-specific options
	Seatbelt SeatbeltConfig `json:"seatbelt"`
}

type DockerConfig struct {
	// Image is the container image (default: "ubuntu:22.04")
	Image string `json:"image"`

	// Command is "docker" or "podman" (auto-detected if empty)
	Command string `json:"command"`

	// AdditionalMounts are extra volume mounts ("host:container:opts")
	AdditionalMounts []string `json:"additional_mounts"`

	// Ports to expose on the host
	Ports []string `json:"ports"`

	// EnableUserMapping enables UID/GID mapping for Linux (requires 'shadow-utils' in image)
	EnableUserMapping bool `json:"enable_user_mapping"`

	// Memory limits container memory (e.g. "512m")
	Memory string `json:"memory"`

	// CPUs limits container CPU usage (e.g. "1.5")
	CPUs string `json:"cpus"`

	// PidsLimit limits the number of processes in the container
	PidsLimit int `json:"pids_limit"`
}

type SeatbeltConfig struct {
	// Profile is "restrictive" or "permissive" (default: "restrictive")
	Profile string `json:"profile"`

	// CustomProfilePath overrides built-in profiles
	CustomProfilePath string `json:"custom_profile_path"`
}

type NetworkConfig struct {
	// AllowedDomains lists domains permitted via proxy (enforced by proxy)
	AllowedDomains []string `json:"allowed_domains"`

	// HTTPProxy is the proxy URL for HTTP traffic
	HTTPProxy string `json:"http_proxy"`

	// HTTPSProxy is the proxy URL for HTTPS traffic
	HTTPSProxy string `json:"https_proxy"`

	// NoProxy lists hosts that should bypass the proxy
	NoProxy []string `json:"no_proxy"`
}

type SandboxMode string

const (
	SandboxModeRegular   SandboxMode = "regular"
	SandboxModeAutoAllow SandboxMode = "auto"
)

// Backend represents a sandboxing implementation
type Backend interface {
	// Name returns the backend identifier
	Name() string

	// Available checks if this backend can be used
	Available() bool

	// WrapCommand wraps a command for sandboxed execution
	WrapCommand(ctx context.Context, cmd *exec.Cmd, cfg *Config) (*exec.Cmd, func(), error)
}

// Manager manages sandbox backends and wraps commands.
type Manager struct {
	backends []Backend
	config   *Config
}

// NewManager creates a new sandbox manager with the given configuration.
func NewManager(cfg *Config) *Manager {
	dockerBackend := NewDockerBackend()
	if cfg != nil && cfg.Docker.Command != "" {
		dockerBackend.command = cfg.Docker.Command
	}
	return &Manager{
		backends: []Backend{
			&SeatbeltBackend{},
			dockerBackend,
		},
		config: cfg,
	}
}

// Config returns the current configuration.
func (m *Manager) Config() *Config {
	return m.config
}

// SelectBackend returns the first available backend, or nil if none are available.
func (m *Manager) SelectBackend() Backend {
	for _, b := range m.backends {
		if b.Available() {
			return b
		}
	}
	return nil
}

// Wrap wraps a command for sandboxed execution if sandboxing is enabled.
func (m *Manager) Wrap(ctx context.Context, cmd *exec.Cmd) (*exec.Cmd, func(), error) {
	if m.config == nil || !m.config.Enabled {
		return cmd, func() {}, nil
	}

	backend := m.SelectBackend()
	if backend == nil {
		if m.config.AuditLog {
			log.Printf("sandbox: no backend available; command=%s", cmd.Path)
		}
		return cmd, func() {}, fmt.Errorf("sandboxing is enabled but no backend is available")
	}

	cfg := m.config
	var proxyCleanup func()

	// If allowed domains are configured, start the internal proxy and configure the
	// command to use it. This overrides any existing proxy configuration in the
	// command's environment for this specific execution.
	if len(cfg.Network.AllowedDomains) > 0 {
		// Clone config to avoid modifying shared state
		cloned := *cfg
		cfg = &cloned

		// Start proxy
		p := proxy.New(cfg.Network.AllowedDomains, m.config.AuditLog)
		addr, err := p.Start()
		if err != nil {
			return nil, func() {}, fmt.Errorf("failed to start sandbox proxy: %w", err)
		}

		proxyURL := "http://" + addr
		cfg.Network.HTTPProxy = proxyURL
		cfg.Network.HTTPSProxy = proxyURL

		proxyCleanup = func() {
			p.Stop()
		}
	}

	if m.config.AuditLog {
		log.Printf("sandbox: wrapping command=%s backend=%s", cmd.Path, backend.Name())
	}

	wrappedCmd, backendCleanup, err := backend.WrapCommand(ctx, cmd, cfg)
	if err != nil {
		if proxyCleanup != nil {
			proxyCleanup()
		}
		return nil, func() {}, err
	}

	finalCleanup := func() {
		backendCleanup()
		if proxyCleanup != nil {
			proxyCleanup()
		}
	}

	return wrappedCmd, finalCleanup, nil
}

// MatchesCommandPattern checks if a command matches a pattern.
// Pattern matching rules:
//   - If pattern ends with " *", treat as prefix match on command name.
//     e.g., "docker *" matches "docker run ..." and "docker" alone.
//   - If pattern contains glob chars (*?[]), match against the first word only.
//   - Otherwise, check if command starts with pattern.
func MatchesCommandPattern(pattern, command string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	// If pattern ends with " *", treat as prefix match on command name.
	// This handles patterns like "docker *" to match "docker run ..." etc.
	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, " *")
		return strings.HasPrefix(command, prefix+" ") || command == prefix
	}
	if strings.ContainsAny(pattern, "*?[]") {
		// For glob-like patterns, match against the first word (command name) only
		cmdParts := strings.Fields(command)
		if len(cmdParts) == 0 {
			return false
		}
		ok, err := path.Match(pattern, cmdParts[0])
		return err == nil && ok
	}
	return strings.HasPrefix(command, pattern)
}
