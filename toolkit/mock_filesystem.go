package toolkit

import (
	"os"
	"strings"
)

// mockFileSystem implements FileSystem for testing
type mockFileSystem struct {
	files       map[string]string // path -> content
	directories map[string]bool   // path -> isDir
	writeError  error             // simulate write errors
	readError   error             // simulate read errors
}

func newMockFileSystem() *mockFileSystem {
	return &mockFileSystem{
		files:       make(map[string]string),
		directories: make(map[string]bool),
	}
}

func (fs *mockFileSystem) AddFile(path, content string) {
	fs.files[path] = content
	fs.directories[path] = false
}

func (fs *mockFileSystem) AddDirectory(path string) {
	fs.directories[path] = true
}

func (fs *mockFileSystem) SetWriteError(err error) {
	fs.writeError = err
}

func (fs *mockFileSystem) SetReadError(err error) {
	fs.readError = err
}

func (fs *mockFileSystem) ReadFile(path string) (string, error) {
	if fs.readError != nil {
		return "", fs.readError
	}
	content, exists := fs.files[path]
	if !exists {
		return "", &mockError{"file does not exist"}
	}
	return content, nil
}

func (fs *mockFileSystem) WriteFile(path string, content string) error {
	if fs.writeError != nil {
		return fs.writeError
	}
	fs.files[path] = content
	fs.directories[path] = false
	return nil
}

func (fs *mockFileSystem) FileExists(path string) bool {
	_, exists := fs.files[path]
	if exists {
		return true
	}
	_, exists = fs.directories[path]
	return exists
}

func (fs *mockFileSystem) IsDir(path string) bool {
	return fs.directories[path]
}

func (fs *mockFileSystem) IsAbs(path string) bool {
	return strings.HasPrefix(path, "/")
}

func (fs *mockFileSystem) ListDir(path string) (string, error) {
	if !fs.IsDir(path) {
		return "", &mockError{"not a directory"}
	}
	// Simulate find command output
	result := []string{}
	for filePath := range fs.files {
		if strings.HasPrefix(filePath, path) && filePath != path {
			result = append(result, filePath)
		}
	}
	for dirPath := range fs.directories {
		if strings.HasPrefix(dirPath, path) && dirPath != path {
			result = append(result, dirPath)
		}
	}
	return strings.Join(result, "\n"), nil
}

func (fs *mockFileSystem) MkdirAll(path string, perm os.FileMode) error {
	fs.directories[path] = true
	return nil
}

func (fs *mockFileSystem) FileSize(path string) (int64, error) {
	content, exists := fs.files[path]
	if !exists {
		return 0, &mockError{"file does not exist"}
	}
	return int64(len(content)), nil
}

type mockError struct {
	message string
}

func (e *mockError) Error() string {
	return e.message
}
