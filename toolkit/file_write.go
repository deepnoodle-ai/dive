package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/diveagents/dive"
	"github.com/diveagents/dive/schema"
)

var _ dive.TypedTool[*FileWriteInput] = &FileWriteTool{}

type FileWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type FileWriteToolOptions struct {
	AllowList     []string // Patterns of allowed paths
	DenyList      []string // Patterns of denied paths
	RootDirectory string   // If set, all paths will be relative to this directory
	Confirmer     dive.Confirmer
}

type FileWriteTool struct {
	allowList     []string // Patterns of allowed paths
	denyList      []string // Patterns of denied paths
	rootDirectory string   // If set, all paths will be relative to this directory
	confirmer     dive.Confirmer
}

// NewFileWriteTool creates a new tool for writing content to files
func NewFileWriteTool(options FileWriteToolOptions) *dive.TypedToolAdapter[*FileWriteInput] {
	return dive.ToolAdapter(&FileWriteTool{
		allowList:     options.AllowList,
		denyList:      options.DenyList,
		rootDirectory: options.RootDirectory,
		confirmer:     options.Confirmer,
	})
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
	cleanRoot := filepath.Clean(absRoot)
	cleanPath := filepath.Clean(absPath)

	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
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

func (t *FileWriteTool) Name() string {
	return "file_write"
}

func (t *FileWriteTool) Description() string {
	return "A tool that writes content to a file. To use this tool, provide a 'path' parameter with the path to the file you want to write to, and a 'content' parameter with the content to write."
}

func (t *FileWriteTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"path", "content"},
		Properties: map[string]*schema.Property{
			"path": {
				Type:        "string",
				Description: "Path to the file to be written",
			},
			"content": {
				Type:        "string",
				Description: "Content to write to the file",
			},
		},
	}
}

func (t *FileWriteTool) Call(ctx context.Context, input *FileWriteInput) (*dive.ToolResult, error) {
	filePath := input.Path
	if filePath == "" {
		return dive.NewToolResultError("Error: No file path provided. Please provide a file path either in the constructor or as an argument."), nil
	}
	resolvedPath, err := t.resolvePath(filePath)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error: %s", err.Error())), nil
	}
	allowed, reason := t.isPathAllowed(resolvedPath)
	if !allowed {
		return dive.NewToolResultError(fmt.Sprintf("Error: Access denied. %s", reason)), nil
	}
	dir := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error: Failed to create directory structure for %s. %s", filePath, err.Error())), nil
	}
	err = os.WriteFile(resolvedPath, []byte(input.Content), 0644)
	if err != nil {
		if os.IsPermission(err) {
			return dive.NewToolResultError(fmt.Sprintf("Error: Permission denied when trying to write to file: %s", filePath)), nil
		}
		return dive.NewToolResultError(fmt.Sprintf("Error: Failed to write to file %s. %s", filePath, err.Error())), nil
	}
	return dive.NewToolResultText(fmt.Sprintf("Successfully wrote %d bytes to %s", len(input.Content), filePath)), nil
}

func (t *FileWriteTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "File Write",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}
