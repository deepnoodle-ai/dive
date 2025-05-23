package toolkit

import (
	"os"
	"os/exec"
	"path/filepath"
)

// FileSystem interface for abstracting file operations (enables testing)
type FileSystem interface {
	ReadFile(path string) (string, error)
	WriteFile(path string, content string) error
	FileExists(path string) bool
	IsDir(path string) bool
	IsAbs(path string) bool
	ListDir(path string) (string, error)
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
	cmd := exec.Command("find", path, "-maxdepth", "2", "-not", "-path", "*/.*")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
