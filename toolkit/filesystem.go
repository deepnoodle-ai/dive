package toolkit

import (
	"os"
	"path/filepath"
	"strings"
)

// FileSystem interface for abstracting file operations (enables testing)
type FileSystem interface {
	ReadFile(path string) (string, error)
	WriteFile(path string, content string) error
	FileExists(path string) bool
	IsDir(path string) bool
	IsAbs(path string) bool
	ListDir(path string) (string, error)
	MkdirAll(path string, perm os.FileMode) error
	FileSize(path string) (int64, error)
}

// RealFileSystem implements FileSystem using actual file operations
type RealFileSystem struct{}

func (fs *RealFileSystem) ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (fs *RealFileSystem) WriteFile(path string, content string) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func (fs *RealFileSystem) FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func (fs *RealFileSystem) IsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func (fs *RealFileSystem) IsAbs(path string) bool {
	return filepath.IsAbs(path)
}

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

func (fs *RealFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (fs *RealFileSystem) FileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
