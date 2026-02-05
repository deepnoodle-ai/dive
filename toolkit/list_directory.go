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

// DefaultListDirectoryMaxEntries is the default limit on entries returned.
const DefaultListDirectoryMaxEntries = 100

// ListDirectoryInput represents the input parameters for the ListDirectory tool.
type ListDirectoryInput struct {
	// Path is the directory to list. Required unless a default path is configured.
	Path string `json:"path"`
}

// DirectoryEntry represents a single file or directory in the listing.
type DirectoryEntry struct {
	// Name is the base name of the file or directory.
	Name string `json:"name"`

	// Path is the full path to the entry.
	Path string `json:"path"`

	// Size is the file size in bytes (0 for directories).
	Size int64 `json:"size"`

	// IsDir is true if this entry is a directory.
	IsDir bool `json:"is_dir"`

	// Mode is the Unix-style permission string (e.g., "-rw-r--r--").
	Mode string `json:"mode"`

	// ModTime is the last modification time.
	ModTime time.Time `json:"mod_time"`

	// Extension is the file extension (empty for directories).
	Extension string `json:"extension,omitempty"`
}

// ListDirectoryToolOptions configures the behavior of [ListDirectoryTool].
type ListDirectoryToolOptions struct {
	// DefaultPath is used when no path is provided in the input.
	DefaultPath string

	// MaxEntries limits the number of entries returned to prevent
	// overwhelming responses for large directories.
	// Defaults to [DefaultListDirectoryMaxEntries] (100).
	MaxEntries int

	// WorkspaceDir restricts listings to paths within this directory.
	// Defaults to the current working directory if empty.
	WorkspaceDir string
}

// ListDirectoryTool lists the contents of a directory with metadata.
//
// Unlike a simple "ls" command, this tool returns structured JSON output
// with detailed metadata including file sizes, permissions, modification
// times, and file extensions. This structured output makes it easier for
// LLMs to analyze directory contents.
//
// Features:
//   - JSON output with comprehensive file metadata
//   - Configurable entry limit to prevent large responses
//   - Extension extraction for easy file type filtering
type ListDirectoryTool struct {
	defaultPath   string
	maxEntries    int
	pathValidator *PathValidator
}

// NewListDirectoryTool creates a new ListDirectoryTool with the given options.
func NewListDirectoryTool(opts ...ListDirectoryToolOptions) *dive.TypedToolAdapter[*ListDirectoryInput] {
	var options ListDirectoryToolOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	if options.MaxEntries == 0 {
		options.MaxEntries = DefaultListDirectoryMaxEntries
	}
	var pathValidator *PathValidator
	if options.WorkspaceDir != "" {
		pathValidator, _ = NewPathValidator(options.WorkspaceDir)
	}
	return dive.ToolAdapter(&ListDirectoryTool{
		defaultPath:   options.DefaultPath,
		maxEntries:    options.MaxEntries,
		pathValidator: pathValidator,
	})
}

// Name returns "ListDirectory" as the tool identifier.
func (t *ListDirectoryTool) Name() string {
	return "ListDirectory"
}

// Description returns usage instructions for the LLM.
func (t *ListDirectoryTool) Description() string {
	return "A tool that lists the contents of a directory. To use this tool, provide a 'path' parameter with the path to the directory you want to list."
}

// Schema returns the JSON schema describing the tool's input parameters.
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

// Annotations returns metadata hints about the tool's behavior.
// ListDirectory is marked as read-only and idempotent.
func (t *ListDirectoryTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "ListDirectory",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

// PreviewCall returns a summary of the list operation for permission prompts.
func (t *ListDirectoryTool) PreviewCall(ctx context.Context, input *ListDirectoryInput) *dive.ToolCallPreview {
	path := input.Path
	if path == "" {
		path = t.defaultPath
	}
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("List %s", path),
	}
}

// Call lists the directory contents and returns them as JSON.
//
// The result includes a message indicating the directory path and an
// array of [DirectoryEntry] objects. If the entry count exceeds MaxEntries,
// only the first MaxEntries items are returned with a note about the limit.
func (t *ListDirectoryTool) Call(ctx context.Context, input *ListDirectoryInput) (*dive.ToolResult, error) {
	dirPath := input.Path
	if dirPath == "" {
		dirPath = t.defaultPath
	}

	if dirPath == "" {
		return NewToolResultError("Error: No directory path provided. Please provide a directory path either in the constructor or as an argument."), nil
	}

	// Validate path is within workspace (skip validation if no validator configured)
	if t.pathValidator != nil {
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
