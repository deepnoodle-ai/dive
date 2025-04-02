package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/diveagents/dive/llm"
)

const DefaultDirectoryListMaxEntries = 250

type DirectoryListInput struct {
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

type DirectoryListToolOptions struct {
	DefaultPath   string
	MaxEntries    int
	RootDirectory string   // If set, all paths will be relative to this directory
	AllowList     []string // Patterns of allowed paths
	DenyList      []string // Patterns of denied paths
}

type DirectoryListTool struct {
	defaultPath   string
	maxEntries    int
	rootDirectory string
	allowList     []string
	denyList      []string
}

// NewDirectoryListTool creates a new tool for listing directory contents
func NewDirectoryListTool(options DirectoryListToolOptions) *DirectoryListTool {
	maxEntries := options.MaxEntries
	if maxEntries <= 0 {
		maxEntries = DefaultDirectoryListMaxEntries
	}

	return &DirectoryListTool{
		defaultPath:   options.DefaultPath,
		maxEntries:    maxEntries,
		rootDirectory: options.RootDirectory,
		allowList:     options.AllowList,
		denyList:      options.DenyList,
	}
}

func (t *DirectoryListTool) Definition() *llm.ToolDefinition {
	description := "A tool that lists the contents of a directory. To use this tool, provide a 'path' parameter with the path to the directory you want to list."

	if t.defaultPath != "" {
		description = fmt.Sprintf("A tool that lists the contents of a directory. The default directory is %s, but you can provide a different 'path' parameter to list another directory.", t.defaultPath)
	}

	if t.rootDirectory != "" {
		description += fmt.Sprintf(" All paths are relative to %s.", t.rootDirectory)
	}

	if len(t.allowList) > 0 || len(t.denyList) > 0 {
		description += " Directory paths are restricted based on configured allowlist and denylist patterns."
	}

	return &llm.ToolDefinition{
		Name:        "directory_list",
		Description: description,
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]*llm.SchemaProperty{
				"path": {
					Type:        "string",
					Description: "Path to the directory to be listed",
				},
			},
		},
	}
}

// isPathAllowed checks if the given path is allowed based on allowList and denyList
func (t *DirectoryListTool) isPathAllowed(path string) (bool, string) {
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

// resolvePath resolves the provided path, applying rootDirectory if configured
// and preventing directory traversal attacks
func (t *DirectoryListTool) resolvePath(path string) (string, error) {
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

func (t *DirectoryListTool) Call(ctx context.Context, input string) (string, error) {
	var params DirectoryListInput
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", err
	}

	dirPath := params.Path
	if dirPath == "" {
		dirPath = t.defaultPath
	}

	if dirPath == "" {
		return "Error: No directory path provided. Please provide a directory path either in the constructor or as an argument.", nil
	}

	// Resolve the path (apply rootDirectory if configured)
	resolvedPath, err := t.resolvePath(dirPath)
	if err != nil {
		return fmt.Sprintf("Error: %s", err.Error()), nil
	}

	// Check if the path is allowed
	if len(t.allowList) > 0 || len(t.denyList) > 0 {
		allowed, reason := t.isPathAllowed(resolvedPath)
		if !allowed {
			return fmt.Sprintf("Error: Access denied. %s", reason), nil
		}
	}

	// Check if the directory exists
	fileInfo, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: Directory not found at path: %s", dirPath), nil
		} else if os.IsPermission(err) {
			return fmt.Sprintf("Error: Permission denied when trying to access directory: %s", dirPath), nil
		}
		return fmt.Sprintf("Error: Failed to access directory %s. %s", dirPath, err.Error()), nil
	}

	// Check if it's actually a directory
	if !fileInfo.IsDir() {
		return fmt.Sprintf("Error: Path %s is not a directory", dirPath), nil
	}

	// Read directory entries
	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return fmt.Sprintf("Error: Failed to read directory %s. %s", dirPath, err.Error()), nil
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

		// For display purposes, use the path relative to rootDirectory if it was set
		displayPath := entryPath
		if t.rootDirectory != "" {
			// Try to make the path relative to rootDirectory for display
			relPath, err := filepath.Rel(t.rootDirectory, resolvedPath)
			if err == nil {
				displayPath = filepath.Join(relPath, entry.Name())
			}
		}

		extension := ""
		if !entry.IsDir() {
			extension = filepath.Ext(entry.Name())
		}

		result = append(result, DirectoryEntry{
			Name:      entry.Name(),
			Path:      displayPath,
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
		return fmt.Sprintf("Error: Failed to format directory listing. %s", err.Error()), nil
	}

	// Add a message if we limited the entries
	message := fmt.Sprintf("Directory listing for %s", dirPath)
	if len(entries) == t.maxEntries {
		message += fmt.Sprintf(" (limited to %d entries)", t.maxEntries)
	}

	return fmt.Sprintf("%s:\n\n%s", message, string(jsonResult)), nil
}

func (t *DirectoryListTool) ShouldReturnResult() bool {
	return true
}
