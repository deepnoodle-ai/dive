package sandbox

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/template"
)

//go:embed profiles/restrictive.sb.tmpl
var restrictiveProfileTmpl string

//go:embed profiles/permissive.sb.tmpl
var permissiveProfileTmpl string

type SeatbeltBackend struct{}

func (s *SeatbeltBackend) Name() string { return "seatbelt" }

func (s *SeatbeltBackend) Available() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	_, err := exec.LookPath("sandbox-exec")
	return err == nil
}

type seatbeltTemplateData struct {
	WorkDir           string
	TmpDir            string
	AllowedWritePaths []string
	AllowNetwork      bool
}

func (s *SeatbeltBackend) WrapCommand(ctx context.Context, cmd *exec.Cmd, cfg *Config) (*exec.Cmd, func(), error) {
	if err := validateNetworkConfig(cfg); err != nil {
		return nil, nil, err
	}

	// Select template
	tmplStr := restrictiveProfileTmpl
	if cfg.Seatbelt.Profile == "permissive" {
		tmplStr = permissiveProfileTmpl
	}
	if cfg.Seatbelt.CustomProfilePath != "" {
		data, err := os.ReadFile(cfg.Seatbelt.CustomProfilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("read custom profile: %w", err)
		}
		tmplStr = string(data)
	}

	tmpl, err := template.New("profile").Parse(tmplStr)
	if err != nil {
		return nil, nil, fmt.Errorf("parse profile template: %w", err)
	}

	// Prepare data
	workDir, err := filepath.Abs(cfg.WorkDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve workDir %q: %w", cfg.WorkDir, err)
	}
	if resolved, err := filepath.EvalSymlinks(workDir); err == nil {
		workDir = resolved
	}

	tmpDir := os.TempDir()
	if resolved, err := filepath.EvalSymlinks(tmpDir); err == nil {
		tmpDir = resolved
	}

	// Also allow /tmp which is separate from os.TempDir() on macOS.
	// On macOS, /tmp is a symlink to /private/tmp, while os.TempDir()
	// returns a per-user directory like /var/folders/.../T/.
	slashTmp := "/tmp"
	if resolved, err := filepath.EvalSymlinks(slashTmp); err == nil {
		slashTmp = resolved
	}

	var allowedPaths []string
	for _, p := range cfg.AllowedWritePaths {
		absP, err := filepath.Abs(p)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to resolve allowed path %q: %w", p, err)
		}
		if resolved, err := filepath.EvalSymlinks(absP); err == nil {
			absP = resolved
		}
		allowedPaths = append(allowedPaths, absP)
	}

	// Escape/Quote paths for Seatbelt syntax to prevent injection.
	// We use simple double quotes and escape existing quotes/backslashes.
	quotePath := func(p string) string {
		return fmt.Sprintf("%q", p)
	}

	// Pre-quote paths in the data structure or use a func map.
	// Since we are using struct fields, let's update the template to expects raw strings
	// but we'll manually quote them here if we can, OR better: use the quotePath logic.
	// Actually, Go's text/template doesn't automatically escape for Scheme.
	// We'll update the data struct to hold Quoted paths or update the template.
	// Updating the template is cleaner if we pass a FuncMap, but for now let's just
	// quote them in the data struct (changing types or just storing quoted strings).
	// Let's create a new struct for the template.

	type templateData struct {
		WorkDir           string
		TmpDir            string
		AllowedWritePaths []string
		AllowNetwork      bool
	}

	data := templateData{
		WorkDir:      quotePath(workDir),
		TmpDir:       quotePath(tmpDir),
		AllowNetwork: cfg.AllowNetwork,
	}
	// Add /tmp if it's different from os.TempDir() (common on macOS)
	if slashTmp != tmpDir {
		data.AllowedWritePaths = append(data.AllowedWritePaths, quotePath(slashTmp))
	}
	for _, p := range allowedPaths {
		data.AllowedWritePaths = append(data.AllowedWritePaths, quotePath(p))
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, nil, fmt.Errorf("execute profile template: %w", err)
	}

	// Write profile to temp file
	f, err := os.CreateTemp("", "dive-sandbox-*.sb")
	if err != nil {
		return nil, nil, err
	}
	profilePath := f.Name()
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		os.Remove(profilePath)
		return nil, nil, err
	}
	f.Close()

	cleanup := func() {
		os.Remove(profilePath)
	}

	// Build sandbox-exec arguments
	args := []string{"-f", profilePath, cmd.Path}
	if len(cmd.Args) > 1 {
		args = append(args, cmd.Args[1:]...)
	}

	wrapped := exec.CommandContext(ctx, "sandbox-exec", args...)
	wrapped.Dir = cmd.Dir
	wrapped.Env = BuildCommandEnv(cmd.Env, cfg)
	wrapped.Stdin = cmd.Stdin
	wrapped.Stdout = cmd.Stdout
	wrapped.Stderr = cmd.Stderr

	return wrapped, cleanup, nil
}
