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
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(workDir); err == nil {
			workDir = resolved
		}
	} else {
		workDir = cfg.WorkDir // Fallback
	}
	tmpDir := os.TempDir()
	if resolved, err := filepath.EvalSymlinks(tmpDir); err == nil {
		tmpDir = resolved
	}

	var allowedPaths []string
	for _, p := range cfg.AllowedWritePaths {
		absP, err := filepath.Abs(p)
		if err == nil {
			if resolved, err := filepath.EvalSymlinks(absP); err == nil {
				absP = resolved
			}
			allowedPaths = append(allowedPaths, absP)
		} else {
			allowedPaths = append(allowedPaths, p)
		}
	}

	data := seatbeltTemplateData{
		WorkDir:           workDir,
		TmpDir:            tmpDir,
		AllowedWritePaths: allowedPaths,
		AllowNetwork:      cfg.AllowNetwork,
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
	wrapped.Env = cmd.Env
	wrapped.Stdin = cmd.Stdin
	wrapped.Stdout = cmd.Stdout
	wrapped.Stderr = cmd.Stderr

	return wrapped, cleanup, nil
}
