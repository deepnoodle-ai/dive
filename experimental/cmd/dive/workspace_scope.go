package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type workspaceBoundary struct {
	Workspace string
	GitRoot   string
	Relative  string
}

func resolveWorkspaceDir(configured, fallback string) (string, error) {
	if configured == "" {
		configured = fallback
	} else if !filepath.IsAbs(configured) {
		configured = filepath.Join(fallback, configured)
	}
	absolute, err := filepath.Abs(configured)
	if err != nil {
		return "", fmt.Errorf("resolve workspace %q: %w", configured, err)
	}
	return filepath.Clean(absolute), nil
}

func detectWorkspaceBoundary(workspaceDir string) (workspaceBoundary, bool) {
	workspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		return workspaceBoundary{}, false
	}
	workspace = canonicalPath(workspace)
	output, err := exec.Command("git", "-C", workspace, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return workspaceBoundary{}, false
	}
	root, err := filepath.Abs(strings.TrimSpace(string(output)))
	if err != nil {
		return workspaceBoundary{}, false
	}
	root = canonicalPath(root)
	if samePath(workspace, root) {
		return workspaceBoundary{}, false
	}
	relative, err := filepath.Rel(workspace, root)
	if err != nil {
		return workspaceBoundary{}, false
	}
	return workspaceBoundary{Workspace: filepath.Clean(workspace), GitRoot: filepath.Clean(root), Relative: relative}, true
}

func samePath(left, right string) bool {
	return canonicalPath(left) == canonicalPath(right)
}

func canonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

func workspaceScopeSummary(workspaceDir string) string {
	boundary, limited := detectWorkspaceBoundary(workspaceDir)
	if !limited {
		return ""
	}
	return fmt.Sprintf("directory only · Git root: %s", boundary.Relative)
}
