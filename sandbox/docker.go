package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
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
	image := cfg.Docker.Image
	if image == "" {
		image = "ubuntu:22.04"
	}

	args := []string{
		"run", "--rm", "-i", "--init",
		"--read-only", // Security: Read-Only Root Filesystem
		"--workdir", getContainerPath(cfg.WorkDir),
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

	// Mount Cloud Credentials
	if cfg.MountCloudCredentials {
		home, err := os.UserHomeDir()
		if err == nil {
			// gcloud
			gcloudPath := filepath.Join(home, ".config", "gcloud")
			if _, err := os.Stat(gcloudPath); err == nil {
				args = append(args, "--volume", fmt.Sprintf("%s:%s:ro", gcloudPath, getContainerPath(gcloudPath)))
			}
			// aws
			awsDir := filepath.Join(home, ".aws")
			if _, err := os.Stat(awsDir); err == nil {
				args = append(args, "--volume", fmt.Sprintf("%s:%s:ro", awsDir, getContainerPath(awsDir)))
			}
		}
		// GOOGLE_APPLICATION_CREDENTIALS
		if gac := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); gac != "" {
			// Ensure we mount the file, even if it's outside standard paths
			args = append(args, "--volume", fmt.Sprintf("%s:%s:ro", gac, getContainerPath(gac)))
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
	for k, v := range cfg.Environment {
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
				exec.Command(d.command, "rm", "-f", strings.TrimSpace(string(cid))).Run()
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

	wrapped := exec.CommandContext(ctx, d.command, args...)
	wrapped.Dir = cmd.Dir
	wrapped.Env = cmd.Env
	wrapped.Stdin = cmd.Stdin
	wrapped.Stdout = cmd.Stdout
	wrapped.Stderr = cmd.Stderr

	return wrapped, cleanup, nil
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

func shellQuote(args []string) string {
	var quoted []string
	for _, s := range args {
		quoted = append(quoted, fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", "'\\''")))
	}
	return strings.Join(quoted, " ")
}
