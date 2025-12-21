package toolkit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathValidator provides workspace-based path validation for tools.
// By default, read operations are allowed within the workspace directory.
// Write operations and access outside the workspace require explicit approval
// at the agent level.
type PathValidator struct {
	// WorkspaceDir is the base directory for workspace operations.
	// Defaults to current working directory if empty.
	WorkspaceDir string
}

// NewPathValidator creates a PathValidator with the given workspace directory.
// If workspaceDir is empty, it defaults to the current working directory.
func NewPathValidator(workspaceDir string) (*PathValidator, error) {
	if workspaceDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		workspaceDir = cwd
	}

	// Resolve to absolute path
	absWorkspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve workspace path: %w", err)
	}

	// Resolve symlinks for the workspace directory
	realWorkspace, err := filepath.EvalSymlinks(absWorkspace)
	if err != nil {
		// If workspace doesn't exist yet, use the absolute path
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to resolve workspace symlinks: %w", err)
		}
		realWorkspace = absWorkspace
	}

	return &PathValidator{WorkspaceDir: realWorkspace}, nil
}

// ResolvePath resolves a path to its absolute, symlink-resolved form.
// Returns the resolved path and any error encountered.
func (v *PathValidator) ResolvePath(path string) (string, error) {
	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Resolve symlinks to prevent traversal attacks
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If file doesn't exist, resolve parent directory symlinks recursively
		if os.IsNotExist(err) {
			return v.resolveNonExistentPath(absPath)
		}
		return "", fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	return realPath, nil
}

// resolveNonExistentPath resolves symlinks in a path where the final component
// doesn't exist yet (for new file creation)
func (v *PathValidator) resolveNonExistentPath(absPath string) (string, error) {
	// Walk up the directory tree until we find an existing directory
	dir := absPath
	var parts []string

	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}

		parts = append([]string{filepath.Base(dir)}, parts...)
		dir = parent

		// Check if this directory exists
		if _, err := os.Stat(dir); err == nil {
			// Found an existing directory, resolve its symlinks
			realDir, err := filepath.EvalSymlinks(dir)
			if err != nil {
				return "", fmt.Errorf("failed to resolve symlinks: %w", err)
			}
			// Rejoin with the remaining path parts
			return filepath.Join(append([]string{realDir}, parts...)...), nil
		}
	}

	// Nothing exists, return the absolute path as-is
	return absPath, nil
}

// IsInWorkspace checks if the given path is within the workspace directory.
// It resolves symlinks before checking to prevent symlink-based attacks.
func (v *PathValidator) IsInWorkspace(path string) (bool, error) {
	realPath, err := v.ResolvePath(path)
	if err != nil {
		return false, err
	}

	// Check if the resolved path is within the workspace
	rel, err := filepath.Rel(v.WorkspaceDir, realPath)
	if err != nil {
		return false, nil // Different drives on Windows, etc.
	}

	// Path is outside workspace if relative path starts with ".."
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return false, nil
	}

	// Also check for absolute paths that don't share the workspace prefix
	if filepath.IsAbs(rel) {
		return false, nil
	}

	return true, nil
}

// ValidateRead checks if reading from the given path is allowed.
// By default, reads within the workspace are allowed.
// Returns nil if allowed, or an error describing why access is denied.
func (v *PathValidator) ValidateRead(path string) error {
	inWorkspace, err := v.IsInWorkspace(path)
	if err != nil {
		return fmt.Errorf("failed to validate path: %w", err)
	}

	if !inWorkspace {
		return &PathAccessError{
			Path:      path,
			Operation: "read",
			Reason:    "path is outside workspace",
			Workspace: v.WorkspaceDir,
		}
	}

	return nil
}

// ValidateWrite checks if writing to the given path is allowed.
// By default, writes within the workspace are allowed.
// Returns nil if allowed, or an error describing why access is denied.
func (v *PathValidator) ValidateWrite(path string) error {
	inWorkspace, err := v.IsInWorkspace(path)
	if err != nil {
		return fmt.Errorf("failed to validate path: %w", err)
	}

	if !inWorkspace {
		return &PathAccessError{
			Path:      path,
			Operation: "write",
			Reason:    "path is outside workspace",
			Workspace: v.WorkspaceDir,
		}
	}

	return nil
}

// PathAccessError is returned when a path access is denied.
type PathAccessError struct {
	Path      string
	Operation string
	Reason    string
	Workspace string
}

func (e *PathAccessError) Error() string {
	return fmt.Sprintf("access denied: cannot %s %q - %s (workspace: %s)",
		e.Operation, e.Path, e.Reason, e.Workspace)
}

// IsPathAccessError returns true if the error is a PathAccessError.
func IsPathAccessError(err error) bool {
	_, ok := err.(*PathAccessError)
	return ok
}
