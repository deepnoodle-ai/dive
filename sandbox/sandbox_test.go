package sandbox

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestSeatbeltBackend_Available(t *testing.T) {
	backend := &SeatbeltBackend{}
	if runtime.GOOS == "darwin" {
		// Just check it doesn't panic. Actual availability depends on sandbox-exec presence.
		// Most macOS systems have it.
		_ = backend.Available()
	} else {
		assert.False(t, backend.Available())
	}
}

func TestDockerBackend_WrapCommand_Args(t *testing.T) {
	backend := NewDockerBackend()

	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/host/work",
		AllowNetwork: false,
		Docker: DockerConfig{
			Image: "test-image",
		},
	}

	cmd := exec.Command("echo", "hello")
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
	defer cleanup()

	// Convert args to string for easier searching
	argsStr := strings.Join(wrapped.Args, " ")

	assert.Contains(t, argsStr, "run")
	assert.Contains(t, argsStr, "--read-only")
	assert.Contains(t, argsStr, "--network none")
	assert.Contains(t, argsStr, "test-image")
	assert.Contains(t, argsStr, "--cidfile")
}

func TestSandbox_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Determine backend to test
	var backend Backend
	sb := &SeatbeltBackend{}
	db := NewDockerBackend()

	if sb.Available() {
		backend = sb
	} else if db.Available() {
		backend = db
	} else {
		t.Skip("No sandbox backend available")
	}

	t.Logf("Testing with backend: %s", backend.Name())

	tempDir := t.TempDir()
	cfg := &Config{
		Enabled:      true,
		WorkDir:      tempDir,
		AllowNetwork: false,
		Docker: DockerConfig{
			Image: "alpine:latest",
		},
	}

	mgr := NewManager(cfg)

	// Test 1: Write allowed file
	cmd := exec.Command("sh", "-c", "echo 'allowed' > allowed.txt")
	cmd.Dir = tempDir
	wrapped, cleanup, err := mgr.Wrap(context.Background(), cmd)
	assert.NoError(t, err)
	defer cleanup()

	output, err := wrapped.CombinedOutput()
	if err != nil {
		t.Logf("Command failed: %s", string(output))
	}
	assert.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(tempDir, "allowed.txt"))
	assert.NoError(t, statErr, "expected file to exist")

	// Test 2: Write denied file (outside workspace)
	// We use HomeDir because TmpDir is allowed by default in the profile.
	homeDir, err := os.UserHomeDir()
	assert.NoError(t, err)

	// Create a dummy file path in home dir (we won't actually write it hopefully)
	deniedPath := filepath.Join(homeDir, "dive-sandbox-test-denied.txt")

	// Ensure we don't overwrite anything existing (unlikely)
	if _, err := os.Stat(deniedPath); err == nil {
		t.Skipf("File %s exists, skipping negative test for safety", deniedPath)
	}

	cmd = exec.Command("sh", "-c", fmt.Sprintf("echo 'denied' > '%s'", deniedPath))
	cmd.Dir = tempDir
	wrapped, cleanup, err = mgr.Wrap(context.Background(), cmd)
	assert.NoError(t, err)
	defer cleanup()

	output, err = wrapped.CombinedOutput()
	// Should fail
	if err == nil {
		// Clean up if it somehow succeeded
		os.Remove(deniedPath)
		t.Errorf("Expected error writing to %s, but succeeded. Output: %s", deniedPath, output)
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

func TestNewManager_UsesConfiguredDockerCommand(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Docker: DockerConfig{
			Command: "podman",
		},
	}
	mgr := NewManager(cfg)
	var dockerBackend *DockerBackend
	for _, backend := range mgr.backends {
		if db, ok := backend.(*DockerBackend); ok {
			dockerBackend = db
			break
		}
	}
	assert.NotNil(t, dockerBackend)
	assert.Equal(t, "podman", dockerBackend.command)
}

func TestDockerBackend_RejectsInvalidAdditionalMount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows mount parsing differs")
	}

	backend := NewDockerBackend()
	cfg := &Config{
		Enabled: true,
		WorkDir: "/host/work",
		Docker: DockerConfig{
			AdditionalMounts: []string{"/tmp:/container:ro:extra"},
		},
	}

	cmd := exec.Command("echo", "hello")
	_, _, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.Error(t, err)
}

func TestDockerBackend_BlocksUnixSocketMounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets not supported on Windows")
	}

	tempDir, err := os.MkdirTemp("/tmp", "dive-sock-")
	if err != nil {
		tempDir = t.TempDir()
	}
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	socketPath := filepath.Join(tempDir, "test.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("unable to create unix socket for test: %v", err)
	}
	defer listener.Close()

	backend := NewDockerBackend()
	cfg := &Config{
		Enabled:           true,
		WorkDir:           tempDir,
		AllowedWritePaths: []string{socketPath},
	}

	cmd := exec.Command("echo", "hello")
	_, _, err = backend.WrapCommand(context.Background(), cmd, cfg)
	assert.Error(t, err)
}

// Test helpers for structured argument checking

// findFlagValue finds the value of a flag in an argument list.
// Returns the value and true if found, empty string and false otherwise.
func findFlagValue(args []string, flag string) (string, bool) {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// findEnvValue finds the value of an environment variable in --env arguments.
func findEnvValue(args []string, envName string) (string, bool) {
	prefix := envName + "="
	for i, arg := range args {
		if arg == "--env" && i+1 < len(args) {
			if strings.HasPrefix(args[i+1], prefix) {
				return strings.TrimPrefix(args[i+1], prefix), true
			}
		}
	}
	return "", false
}

// findAllFlagValues finds all values of a repeated flag in an argument list.
func findAllFlagValues(args []string, flag string) []string {
	var values []string
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			values = append(values, args[i+1])
		}
	}
	return values
}

// TestConvertPathForContainer tests Windows path conversion logic
// using the pure function that can be tested on any platform.
func TestConvertPathForContainer(t *testing.T) {
	tests := []struct {
		name     string
		hostPath string
		goos     string
		expected string
	}{
		{
			name:     "unix path unchanged on linux",
			hostPath: "/home/user/project",
			goos:     "linux",
			expected: "/home/user/project",
		},
		{
			name:     "unix path unchanged on darwin",
			hostPath: "/Users/user/project",
			goos:     "darwin",
			expected: "/Users/user/project",
		},
		{
			name:     "windows drive letter conversion",
			hostPath: "C:\\Users\\test\\project",
			goos:     "windows",
			expected: "/c/Users/test/project",
		},
		{
			name:     "windows uppercase drive converted to lowercase",
			hostPath: "D:\\data\\files",
			goos:     "windows",
			expected: "/d/data/files",
		},
		{
			name:     "windows forward slashes preserved",
			hostPath: "E:/work/code",
			goos:     "windows",
			expected: "/e/work/code",
		},
		{
			name:     "windows path without drive letter",
			hostPath: "\\relative\\path",
			goos:     "windows",
			expected: "/relative/path",
		},
		{
			name:     "windows drive root only",
			hostPath: "C:",
			goos:     "windows",
			expected: "/c",
		},
		{
			name:     "windows mixed slashes",
			hostPath: "C:\\Users/test\\project/src",
			goos:     "windows",
			expected: "/c/Users/test/project/src",
		},
		{
			name:     "empty path",
			hostPath: "",
			goos:     "windows",
			expected: "",
		},
		{
			name:     "single character non-drive",
			hostPath: "a",
			goos:     "windows",
			expected: "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertPathForContainer(tt.hostPath, tt.goos)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDockerBackend_NetworkEnabled tests Docker with network enabled
func TestDockerBackend_NetworkEnabled(t *testing.T) {
	backend := NewDockerBackend()

	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/host/work",
		AllowNetwork: true,
		Docker: DockerConfig{
			Image: "test-image",
		},
	}

	cmd := exec.Command("curl", "https://example.com")
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
	defer cleanup()

	argsStr := strings.Join(wrapped.Args, " ")

	// Should NOT contain --network none when network is enabled
	assert.NotContains(t, argsStr, "--network none")
	assert.Contains(t, argsStr, "run")
}

// TestDockerBackend_ProxyConfiguration tests proxy environment setup
func TestDockerBackend_ProxyConfiguration(t *testing.T) {
	backend := NewDockerBackend()

	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/host/work",
		AllowNetwork: true,
		Network: NetworkConfig{
			HTTPProxy:  "http://proxy.example.com:8080",
			HTTPSProxy: "https://proxy.example.com:8443",
			NoProxy:    []string{"localhost", "127.0.0.1"},
		},
		Docker: DockerConfig{
			Image: "test-image",
		},
	}

	cmd := exec.Command("curl", "https://example.com")
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
	defer cleanup()

	// Use structured arg checking for env vars
	httpProxy, ok := findEnvValue(wrapped.Args, "HTTP_PROXY")
	assert.True(t, ok, "HTTP_PROXY should be set")
	assert.Equal(t, "http://proxy.example.com:8080", httpProxy)

	httpsProxy, ok := findEnvValue(wrapped.Args, "HTTPS_PROXY")
	assert.True(t, ok, "HTTPS_PROXY should be set")
	assert.Equal(t, "https://proxy.example.com:8443", httpsProxy)

	noProxy, ok := findEnvValue(wrapped.Args, "NO_PROXY")
	assert.True(t, ok, "NO_PROXY should be set")
	assert.Equal(t, "localhost,127.0.0.1", noProxy)
}

// TestDockerBackend_AllowedDomains tests domain allowlist configuration
func TestDockerBackend_AllowedDomains(t *testing.T) {
	backend := NewDockerBackend()

	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/host/work",
		AllowNetwork: true,
		Network: NetworkConfig{
			AllowedDomains: []string{"api.example.com", "cdn.example.com"},
			HTTPSProxy:     "https://proxy.example.com:8443",
		},
		Docker: DockerConfig{
			Image: "test-image",
		},
	}

	cmd := exec.Command("curl", "https://api.example.com")
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
	defer cleanup()

	// Use structured arg checking
	domains, ok := findEnvValue(wrapped.Args, "DIVE_ALLOWED_DOMAINS")
	assert.True(t, ok, "DIVE_ALLOWED_DOMAINS should be set")
	assert.Equal(t, "api.example.com,cdn.example.com", domains)
}

// TestDockerBackend_AllowedDomainsAutoProxy tests that proxy is not required in config
func TestDockerBackend_AllowedDomainsAutoProxy(t *testing.T) {
	backend := NewDockerBackend()

	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/host/work",
		AllowNetwork: true,
		Network: NetworkConfig{
			AllowedDomains: []string{"api.example.com"},
			// No proxy configured - should NOT fail validation anymore
		},
		Docker: DockerConfig{
			Image: "test-image",
		},
	}

	cmd := exec.Command("curl", "https://api.example.com")
	_, _, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
}

type MockBackend struct {
	WrapCommandFunc func(ctx context.Context, cmd *exec.Cmd, cfg *Config) (*exec.Cmd, func(), error)
}

func (m *MockBackend) Name() string    { return "mock" }
func (m *MockBackend) Available() bool { return true }
func (m *MockBackend) WrapCommand(ctx context.Context, cmd *exec.Cmd, cfg *Config) (*exec.Cmd, func(), error) {
	if m.WrapCommandFunc != nil {
		return m.WrapCommandFunc(ctx, cmd, cfg)
	}
	return cmd, func() {}, nil
}

func TestManager_Wrap_StartsProxy(t *testing.T) {
	mockBackend := &MockBackend{}
	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/tmp",
		AllowNetwork: true,
		Network: NetworkConfig{
			AllowedDomains: []string{"example.com"},
		},
	}

	mgr := &Manager{
		backends: []Backend{mockBackend},
		config:   cfg,
	}

	// Verify that the backend receives a config with proxy set
	mockBackend.WrapCommandFunc = func(ctx context.Context, cmd *exec.Cmd, cfg *Config) (*exec.Cmd, func(), error) {
		assert.NotEmpty(t, cfg.Network.HTTPProxy, "HTTPProxy should be set by manager")
		assert.NotEmpty(t, cfg.Network.HTTPSProxy, "HTTPSProxy should be set by manager")
		assert.True(t, strings.HasPrefix(cfg.Network.HTTPProxy, "http://127.0.0.1:"), "Proxy should be local")
		return cmd, func() {}, nil
	}

	cmd := exec.Command("echo", "hello")
	_, cleanup, err := mgr.Wrap(context.Background(), cmd)
	assert.NoError(t, err)
	defer cleanup()
}

// TestDockerBackend_AllowedDomainsRequiresNetwork tests validation
func TestDockerBackend_AllowedDomainsRequiresNetwork(t *testing.T) {
	backend := NewDockerBackend()

	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/host/work",
		AllowNetwork: false, // Network disabled
		Network: NetworkConfig{
			AllowedDomains: []string{"api.example.com"},
			HTTPSProxy:     "https://proxy.example.com:8443",
		},
		Docker: DockerConfig{
			Image: "test-image",
		},
	}

	cmd := exec.Command("curl", "https://api.example.com")
	_, _, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "allow_network=true")
}

// TestSeatbeltBackend_CustomProfile tests custom Seatbelt profile loading
func TestSeatbeltBackend_CustomProfile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Seatbelt is macOS only")
	}

	backend := &SeatbeltBackend{}
	if !backend.Available() {
		t.Skip("sandbox-exec not available")
	}

	// Create a temporary custom profile
	tempDir := t.TempDir()
	customProfile := filepath.Join(tempDir, "custom.sb")

	profileContent := `(version 1)
(allow default)
(deny file-write* (subpath "/"))
(allow file-write* (subpath {{.WorkDir}}))
(allow file-write* (subpath {{.TmpDir}}))
{{range .AllowedWritePaths}}
(allow file-write* (subpath {{.}}))
{{end}}
`
	err := os.WriteFile(customProfile, []byte(profileContent), 0644)
	assert.NoError(t, err)

	cfg := &Config{
		Enabled: true,
		WorkDir: tempDir,
		Seatbelt: SeatbeltConfig{
			CustomProfilePath: customProfile,
		},
	}

	cmd := exec.Command("echo", "hello")
	cmd.Dir = tempDir
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
	defer cleanup()

	// Verify sandbox-exec is being used
	assert.Equal(t, "sandbox-exec", filepath.Base(wrapped.Path))
	assert.Contains(t, wrapped.Args, "-f")
}

// TestSeatbeltBackend_CustomProfileNotFound tests error handling for missing custom profile
func TestSeatbeltBackend_CustomProfileNotFound(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Seatbelt is macOS only")
	}

	backend := &SeatbeltBackend{}
	if !backend.Available() {
		t.Skip("sandbox-exec not available")
	}

	cfg := &Config{
		Enabled: true,
		WorkDir: t.TempDir(),
		Seatbelt: SeatbeltConfig{
			CustomProfilePath: "/nonexistent/path/to/profile.sb",
		},
	}

	cmd := exec.Command("echo", "hello")
	_, _, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "read custom profile")
}

// TestSeatbeltBackend_PermissiveProfile tests permissive profile selection
func TestSeatbeltBackend_PermissiveProfile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Seatbelt is macOS only")
	}

	backend := &SeatbeltBackend{}
	if !backend.Available() {
		t.Skip("sandbox-exec not available")
	}

	tempDir := t.TempDir()
	cfg := &Config{
		Enabled: true,
		WorkDir: tempDir,
		Seatbelt: SeatbeltConfig{
			Profile: "permissive",
		},
	}

	cmd := exec.Command("echo", "hello")
	cmd.Dir = tempDir
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
	defer cleanup()

	// Verify command was wrapped successfully
	assert.Equal(t, "sandbox-exec", filepath.Base(wrapped.Path))
}

// TestSeatbeltBackend_TmpAccess verifies /tmp is writable in the sandbox
func TestSeatbeltBackend_TmpAccess(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Seatbelt is macOS only")
	}

	backend := &SeatbeltBackend{}
	if !backend.Available() {
		t.Skip("sandbox-exec not available")
	}

	tempDir := t.TempDir()
	testFile := filepath.Join("/tmp", fmt.Sprintf("dive-seatbelt-test-%d.txt", os.Getpid()))

	cfg := &Config{
		Enabled: true,
		WorkDir: tempDir,
	}

	// Try to write to /tmp
	cmd := exec.Command("bash", "-c", fmt.Sprintf("echo test > %s && cat %s", testFile, testFile))
	cmd.Dir = tempDir
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
	defer cleanup()
	defer os.Remove(testFile)

	output, err := wrapped.CombinedOutput()
	assert.NoError(t, err, "sandbox should allow /tmp writes: %s", string(output))
	assert.Equal(t, "test\n", string(output))
}

// TestDockerBackend_ResourceLimits tests resource limit arguments
func TestDockerBackend_ResourceLimits(t *testing.T) {
	backend := NewDockerBackend()

	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/host/work",
		AllowNetwork: false,
		Docker: DockerConfig{
			Image:     "test-image",
			Memory:    "512m",
			CPUs:      "1.5",
			PidsLimit: 100,
		},
	}

	cmd := exec.Command("echo", "hello")
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
	defer cleanup()

	// Use structured arg checking
	memory, ok := findFlagValue(wrapped.Args, "--memory")
	assert.True(t, ok, "--memory flag should be set")
	assert.Equal(t, "512m", memory)

	cpus, ok := findFlagValue(wrapped.Args, "--cpus")
	assert.True(t, ok, "--cpus flag should be set")
	assert.Equal(t, "1.5", cpus)

	pidsLimit, ok := findFlagValue(wrapped.Args, "--pids-limit")
	assert.True(t, ok, "--pids-limit flag should be set")
	assert.Equal(t, "100", pidsLimit)
}

// TestDockerBackend_PortExposure tests port exposure arguments
func TestDockerBackend_PortExposure(t *testing.T) {
	backend := NewDockerBackend()

	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/host/work",
		AllowNetwork: true,
		Docker: DockerConfig{
			Image: "test-image",
			Ports: []string{"8080", "3000"},
		},
	}

	cmd := exec.Command("node", "server.js")
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
	defer cleanup()

	// Use structured arg checking for repeated flags
	publishPorts := findAllFlagValues(wrapped.Args, "--publish")
	assert.Contains(t, publishPorts, "8080:8080")
	assert.Contains(t, publishPorts, "3000:3000")
}

// TestDockerBackend_WorkdirFromCmd tests that cmd.Dir is respected
func TestDockerBackend_WorkdirFromCmd(t *testing.T) {
	backend := NewDockerBackend()

	cfg := &Config{
		Enabled:      true,
		WorkDir:      "/host/work",
		AllowNetwork: false,
		Docker: DockerConfig{
			Image: "test-image",
		},
	}

	cmd := exec.Command("ls", "-la")
	cmd.Dir = "/host/work/subdir"
	wrapped, cleanup, err := backend.WrapCommand(context.Background(), cmd, cfg)
	assert.NoError(t, err)
	defer cleanup()

	// Use structured arg checking
	workdir, ok := findFlagValue(wrapped.Args, "--workdir")
	assert.True(t, ok, "--workdir flag should be set")
	assert.Equal(t, "/host/work/subdir", workdir)
}
