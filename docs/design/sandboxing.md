# Sandboxing Design Document

> **Note**: This is a design document. The implementation lives in `experimental/sandbox/` and may differ from some details described here. See the [sandboxing guide](../guides/experimental/sandboxing.md) for current usage.

This document describes the design for sandboxed command execution in Dive, providing secure isolation for AI agent tool execution across different platforms.

## Overview

When AI agents execute shell commands via tools like `BashTool`, there's inherent risk that malicious or buggy commands could damage the host system, exfiltrate data, or cause other unintended effects. Sandboxing provides defense-in-depth by restricting what sandboxed processes can do at the OS level, independent of application-level permission checks.

### Goals

1. **Filesystem isolation** - Restrict write access to the project directory and temporary files
2. **Optional network isolation** - Control outbound network access
3. **Process isolation** - Prevent escape from the sandbox
4. **Cross-platform support** - Work on macOS, Linux, and optionally Windows
5. **Minimal friction** - Easy to enable with sensible defaults
6. **Transparency** - Clear feedback when sandbox violations occur

### Non-Goals

1. Perfect security against determined attackers (defense-in-depth, not absolute security)
2. GUI application support
3. Real-time filesystem monitoring/IDS capabilities
4. ~~Fine-grained network filtering~~ (now implemented via built-in proxy)

## Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│                      SandboxManager                         │
│  - Detects platform & available backends                    │
│  - Selects appropriate backend based on availability        │
│  - Provides unified Wrap() interface                        │
└─────────────────────┬───────────────────────────────────────┘
                      │
         ┌────────────┴────────────┐
         │                         │
         ▼                         ▼
┌─────────────────┐       ┌─────────────────┐
│ SeatbeltBackend │       │  DockerBackend  │
│   (macOS)       │       │ (Linux/Windows) │
│                 │       │                 │
│ sandbox-exec    │       │ docker/podman   │
│ + .sb profiles  │       │ + volume mounts │
└─────────────────┘       └─────────────────┘
```

### Backend Selection Priority

1. **macOS**: Seatbelt (sandbox-exec) - native, low overhead, no dependencies
2. **Linux**: Docker or Podman - containerization with namespace isolation
3. **Windows**: Docker - via WSL2 or Docker Desktop
4. **Fallback**: No sandboxing with warning

## Platform Implementations

### macOS: Seatbelt (sandbox-exec)

macOS includes a powerful sandboxing mechanism called Seatbelt, accessible via the `sandbox-exec` command. This is the same technology that powers App Sandbox for Mac App Store applications.

#### How It Works

Commands are wrapped with `sandbox-exec`:

```bash
sandbox-exec -D TARGET_DIR=/path/to/project \
             -D TMP_DIR=/tmp \
             -D HOME_DIR=/Users/user \
             -f /path/to/profile.sb \
             /bin/bash -c "user command here"
```

The `-D` flags pass parameters to the profile, and `-f` specifies the Seatbelt profile file.

#### Profile Language (SBPL)

Seatbelt profiles use the Sandbox Profile Language, a Scheme-based DSL:

```scheme
(version 1)

;; Default deny - explicit allowlist approach
(deny default)

;; Allow reading files from anywhere
(allow file-read*)

;; Allow process execution (children inherit policy)
(allow process-exec)
(allow process-fork)

;; Allow signals to self
(allow signal (target self))

;; Allow writes only to specific parameterized paths
(allow file-write*
    (subpath (param "TARGET_DIR"))
    (subpath (param "TMP_DIR"))
    (literal "/dev/stdout")
    (literal "/dev/stderr")
    (literal "/dev/null"))

;; Terminal/PTY access for interactive commands
(allow file-ioctl (regex #"^/dev/tty.*"))

;; Optional: allow network access
(allow network-outbound)
```

#### Profile Variants

We provide two built-in profiles:

| Profile       | Default Action | File Writes        | Network      |
| ------------- | -------------- | ------------------ | ------------ |
| `restrictive` | Deny           | Project + tmp only | Configurable |
| `permissive`  | Allow          | Project + tmp only | Configurable |

The **restrictive** profile denies by default and explicitly allows required operations. The **permissive** profile allows by default but denies writes outside allowed paths. The restrictive profile is more secure but may break some tools that need unexpected system access.

#### Considerations

- **Deprecated but functional**: Apple deprecated `sandbox-exec` but still uses it internally for system services. The underlying functionality is unlikely to disappear.
- **Undocumented language**: SBPL is not officially documented. Profile development requires experimentation or referencing existing profiles (e.g., `/System/Library/Sandbox/Profiles/`, Chromium's sandbox).
- **Child inheritance**: Spawned processes inherit the sandbox policy, which is essential for shell pipelines.

#### Reference: gemini-cli Seatbelt Implementation

Google's gemini-cli uses Seatbelt with 6 static profile variants:

- `permissive-open` / `restrictive-open` - allow network
- `permissive-closed` / `restrictive-closed` - deny network
- `permissive-proxied` / `restrictive-proxied` - network via proxy

Key patterns from gemini-cli (`packages/cli/src/utils/sandbox.ts`):

- Profiles are bundled as static files
- Parameters passed via `-D` flags for paths
- Support for up to 5 additional "include directories" for multi-root workspaces
- Optional proxy support for network filtering

### Linux/Windows: Docker/Podman Containers

For non-macOS platforms, we use container technology which provides robust namespace-based isolation.

#### How It Works

Commands are executed inside ephemeral containers:

```bash
docker run --rm -i --init \
    --workdir /workspace \
    --volume /host/project:/workspace \
    --volume /tmp:/tmp \
    --network none \
    ubuntu:22.04 \
    bash -c "user command here"
```

#### Key Container Options

| Option           | Purpose                                |
| ---------------- | -------------------------------------- |
| `--rm`           | Auto-remove container on exit          |
| `-i`             | Keep stdin open for input              |
| `-t`             | Allocate TTY (if available)            |
| `--init`         | Use tini for proper signal handling    |
| `--workdir`      | Set working directory inside container |
| `--volume`       | Mount host directories                 |
| `--network none` | Disable network access                 |
| `--user`         | Run as specific UID:GID                |

#### Volume Mounting Strategy

```text
Host                          Container
────────────────────────────────────────────
/path/to/project      →      /path/to/project (rw)
/tmp                  →      /tmp (rw)
~/.config/dive        →      ~/.config/dive (rw)
~/.gitconfig          →      ~/.gitconfig (ro)
```

The project directory is mounted read-write at the same path to preserve path consistency. Additional mounts can be configured for tool-specific needs.

#### UID/GID Mapping

A critical consideration on Linux is file ownership. Files created in the container will be owned by the container's user, which can cause permission issues on the host.

Solution (from gemini-cli):

1. Start container as root
2. Create a user inside the container with the host user's UID/GID
3. Drop privileges to that user before running commands

```bash
docker run --user root ... image bash -c "
    groupadd -f -g 1000 dive &&
    useradd -o -u 1000 -g 1000 -d /home/dive -s /bin/bash dive 2>/dev/null || true &&
    su -p dive -c 'actual command here'
"
```

#### Windows Path Translation

Windows paths must be converted for Docker:

```text
C:\Users\name\project  →  /c/Users/name/project
```

#### Docker vs Podman

| Feature       | Docker          | Podman                |
| ------------- | --------------- | --------------------- |
| Daemon        | Required        | Daemonless            |
| Rootless      | Requires config | Default               |
| Compatibility | Standard        | Docker-compatible CLI |
| Availability  | Wider           | Growing               |

We prefer Podman when available on Linux (rootless by default), falling back to Docker.

#### Reference: gemini-cli Container Implementation

gemini-cli's container sandboxing (`packages/cli/src/utils/sandbox.ts`) includes:

- Support for both Docker and Podman
- Custom sandbox images with pre-installed tools
- UID/GID mapping for Linux file permissions
- Environment variable passthrough for API keys, config
- Port exposure for debugging and dev servers
- Optional proxy container on isolated network for network filtering
- TTY detection and passthrough
- Volume mounts for project, temp, settings, and credentials

#### Reference: sandbox-runtime Advanced Features

Anthropic's sandbox-runtime project demonstrates more advanced sandboxing:

**Linux (Bubblewrap + seccomp)**:

- Uses `bwrap` for namespace isolation instead of full containers
- Adds seccomp filters to block specific syscalls (e.g., Unix socket creation)
- Dynamic profile generation based on configuration
- Proxy-based network filtering with socat bridges

**macOS (Dynamic Seatbelt)**:

- Generates Seatbelt profiles dynamically at runtime
- Glob-to-regex conversion for flexible path patterns
- Real-time violation monitoring via sandbox logs
- Automatic detection of "dangerous files" (.env, credentials, etc.)

These advanced features could be considered for future iterations but add significant complexity.

## Configuration

### SandboxConfig

```go
type SandboxConfig struct {
    // Enabled turns sandboxing on/off
    Enabled bool

    // WorkDir is the project directory (mounted read-write)
    WorkDir string

    // AllowNetwork permits outbound network access
    AllowNetwork bool

    // AllowedWritePaths are additional writable paths outside WorkDir
    AllowedWritePaths []string

    // Environment variables to pass through to sandboxed process
    Environment map[string]string

    // Docker-specific options
    Docker DockerConfig

    // Seatbelt-specific options
    Seatbelt SeatbeltConfig
}

type DockerConfig struct {
    // Image is the container image (default: "ubuntu:22.04")
    Image string

    // Command is "docker" or "podman" (auto-detected if empty)
    Command string

    // AdditionalMounts are extra volume mounts ("host:container:opts")
    AdditionalMounts []string

    // Ports to expose on the host
    Ports []string
}

type SeatbeltConfig struct {
    // Profile is "restrictive" or "permissive" (default: "restrictive")
    Profile string

    // CustomProfilePath overrides built-in profiles
    CustomProfilePath string
}
```

### YAML Configuration

```yaml
# In dive.yaml or agent config
sandbox:
  enabled: true
  allow_network: false
  allowed_write_paths:
    - ~/.cache/go-build
    - ~/.npm

  docker:
    image: "node:20-slim"
    ports:
      - "3000"
      - "9229" # Node debugger

  seatbelt:
    profile: restrictive
```

### Programmatic Configuration

```go
import "github.com/deepnoodle-ai/dive/experimental/sandbox"

cfg := &sandbox.Config{
    Enabled:      true,
    WorkDir:      "/path/to/project",
    AllowNetwork: false,
    AllowedWritePaths: []string{
        filepath.Join(os.Getenv("HOME"), ".cache"),
    },
    Docker: sandbox.DockerConfig{
        Image: "golang:1.22",
    },
}

bashTool := toolkit.NewBashTool(toolkit.BashToolOptions{
    SandboxConfig: cfg,
})
```

## Implementation

### Package Structure

```text
experimental/sandbox/
├── sandbox.go        # Manager, Backend interface, Config types
├── seatbelt.go       # macOS Seatbelt backend
├── docker.go         # Docker/Podman backend
├── proxy/            # Network proxy for domain filtering
├── profiles/
│   ├── restrictive.sb
│   └── permissive.sb
└── sandbox_test.go
```

### Backend Interface

```go
// Backend represents a sandboxing implementation
type Backend interface {
    // Name returns the backend identifier
    Name() string

    // Available checks if this backend can be used
    Available() bool

    // WrapCommand wraps a command for sandboxed execution
    WrapCommand(ctx context.Context, cmd *exec.Cmd, cfg *Config) (*exec.Cmd, error)
}
```

### Manager

```go
type Manager struct {
    backends []Backend
    config   *Config
}

func NewManager(cfg *Config) *Manager {
    return &Manager{
        backends: []Backend{
            &SeatbeltBackend{},
            NewDockerBackend(),
        },
        config: cfg,
    }
}

func (m *Manager) SelectBackend() Backend {
    for _, b := range m.backends {
        if b.Available() {
            return b
        }
    }
    return nil
}

func (m *Manager) Wrap(ctx context.Context, cmd *exec.Cmd) (*exec.Cmd, error) {
    if !m.config.Enabled {
        return cmd, nil
    }

    backend := m.SelectBackend()
    if backend == nil {
        // Log warning: no sandbox available
        return cmd, nil
    }

    return backend.WrapCommand(ctx, cmd, m.config)
}
```

### Integration with BashSession

The `BashSession` in `toolkit/bash.go` will be modified to optionally use sandboxing:

```go
type BashSessionOptions struct {
    WorkingDirectory string
    MaxOutputLength  int
    PathValidator    *PathValidator
    SandboxManager   *sandbox.Manager  // NEW
}

func (s *BashSession) start() error {
    s.mu.Lock()
    defer s.mu.Unlock()

    shell := "/bin/bash"
    if runtime.GOOS == "windows" {
        shell = "cmd"
    }

    s.cmd = exec.Command(shell)
    if s.workingDir != "" {
        s.cmd.Dir = s.workingDir
    }

    // Apply sandboxing if configured
    if s.sandboxManager != nil {
        wrapped, err := s.sandboxManager.Wrap(context.Background(), s.cmd)
        if err != nil {
            return fmt.Errorf("sandbox wrap failed: %w", err)
        }
        s.cmd = wrapped
    }

    // ... rest of start() unchanged
}
```

### Seatbelt Backend Implementation

```go
//go:embed profiles/restrictive.sb
var restrictiveProfile string

//go:embed profiles/permissive.sb
var permissiveProfile string

type SeatbeltBackend struct{}

func (s *SeatbeltBackend) Name() string { return "seatbelt" }

func (s *SeatbeltBackend) Available() bool {
    if runtime.GOOS != "darwin" {
        return false
    }
    _, err := exec.LookPath("sandbox-exec")
    return err == nil
}

func (s *SeatbeltBackend) WrapCommand(ctx context.Context, cmd *exec.Cmd, cfg *Config) (*exec.Cmd, error) {
    // Select and write profile
    profile := restrictiveProfile
    if cfg.Seatbelt.Profile == "permissive" {
        profile = permissiveProfile
    }
    if cfg.Seatbelt.CustomProfilePath != "" {
        data, err := os.ReadFile(cfg.Seatbelt.CustomProfilePath)
        if err != nil {
            return nil, fmt.Errorf("read custom profile: %w", err)
        }
        profile = string(data)
    }

    // Modify profile for network setting
    if !cfg.AllowNetwork {
        profile = strings.Replace(profile,
            "(allow network-outbound)",
            ";; (allow network-outbound) - disabled", 1)
    }

    // Write profile to temp file
    profilePath := filepath.Join(os.TempDir(),
        fmt.Sprintf("dive-sandbox-%d.sb", os.Getpid()))
    if err := os.WriteFile(profilePath, []byte(profile), 0644); err != nil {
        return nil, err
    }

    // Build sandbox-exec arguments
    args := []string{
        "-D", fmt.Sprintf("TARGET_DIR=%s", cfg.WorkDir),
        "-D", fmt.Sprintf("TMP_DIR=%s", os.TempDir()),
        "-D", fmt.Sprintf("HOME_DIR=%s", os.Getenv("HOME")),
        "-D", fmt.Sprintf("CACHE_DIR=%s", cacheDir()),
    }

    // Add allowed write paths
    for i, p := range cfg.AllowedWritePaths {
        if i >= 5 { break } // Limit to 5 additional paths
        args = append(args, "-D", fmt.Sprintf("ALLOWED_DIR_%d=%s", i, p))
    }
    // Pad remaining slots with /dev/null
    for i := len(cfg.AllowedWritePaths); i < 5; i++ {
        args = append(args, "-D", fmt.Sprintf("ALLOWED_DIR_%d=/dev/null", i))
    }

    args = append(args, "-f", profilePath)
    args = append(args, cmd.Path)
    args = append(args, cmd.Args[1:]...)

    wrapped := exec.CommandContext(ctx, "sandbox-exec", args...)
    wrapped.Dir = cmd.Dir
    wrapped.Env = cmd.Env
    wrapped.Stdin = cmd.Stdin
    wrapped.Stdout = cmd.Stdout
    wrapped.Stderr = cmd.Stderr

    return wrapped, nil
}
```

### Docker Backend Implementation

```go
type DockerBackend struct {
    command string
}

func NewDockerBackend() *DockerBackend {
    // Prefer podman on Linux
    if _, err := exec.LookPath("podman"); err == nil {
        return &DockerBackend{command: "podman"}
    }
    return &DockerBackend{command: "docker"}
}

func (d *DockerBackend) Name() string { return d.command }

func (d *DockerBackend) Available() bool {
    if _, err := exec.LookPath(d.command); err != nil {
        return false
    }
    // Verify daemon is running
    cmd := exec.Command(d.command, "info")
    cmd.Stdout = io.Discard
    cmd.Stderr = io.Discard
    return cmd.Run() == nil
}

func (d *DockerBackend) WrapCommand(ctx context.Context, cmd *exec.Cmd, cfg *Config) (*exec.Cmd, error) {
    image := cfg.Docker.Image
    if image == "" {
        image = "ubuntu:22.04"
    }

    args := []string{
        "run", "--rm", "-i", "--init",
        "--workdir", getContainerPath(cfg.WorkDir),
    }

    // TTY if available
    if isTerminal(os.Stdin) {
        args = append(args, "-t")
    }

    // Mount project directory
    args = append(args, "--volume",
        fmt.Sprintf("%s:%s", cfg.WorkDir, getContainerPath(cfg.WorkDir)))

    // Mount temp directory
    args = append(args, "--volume",
        fmt.Sprintf("%s:%s", os.TempDir(), getContainerPath(os.TempDir())))

    // Mount additional paths
    for _, p := range cfg.AllowedWritePaths {
        args = append(args, "--volume",
            fmt.Sprintf("%s:%s", p, getContainerPath(p)))
    }
    for _, m := range cfg.Docker.AdditionalMounts {
        args = append(args, "--volume", m)
    }

    // Network isolation
    if !cfg.AllowNetwork {
        args = append(args, "--network", "none")
    }

    // Port exposure
    for _, port := range cfg.Docker.Ports {
        args = append(args, "--publish", fmt.Sprintf("%s:%s", port, port))
    }

    // Environment variables
    for k, v := range cfg.Environment {
        args = append(args, "--env", fmt.Sprintf("%s=%s", k, v))
    }

    // UID/GID handling on Linux
    if runtime.GOOS == "linux" {
        args = d.addLinuxUserMapping(args, cmd, cfg, image)
    } else {
        args = append(args, image)
        args = append(args, cmd.Path)
        args = append(args, cmd.Args[1:]...)
    }

    wrapped := exec.CommandContext(ctx, d.command, args...)
    wrapped.Dir = cmd.Dir
    wrapped.Env = cmd.Env
    wrapped.Stdin = cmd.Stdin
    wrapped.Stdout = cmd.Stdout
    wrapped.Stderr = cmd.Stderr

    return wrapped, nil
}

func (d *DockerBackend) addLinuxUserMapping(args []string, cmd *exec.Cmd, cfg *Config, image string) []string {
    u, err := user.Current()
    if err != nil {
        args = append(args, image)
        args = append(args, cmd.Path)
        args = append(args, cmd.Args[1:]...)
        return args
    }

    args = append(args, "--user", "root")

    innerCmd := shellQuote(append([]string{cmd.Path}, cmd.Args[1:]...))
    shellCmd := fmt.Sprintf(
        "groupadd -f -g %s dive 2>/dev/null; "+
        "useradd -o -u %s -g %s -d /home/dive -s /bin/bash dive 2>/dev/null || true; "+
        "su -p dive -c %s",
        u.Gid, u.Uid, u.Gid, innerCmd,
    )

    args = append(args, image, "bash", "-c", shellCmd)
    return args
}

func getContainerPath(hostPath string) string {
    if runtime.GOOS != "windows" {
        return hostPath
    }
    // Convert C:\foo\bar to /c/foo/bar
    path := strings.ReplaceAll(hostPath, "\\", "/")
    if len(path) >= 2 && path[1] == ':' {
        return "/" + strings.ToLower(string(path[0])) + path[2:]
    }
    return path
}
```

## Security Considerations

### What Sandboxing Prevents

1. **Filesystem damage** - Cannot write outside project/temp directories
2. **Data exfiltration** - Network disabled by default
3. **System modification** - Cannot modify system files
4. **Privilege escalation** - Runs as unprivileged user in container

### What Sandboxing Does NOT Prevent

1. **Reading sensitive files** - Files readable by the user are still readable
2. **Resource exhaustion** - CPU/memory limits not enforced (could be added)
3. **Side-channel attacks** - Timing attacks, cache attacks, etc.
4. **Sandbox escapes** - Kernel vulnerabilities could allow escape

### Defense in Depth

Sandboxing is one layer of defense. It should be combined with:

1. **Permission system** - Pre-approve/deny tool calls
2. **Input validation** - Sanitize command inputs
3. **Output filtering** - Detect sensitive data in output
4. **Audit logging** - Track all tool executions
5. **Rate limiting** - Prevent runaway commands

### Image Security (Docker)

For Docker-based sandboxing:

1. Use minimal base images (alpine, distroless)
2. Keep images updated for security patches
3. Consider read-only root filesystem (`--read-only`)
4. Add resource limits (`--memory`, `--cpus`)

## Testing

### Unit Tests

```go
func TestSeatbeltBackend_Available(t *testing.T) {
    backend := &SeatbeltBackend{}
    if runtime.GOOS == "darwin" {
        require.True(t, backend.Available())
    } else {
        require.False(t, backend.Available())
    }
}

func TestDockerBackend_WrapCommand(t *testing.T) {
    backend := NewDockerBackend()
    if !backend.Available() {
        t.Skip("Docker not available")
    }

    cfg := &Config{
        Enabled:      true,
        WorkDir:      t.TempDir(),
        AllowNetwork: false,
    }

    cmd := exec.Command("echo", "hello")
    wrapped, err := backend.WrapCommand(context.Background(), cmd, cfg)
    require.NoError(t, err)
    require.Equal(t, backend.command, filepath.Base(wrapped.Path))
}
```

### Integration Tests

```go
func TestSandbox_FilesystemIsolation(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    mgr := sandbox.NewManager(&sandbox.Config{
        Enabled:      true,
        WorkDir:      t.TempDir(),
        AllowNetwork: false,
    })

    // Should succeed: write to project dir
    cmd := exec.Command("bash", "-c", "touch allowed.txt")
    cmd.Dir = mgr.Config().WorkDir
    wrapped, _ := mgr.Wrap(context.Background(), cmd)
    require.NoError(t, wrapped.Run())

    // Should fail: write outside project dir
    cmd = exec.Command("bash", "-c", "touch /tmp/outside-project/bad.txt")
    wrapped, _ = mgr.Wrap(context.Background(), cmd)
    err := wrapped.Run()
    require.Error(t, err)
}
```

## Future Enhancements

### Short Term

1. Resource limits (memory, CPU, disk I/O)
2. Read-only filesystem mode
3. Custom Docker images per project
4. Violation logging and reporting

### Medium Term

1. Network proxy for domain filtering
2. Bubblewrap backend for Linux (lighter than Docker)
3. Windows native sandboxing (AppContainers)
4. Seccomp filters for syscall blocking

### Long Term

1. Real-time violation monitoring
2. Automatic "dangerous file" detection
3. Sandbox profiles learned from tool behavior
4. Integration with cloud sandboxing services

## References

### External Projects

- **gemini-cli** (Google) - Reference implementation for Seatbelt and Docker sandboxing
  - Source: `packages/cli/src/utils/sandbox.ts`
  - Seatbelt profiles: `packages/cli/src/utils/sandbox-macos-*.sb`

- **sandbox-runtime** (Anthropic) - Advanced sandboxing with Bubblewrap and seccomp
  - Source: `src/sandbox/`
  - Linux: Bubblewrap + seccomp filters
  - macOS: Dynamic Seatbelt profile generation

### Apple Documentation

- [App Sandbox Design Guide](https://developer.apple.com/library/archive/documentation/Security/Conceptual/AppSandboxDesignGuide/)
- Seatbelt profiles: `/System/Library/Sandbox/Profiles/`

### Container Security

- [Docker Security Best Practices](https://docs.docker.com/engine/security/)
- [Podman Rootless Containers](https://github.com/containers/podman/blob/main/rootless.md)

### Linux Sandboxing

- [Bubblewrap](https://github.com/containers/bubblewrap)
- [seccomp BPF](https://www.kernel.org/doc/html/latest/userspace-api/seccomp_filter.html)
