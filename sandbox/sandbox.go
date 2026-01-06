package sandbox

import (
	"context"
	"os/exec"
)

// Config holds configuration for the sandbox manager.
type Config struct {
	// Enabled turns sandboxing on/off
	Enabled bool `json:"enabled"`

	// WorkDir is the project directory (mounted read-write)
	WorkDir string `json:"work_dir"`

	// AllowNetwork permits outbound network access
	AllowNetwork bool `json:"allow_network"`

	// AllowedWritePaths are additional writable paths outside WorkDir
	AllowedWritePaths []string `json:"allowed_write_paths"`

	// Environment variables to pass through to sandboxed process
	Environment map[string]string `json:"environment"`

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
}

type SeatbeltConfig struct {
	// Profile is "restrictive" or "permissive" (default: "restrictive")
	Profile string `json:"profile"`

	// CustomProfilePath overrides built-in profiles
	CustomProfilePath string `json:"custom_profile_path"`
}

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
	return &Manager{
		backends: []Backend{
			&SeatbeltBackend{},
			NewDockerBackend(),
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
	if !m.config.Enabled {
		return cmd, func() {}, nil
	}

	backend := m.SelectBackend()
	if backend == nil {
		// No sandbox backend available.
		// TODO: Should we warn or fail? The design doc says "Fallback: No sandboxing with warning"
		// For now, we return the command as is, effectively falling back to no sandbox.
		return cmd, func() {}, nil
	}

	return backend.WrapCommand(ctx, cmd, m.config)
}
