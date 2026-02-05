package toolkit

import (
	"os"
	"path/filepath"
	"strings"
)

// FileSystem abstracts file operations to enable testing with mock implementations.
//
// This interface is used by tools like [TextEditorTool] to perform file
// operations. In production, use [RealFileSystem]. For testing, implement
// a mock that returns controlled responses.
type FileSystem interface {
	// ReadFile returns the contents of the file as a string.
	ReadFile(path string) (string, error)

	// WriteFile writes content to the file, creating parent directories as needed.
	WriteFile(path string, content string) error

	// FileExists returns true if a file or directory exists at the path.
	FileExists(path string) bool

	// IsDir returns true if the path is a directory.
	IsDir(path string) bool

	// IsAbs returns true if the path is absolute.
	IsAbs(path string) bool

	// ListDir returns a string listing of directory contents up to 2 levels deep.
	ListDir(path string) (string, error)

	// MkdirAll creates the directory and any necessary parents.
	MkdirAll(path string, perm os.FileMode) error

	// FileSize returns the size of the file in bytes.
	FileSize(path string) (int64, error)
}

// RealFileSystem implements [FileSystem] using actual file system operations.
// This is the production implementation used by default.
type RealFileSystem struct{}

// ReadFile reads the entire file and returns its contents as a string.
func (fs *RealFileSystem) ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFile writes content to the file, creating parent directories as needed.
// Files are created with 0644 permissions; directories with 0755.
func (fs *RealFileSystem) WriteFile(path string, content string) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// FileExists returns true if a file or directory exists at the path.
func (fs *RealFileSystem) FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// IsDir returns true if the path exists and is a directory.
func (fs *RealFileSystem) IsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// IsAbs returns true if the path is absolute.
func (fs *RealFileSystem) IsAbs(path string) bool {
	return filepath.IsAbs(path)
}

// ListDir returns a newline-separated list of paths in the directory,
// walking up to 2 levels deep and excluding hidden files (starting with ".").
func (fs *RealFileSystem) ListDir(path string) (string, error) {
	var result strings.Builder

	// Walk directory up to 2 levels deep, excluding hidden files
	err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip entries we can't access
		}

		// Get relative path for depth calculation
		relPath, err := filepath.Rel(path, p)
		if err != nil {
			return nil
		}

		// Skip hidden files and directories (starting with .)
		name := d.Name()
		if len(name) > 0 && name[0] == '.' && relPath != "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Calculate depth (count path separators)
		depth := 0
		if relPath != "." {
			depth = strings.Count(relPath, string(filepath.Separator)) + 1
		}

		// Skip if deeper than 2 levels
		if depth > 2 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		result.WriteString(p)
		result.WriteString("\n")
		return nil
	})
	if err != nil {
		return "", err
	}
	return result.String(), nil
}

// MkdirAll creates the directory and all parent directories with the given permissions.
func (fs *RealFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// FileSize returns the size of the file in bytes.
func (fs *RealFileSystem) FileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
