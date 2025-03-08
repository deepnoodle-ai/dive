package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/getstingrai/dive/llm"
)

type FileWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type FileWriteToolOptions struct {
	DefaultFilePath string
	AllowList       []string // Patterns of allowed paths
	DenyList        []string // Patterns of denied paths
	RootDirectory   string   // If set, all paths will be relative to this directory
}

type FileWriteTool struct {
	defaultFilePath string
	allowList       []string // Patterns of allowed paths
	denyList        []string // Patterns of denied paths
	rootDirectory   string   // If set, all paths will be relative to this directory
}

// NewFileWriteTool creates a new tool for writing content to files
func NewFileWriteTool(options FileWriteToolOptions) *FileWriteTool {
	return &FileWriteTool{
		defaultFilePath: options.DefaultFilePath,
		allowList:       options.AllowList,
		denyList:        options.DenyList,
		rootDirectory:   options.RootDirectory,
	}
}

// resolvePath resolves the provided path, applying rootDirectory if configured
// and preventing directory traversal attacks
func (t *FileWriteTool) resolvePath(path string) (string, error) {
	if t.rootDirectory == "" {
		// If no root directory is set, use the path as is
		return path, nil
	}

	// Join the root directory and the provided path
	resolvedPath := filepath.Join(t.rootDirectory, path)

	// Get the absolute paths to check for directory traversal
	absRoot, err := filepath.Abs(t.rootDirectory)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for root directory: %w", err)
	}

	absPath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Check if the resolved path is within the root directory
	if !filepath.HasPrefix(absPath, absRoot) {
		return "", fmt.Errorf("path attempts to access location outside of root directory")
	}

	return resolvedPath, nil
}

// isPathAllowed checks if the given path is allowed based on allowList and denyList
func (t *FileWriteTool) isPathAllowed(path string) (bool, string) {
	// Convert to absolute path for consistent checking
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Sprintf("Error resolving absolute path: %s", err.Error())
	}

	// If denyList is specified, check against it first
	if len(t.denyList) > 0 {
		for _, pattern := range t.denyList {
			matched, err := matchesPattern(absPath, pattern)
			if err != nil {
				return false, fmt.Sprintf("Error matching pattern '%s': %s", pattern, err.Error())
			}
			if matched {
				return false, fmt.Sprintf("Path '%s' matches denied pattern '%s'", path, pattern)
			}
		}
	}

	// If allowList is specified, path must match at least one pattern
	if len(t.allowList) > 0 {
		allowed := false
		for _, pattern := range t.allowList {
			matched, err := matchesPattern(absPath, pattern)
			if err != nil {
				return false, fmt.Sprintf("Error matching pattern '%s': %s", pattern, err.Error())
			}
			if matched {
				allowed = true
				break
			}
		}
		if !allowed {
			return false, fmt.Sprintf("Path '%s' does not match any allowed patterns", path)
		}
	}

	return true, ""
}

func matchesPattern(path, pattern string) (bool, error) {
	return doublestar.PathMatch(pattern, path)
}

func (t *FileWriteTool) Definition() *llm.ToolDefinition {
	description := "A tool that writes content to a file. To use this tool, provide a 'path' parameter with the path to the file you want to write to, and a 'content' parameter with the content to write."

	if t.defaultFilePath != "" {
		description = fmt.Sprintf("A tool that writes content to a file. The default file is %s, but you can provide a different 'path' parameter to write to another file.", t.defaultFilePath)
	}

	if t.rootDirectory != "" {
		description += fmt.Sprintf(" All paths are relative to %s.", t.rootDirectory)
	}

	if len(t.allowList) > 0 || len(t.denyList) > 0 {
		description += " File paths are restricted based on configured allowlist and denylist patterns."
	}

	return &llm.ToolDefinition{
		Name:        "file_write",
		Description: description,
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"path", "content"},
			Properties: map[string]*llm.SchemaProperty{
				"path": {
					Type:        "string",
					Description: "Path to the file to be written",
				},
				"content": {
					Type:        "string",
					Description: "Content to write to the file",
				},
			},
		},
	}
}

func (t *FileWriteTool) Call(ctx context.Context, input string) (string, error) {
	var params FileWriteInput
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", err
	}

	filePath := params.Path
	if filePath == "" {
		filePath = t.defaultFilePath
	}

	if filePath == "" {
		return "Error: No file path provided. Please provide a file path either in the constructor or as an argument.", nil
	}

	// Resolve the path (apply rootDirectory if configured)
	resolvedPath, err := t.resolvePath(filePath)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}

	// Check if the path is allowed
	allowed, reason := t.isPathAllowed(resolvedPath)
	if !allowed {
		return fmt.Sprintf("Error: Access denied. %s", reason), nil
	}

	// Ensure the directory exists
	dir := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Sprintf("Error: Failed to create directory structure for %s. %s", filePath, err.Error()), nil
	}

	// Write the content to the file
	err = os.WriteFile(resolvedPath, []byte(params.Content), 0644)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Sprintf("Error: Permission denied when trying to write to file: %s", filePath), nil
		}
		return fmt.Sprintf("Error: Failed to write to file %s. %s", filePath, err.Error()), nil
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(params.Content), filePath), nil
}

func (t *FileWriteTool) ShouldReturnResult() bool {
	return true
}
