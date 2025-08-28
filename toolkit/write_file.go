package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/schema"
)

var _ dive.TypedTool[*WriteFileInput] = &WriteFileTool{}

type WriteFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type WriteFileToolOptions struct {
	AllowList []string // Patterns of allowed paths
	DenyList  []string // Patterns of denied paths
}

type WriteFileTool struct {
	allowList []string // Patterns of allowed paths
	denyList  []string // Patterns of denied paths
}

// NewWriteFileTool creates a new tool for writing content to files
func NewWriteFileTool(options WriteFileToolOptions) *dive.TypedToolAdapter[*WriteFileInput] {
	return dive.ToolAdapter(&WriteFileTool{
		allowList: options.AllowList,
		denyList:  options.DenyList,
	})
}

// isPathAllowed checks if the given path is allowed based on allowList and denyList
func (t *WriteFileTool) isPathAllowed(path string) (bool, string) {
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

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "A tool that writes content to a file. To use this tool, provide a 'path' parameter with the path to the file you want to write to, and a 'content' parameter with the content to write."
}

func (t *WriteFileTool) Schema() *schema.Schema {
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

func (t *WriteFileTool) Call(ctx context.Context, input *WriteFileInput) (*dive.ToolResult, error) {
	filePath := input.Path
	if filePath == "" {
		return dive.NewToolResultError("Error: No file path provided. Please provide a file path either in the constructor or as an argument."), nil
	}

	// Check if the path is allowed
	allowed, reason := t.isPathAllowed(filePath)
	if !allowed {
		return dive.NewToolResultError(fmt.Sprintf("Error: Access denied. %s", reason)), nil
	}

	// Convert to absolute path for file operations
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error: Failed to resolve absolute path for %s. %s", filePath, err.Error())), nil
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error: Failed to create directory structure for %s. %s", filePath, err.Error())), nil
	}

	err = os.WriteFile(absPath, []byte(input.Content), 0644)
	if err != nil {
		if os.IsPermission(err) {
			return dive.NewToolResultError(fmt.Sprintf("Error: Permission denied when trying to write to file: %s", filePath)), nil
		}
		return dive.NewToolResultError(fmt.Sprintf("Error: Failed to write to file %s. %s", filePath, err.Error())), nil
	}
	return dive.NewToolResultText(fmt.Sprintf("Successfully wrote %d bytes to %s", len(input.Content), filePath)), nil
}

func (t *WriteFileTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "write_file",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}
