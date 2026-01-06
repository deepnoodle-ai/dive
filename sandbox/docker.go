package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

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

func (d *DockerBackend) WrapCommand(ctx context.Context, cmd *exec.Cmd, cfg *Config) (*exec.Cmd, func(), error) {
	if err := validateNetworkConfig(cfg); err != nil {
		return nil, nil, err
	}

	// Respect configured command if set
	dockerCmd := d.command
	if cfg.Docker.Command != "" {
		dockerCmd = cfg.Docker.Command
	}

	image := cfg.Docker.Image
	if image == "" {
		image = "ubuntu:22.04"
	}

	// Determine container workdir
	// If cmd.Dir is set, use it (relative to host WorkDir -> container path)
	// Otherwise default to project root
	containerWorkDir := getContainerPath(cfg.WorkDir)
	if cmd.Dir != "" {
		// Just map the requested dir directly to container path
		// (assuming it's within the mounted volumes)
		containerWorkDir = getContainerPath(cmd.Dir)
	}

	args := []string{
		"run", "--rm", "-i", "--init",
		"--read-only", // Security: Read-Only Root Filesystem
		"--workdir", containerWorkDir,
	}

	// Resource limits
	if cfg.Docker.Memory != "" {
		args = append(args, "--memory", cfg.Docker.Memory)
	}
	if cfg.Docker.CPUs != "" {
		args = append(args, "--cpus", cfg.Docker.CPUs)
	}
	if cfg.Docker.PidsLimit > 0 {
		args = append(args, "--pids-limit", strconv.Itoa(cfg.Docker.PidsLimit))
	}

	// Helper to safely append mounts
	addMount := func(hostPath, containerPath, opts string) error {
		// Validate host path (simple check to prevent injection via colon)
		if strings.Contains(hostPath, ":") && runtime.GOOS != "windows" { // Windows has C:\
			return fmt.Errorf("invalid host path (contains colon): %s", hostPath)
		}
		if err := validateUnixSocket(hostPath, cfg); err != nil {
			return err
		}
		mount := fmt.Sprintf("%s:%s", hostPath, containerPath)
		if opts != "" {
			mount += ":" + opts
		}
		args = append(args, "--volume", mount)
		return nil
	}

	// Mount project directory
	if err := addMount(cfg.WorkDir, getContainerPath(cfg.WorkDir), ""); err != nil {
		return nil, nil, err
	}

	// Mount temp directory
	if err := addMount(os.TempDir(), getContainerPath(os.TempDir()), ""); err != nil {
		return nil, nil, err
	}

	// Mount additional paths
	for _, p := range cfg.AllowedWritePaths {
		if err := addMount(p, getContainerPath(p), ""); err != nil {
			return nil, nil, err
		}
	}
	for _, m := range cfg.Docker.AdditionalMounts {
		if runtime.GOOS == "windows" {
			args = append(args, "--volume", m)
			continue
		}
		host, container, opts, err := parseMount(m)
		if err != nil {
			return nil, nil, err
		}
		if err := addMount(host, container, opts); err != nil {
			return nil, nil, err
		}
	}

	// Mount Cloud Credentials
	if cfg.MountCloudCredentials {
		home, err := os.UserHomeDir()
		if err == nil {
			// gcloud
			gcloudPath := filepath.Join(home, ".config", "gcloud")
			if _, err := os.Stat(gcloudPath); err == nil {
				if err := addMount(gcloudPath, getContainerPath(gcloudPath), "ro"); err != nil {
					return nil, nil, err
				}
			}
			// aws
			awsDir := filepath.Join(home, ".aws")
			if _, err := os.Stat(awsDir); err == nil {
				if err := addMount(awsDir, getContainerPath(awsDir), "ro"); err != nil {
					return nil, nil, err
				}
			}
		}
		// GOOGLE_APPLICATION_CREDENTIALS
		if gac := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); gac != "" {
			if err := addMount(gac, getContainerPath(gac), "ro"); err != nil {
				return nil, nil, err
			}
			args = append(args, "--env", fmt.Sprintf("GOOGLE_APPLICATION_CREDENTIALS=%s", getContainerPath(gac)))
		}
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
	envs := buildProxyEnv(cfg)
	for k, v := range cfg.Environment {
		envs[k] = v
	}
	for k, v := range envs {
		args = append(args, "--env", fmt.Sprintf("%s=%s", k, v))
	}

	// Pass through standard envs
	for _, env := range []string{"TERM", "COLORTERM"} {
		if v := os.Getenv(env); v != "" {
			args = append(args, "--env", fmt.Sprintf("%s=%s", env, v))
		}
	}

	// Setup CID file for cleanup
	cidFile, err := os.CreateTemp("", "dive-docker-cid-")
	var cleanup func() = func() {}

	if err == nil {
		cidPath := cidFile.Name()
		cidFile.Close()
		os.Remove(cidPath) // Docker wants to create it

		// Insert --cidfile after "run"
		newArgs := make([]string, 0, len(args)+2)
		newArgs = append(newArgs, args[0])
		newArgs = append(newArgs, "--cidfile", cidPath)
		newArgs = append(newArgs, args[1:]...)
		args = newArgs

		cleanup = func() {
			cid, err := os.ReadFile(cidPath)
			if err == nil && len(cid) > 0 {
				// remove container
				exec.Command(dockerCmd, "rm", "-f", strings.TrimSpace(string(cid))).Run()
			}
			os.Remove(cidPath)
		}
	}

	// UID/GID handling on Linux
	if runtime.GOOS == "linux" && cfg.Docker.EnableUserMapping {
		args = d.addLinuxUserMapping(args, cmd, cfg, image)
	} else {
		args = append(args, image)
		args = append(args, cmd.Path)
		args = append(args, cmd.Args[1:]...)
	}

	wrapped := exec.CommandContext(ctx, dockerCmd, args...)
	wrapped.Dir = cmd.Dir
	wrapped.Env = cmd.Env
	wrapped.Stdin = cmd.Stdin
	wrapped.Stdout = cmd.Stdout
	wrapped.Stderr = cmd.Stderr

	return wrapped, cleanup, nil
}

func parseMount(mount string) (string, string, string, error) {
	if mount == "" {
		return "", "", "", fmt.Errorf("invalid mount: empty string")
	}
	parts := strings.Split(mount, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", "", fmt.Errorf("invalid mount format: %s", mount)
	}
	host := parts[0]
	container := parts[1]
	opts := ""
	if len(parts) == 3 {
		opts = parts[2]
	}
	if host == "" || container == "" {
		return "", "", "", fmt.Errorf("invalid mount format: %s", mount)
	}
	return host, container, opts, nil
}

func validateUnixSocket(hostPath string, cfg *Config) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(hostPath)
	if err != nil {
		return nil
	}
	if info.Mode()&os.ModeSocket == 0 {
		return nil
	}
	if len(cfg.AllowedUnixSockets) == 0 {
		return fmt.Errorf("unix socket mount not allowed: %s", hostPath)
	}
	for _, pattern := range cfg.AllowedUnixSockets {
		if matchPattern(pattern, hostPath) {
			return nil
		}
	}
	return fmt.Errorf("unix socket mount not allowed: %s", hostPath)
}

func matchPattern(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if strings.ContainsAny(pattern, "*?[]") {
		ok, err := path.Match(pattern, value)
		return err == nil && ok
	}
	return strings.HasPrefix(value, pattern)
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
	// Use /bin/sh for better compatibility (Alpine)
	// Try to create user/group but ignore failures if they exist
	shellCmd := fmt.Sprintf(
		"groupadd -f -g %s dive 2>/dev/null; "+
			"useradd -o -u %s -g %s -d /home/dive -s /bin/sh dive 2>/dev/null || true; "+
			"su -p dive -c %s",
		u.Gid, u.Uid, u.Gid, innerCmd,
	)

	// Wrap in sh
	args = append(args, image, "/bin/sh", "-c", shellCmd)
	return args
}

func getContainerPath(hostPath string) string {
	return convertPathForContainer(hostPath, runtime.GOOS)
}

// convertPathForContainer converts a host path to a container-compatible path.
// On Windows, converts paths like C:\foo\bar to /c/foo/bar for Docker/Podman.
// On other platforms, returns the path unchanged.
func convertPathForContainer(hostPath, goos string) string {
	if goos != "windows" {
		return hostPath
	}
	// Convert C:\foo\bar to /c/foo/bar
	path := strings.ReplaceAll(hostPath, "\\", "/")
	if len(path) >= 2 && path[1] == ':' {
		return "/" + strings.ToLower(string(path[0])) + path[2:]
	}
	return path
}

func shellQuote(args []string) string {
	var quoted []string
	for _, s := range args {
		quoted = append(quoted, fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "'\\''")))
	}
	return strings.Join(quoted, " ")
}
