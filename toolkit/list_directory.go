package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

var _ dive.TypedTool[*ListDirectoryInput] = &ListDirectoryTool{}
var _ dive.TypedToolPreviewer[*ListDirectoryInput] = &ListDirectoryTool{}

const DefaultListDirectoryMaxEntries = 100

type ListDirectoryInput struct {
	Path string `json:"path"`
}

type DirectoryEntry struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	IsDir     bool      `json:"is_dir"`
	Mode      string    `json:"mode"`
	ModTime   time.Time `json:"mod_time"`
	Extension string    `json:"extension,omitempty"`
}

type ListDirectoryToolOptions struct {
	DefaultPath  string
	MaxEntries   int
	WorkspaceDir string // Base directory for workspace validation (defaults to cwd)
}

type ListDirectoryTool struct {
	defaultPath   string
	maxEntries    int
	pathValidator *PathValidator
}

// NewListDirectoryTool creates a new tool for listing directory contents
func NewListDirectoryTool(options ListDirectoryToolOptions) *dive.TypedToolAdapter[*ListDirectoryInput] {
	if options.MaxEntries == 0 {
		options.MaxEntries = DefaultListDirectoryMaxEntries
	}
	pathValidator, err := NewPathValidator(options.WorkspaceDir)
	if err != nil {
		pathValidator = &PathValidator{}
	}
	return dive.ToolAdapter(&ListDirectoryTool{
		defaultPath:   options.DefaultPath,
		maxEntries:    options.MaxEntries,
		pathValidator: pathValidator,
	})
}

func (t *ListDirectoryTool) Name() string {
	return "list_directory"
}

func (t *ListDirectoryTool) Description() string {
	return "A tool that lists the contents of a directory. To use this tool, provide a 'path' parameter with the path to the directory you want to list."
}

func (t *ListDirectoryTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Property{
			"path": {
				Type:        "string",
				Description: "The path to the directory you want to list.",
			},
		},
		Required: []string{"path"},
	}
}

func (t *ListDirectoryTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "List Directory",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

func (t *ListDirectoryTool) PreviewCall(ctx context.Context, input *ListDirectoryInput) *dive.ToolCallPreview {
	path := input.Path
	if path == "" {
		path = t.defaultPath
	}
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("List %s", path),
	}
}

func (t *ListDirectoryTool) Call(ctx context.Context, input *ListDirectoryInput) (*dive.ToolResult, error) {
	dirPath := input.Path
	if dirPath == "" {
		dirPath = t.defaultPath
	}

	if dirPath == "" {
		return NewToolResultError("Error: No directory path provided. Please provide a directory path either in the constructor or as an argument."), nil
	}

	// Validate path is within workspace
	if t.pathValidator != nil && t.pathValidator.WorkspaceDir != "" {
		if err := t.pathValidator.ValidateRead(dirPath); err != nil {
			return NewToolResultError(fmt.Sprintf("Error: %s", err.Error())), nil
		}
	}

	// Resolve to absolute path
	resolvedPath, err := filepath.Abs(dirPath)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("Unable to resolve path: %s", err.Error())), nil
	}

	// Check if the directory exists
	fileInfo, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewToolResultError(fmt.Sprintf("Directory not found at path: %s", dirPath)), nil
		} else if os.IsPermission(err) {
			return NewToolResultError(fmt.Sprintf("Permission denied when trying to access directory: %s", dirPath)), nil
		}
		return NewToolResultError(fmt.Sprintf("Failed to access directory %s. %s", dirPath, err.Error())), nil
	}

	// Check if it's actually a directory
	if !fileInfo.IsDir() {
		return NewToolResultError(fmt.Sprintf("Path %s is not a directory", dirPath)), nil
	}

	// Read directory entries
	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("Failed to read directory %s. %s", dirPath, err.Error())), nil
	}

	// Limit the number of entries to avoid overwhelming responses
	if len(entries) > t.maxEntries {
		entries = entries[:t.maxEntries]
	}

	// Convert to our structured format
	result := make([]DirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue // Skip entries we can't get info for
		}

		entryPath := filepath.Join(dirPath, entry.Name())

		extension := ""
		if !entry.IsDir() {
			extension = filepath.Ext(entry.Name())
		}

		result = append(result, DirectoryEntry{
			Name:      entry.Name(),
			Path:      entryPath,
			Size:      info.Size(),
			IsDir:     entry.IsDir(),
			Mode:      info.Mode().String(),
			ModTime:   info.ModTime(),
			Extension: extension,
		})
	}

	// Convert to JSON for the response
	jsonResult, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return NewToolResultError(fmt.Sprintf("Failed to format directory listing. %s", err.Error())), nil
	}

	// Add a message if we limited the entries
	message := fmt.Sprintf("Directory listing for %s", dirPath)
	if len(entries) == t.maxEntries {
		message += fmt.Sprintf(" (limited to %d entries)", t.maxEntries)
	}

	display := fmt.Sprintf("Listed %d entries", len(result))
	return NewToolResultText(fmt.Sprintf("%s:\n\n%s", message, string(jsonResult))).
		WithDisplay(display), nil
}
