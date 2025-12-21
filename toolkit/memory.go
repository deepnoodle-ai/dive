package toolkit

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
)

var (
	_ dive.TypedTool[*MemoryToolInput] = &MemoryTool{}
	_ llm.ToolConfiguration            = &MemoryTool{}
)

const (
	// MaxMemoryLines is the maximum number of lines a memory file can have.
	MaxMemoryLines = 999999
	// MemorySnippetLines is the number of context lines to show around edits.
	MemorySnippetLines = 5
)

// MemoryCommand represents the available commands for the memory tool.
type MemoryCommand string

const (
	MemoryCommandView       MemoryCommand = "view"
	MemoryCommandCreate     MemoryCommand = "create"
	MemoryCommandStrReplace MemoryCommand = "str_replace"
	MemoryCommandInsert     MemoryCommand = "insert"
	MemoryCommandDelete     MemoryCommand = "delete"
	MemoryCommandRename     MemoryCommand = "rename"
)

// MemoryStorage defines the interface for memory storage backends.
// Implementations can store memory files in different ways (filesystem, database, cloud, etc.).
type MemoryStorage interface {
	// ReadFile reads the content of a file at the given path.
	ReadFile(path string) (string, error)

	// WriteFile writes content to a file at the given path.
	WriteFile(path string, content string) error

	// DeleteFile deletes a file at the given path.
	DeleteFile(path string) error

	// DeleteDir deletes a directory and all its contents at the given path.
	DeleteDir(path string) error

	// Rename renames/moves a file or directory from oldPath to newPath.
	Rename(oldPath, newPath string) error

	// Exists returns true if the path exists (file or directory).
	Exists(path string) bool

	// IsDir returns true if the path is a directory.
	IsDir(path string) bool

	// ListDir returns a listing of the directory with sizes.
	// The listing should be formatted with human-readable sizes and paths.
	ListDir(path string, maxDepth int) (string, error)

	// FileSize returns the size of a file in bytes.
	FileSize(path string) (int64, error)
}

// MemoryToolInput represents the input parameters for the memory tool.
type MemoryToolInput struct {
	Command    MemoryCommand `json:"command"`
	Path       string        `json:"path,omitempty"`
	FileText   *string       `json:"file_text,omitempty"`
	ViewRange  []int         `json:"view_range,omitempty"`
	OldStr     *string       `json:"old_str,omitempty"`
	NewStr     *string       `json:"new_str,omitempty"`
	InsertLine *int          `json:"insert_line,omitempty"`
	InsertText *string       `json:"insert_text,omitempty"`
	OldPath    *string       `json:"old_path,omitempty"`
	NewPath    *string       `json:"new_path,omitempty"`
}

// MemoryToolOptions are the options used to configure a MemoryTool.
type MemoryToolOptions struct {
	// Type is the Anthropic tool type identifier.
	Type string
	// Name is the tool name.
	Name string
	// Storage is the memory storage backend (defaults to InMemoryStorage).
	Storage MemoryStorage
	// MemoryDir is the base directory for memory files (defaults to "/memories").
	MemoryDir string
}

// NewMemoryTool creates a new MemoryTool with the given options.
func NewMemoryTool(opts ...MemoryToolOptions) *dive.TypedToolAdapter[*MemoryToolInput] {
	var resolvedOpts MemoryToolOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	if resolvedOpts.Type == "" {
		resolvedOpts.Type = "memory_20250818"
	}
	if resolvedOpts.Name == "" {
		resolvedOpts.Name = "memory"
	}
	if resolvedOpts.Storage == nil {
		resolvedOpts.Storage = NewInMemoryStorage()
	}
	if resolvedOpts.MemoryDir == "" {
		resolvedOpts.MemoryDir = "/memories"
	}
	return dive.ToolAdapter(&MemoryTool{
		typeString: resolvedOpts.Type,
		name:       resolvedOpts.Name,
		storage:    resolvedOpts.Storage,
		memoryDir:  resolvedOpts.MemoryDir,
	})
}

// MemoryTool implements Anthropic's memory tool specification.
// It enables Claude to store and retrieve information across conversations.
type MemoryTool struct {
	typeString string
	name       string
	storage    MemoryStorage
	memoryDir  string
}

func (t *MemoryTool) Name() string {
	return t.name
}

func (t *MemoryTool) Description() string {
	return `A memory tool for storing and retrieving information across conversations. This tool provides six main commands:

1. view: Read file contents or list directory contents
   - For files: optionally specify view_range [start_line, end_line] to see specific lines
   - For directories: lists files and subdirectories up to 2 levels deep with sizes
   - Returns numbered lines for easy reference

2. create: Create new files with specified content
   - Requires path and file_text parameter
   - Will fail if file already exists (cannot overwrite)

3. str_replace: Replace exact text matches in existing files
   - Requires old_str (exact text to find) and new_str (replacement text)
   - The old_str must appear exactly once in the file (unique match required)

4. insert: Insert text at a specific line number
   - Requires insert_line (0-indexed) and insert_text (text to insert)
   - Line 0 inserts at beginning, line N inserts after line N

5. delete: Delete a file or directory
   - Deletes the file or directory recursively

6. rename: Rename or move a file/directory
   - Requires old_path and new_path parameters

IMPORTANT: All paths must be within the /memories directory.`
}

func (t *MemoryTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Property{
			"command": {
				Type:        "string",
				Description: "The command to execute: view, create, str_replace, insert, delete, rename",
				Enum:        []any{"view", "create", "str_replace", "insert", "delete", "rename"},
			},
			"path": {
				Type:        "string",
				Description: "Path to the file or directory (for view, create, str_replace, insert, delete)",
			},
			"file_text": {
				Type:        "string",
				Description: "Text content for create command",
			},
			"view_range": {
				Type:        "array",
				Description: "Optional line range for view command [start_line, end_line]",
				Items: &schema.Property{
					Type: "integer",
				},
			},
			"old_str": {
				Type:        "string",
				Description: "String to replace (for str_replace command)",
			},
			"new_str": {
				Type:        "string",
				Description: "Replacement string (for str_replace command)",
			},
			"insert_line": {
				Type:        "integer",
				Description: "Line number to insert at (for insert command, 0-indexed)",
			},
			"insert_text": {
				Type:        "string",
				Description: "Text to insert (for insert command)",
			},
			"old_path": {
				Type:        "string",
				Description: "Source path (for rename command)",
			},
			"new_path": {
				Type:        "string",
				Description: "Destination path (for rename command)",
			},
		},
		Required: []string{"command"},
	}
}

func (t *MemoryTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "memory",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

func (t *MemoryTool) Call(ctx context.Context, input *MemoryToolInput) (*dive.ToolResult, error) {
	switch input.Command {
	case MemoryCommandView:
		return t.handleView(input.Path, input.ViewRange)
	case MemoryCommandCreate:
		return t.handleCreate(input.Path, input.FileText)
	case MemoryCommandStrReplace:
		return t.handleStrReplace(input.Path, input.OldStr, input.NewStr)
	case MemoryCommandInsert:
		return t.handleInsert(input.Path, input.InsertLine, input.InsertText)
	case MemoryCommandDelete:
		return t.handleDelete(input.Path)
	case MemoryCommandRename:
		return t.handleRename(input.OldPath, input.NewPath)
	default:
		return dive.NewToolResultError(fmt.Sprintf("Unrecognized command %s. The allowed commands are: view, create, str_replace, insert, delete, rename", input.Command)), nil
	}
}

// validatePath validates that a path is within the memory directory.
// It prevents path traversal attacks by resolving the path and checking it starts with memoryDir.
func (t *MemoryTool) validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	// Decode URL-encoded paths to prevent traversal via encoded sequences like %2e%2e
	decodedPath, err := url.PathUnescape(path)
	if err != nil {
		return fmt.Errorf("invalid path encoding")
	}

	// Check for traversal patterns in both original and decoded paths
	if strings.Contains(decodedPath, "..") {
		return fmt.Errorf("path traversal not allowed")
	}

	// Clean the path to resolve . and ..
	cleanPath := filepath.Clean(decodedPath)

	// Check if path starts with the memory directory
	if !strings.HasPrefix(cleanPath, t.memoryDir) {
		return fmt.Errorf("path must be within %s directory", t.memoryDir)
	}

	// Additional check: ensure no path traversal after cleaning
	relPath, err := filepath.Rel(t.memoryDir, cleanPath)
	if err != nil {
		return fmt.Errorf("invalid path")
	}
	if strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("path traversal not allowed")
	}

	return nil
}

func (t *MemoryTool) handleView(path string, viewRange []int) (*dive.ToolResult, error) {
	if err := t.validatePath(path); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("The path %s does not exist. Please provide a valid path.", path)), nil
	}

	if !t.storage.Exists(path) {
		return dive.NewToolResultError(fmt.Sprintf("The path %s does not exist. Please provide a valid path.", path)), nil
	}

	// Handle directory viewing
	if t.storage.IsDir(path) {
		if len(viewRange) > 0 {
			return dive.NewToolResultError("view_range is not allowed for directories"), nil
		}

		output, err := t.storage.ListDir(path, 2)
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error listing directory: %v", err)), nil
		}

		result := fmt.Sprintf("Here're the files and directories up to 2 levels deep in %s, excluding hidden items and node_modules:\n%s", path, output)
		return dive.NewToolResultText(result), nil
	}

	// Handle file viewing
	content, err := t.storage.ReadFile(path)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error reading file: %v", err)), nil
	}

	lines := strings.Split(content, "\n")
	nLines := len(lines)

	// Check line limit
	if nLines > MaxMemoryLines {
		return dive.NewToolResultError(fmt.Sprintf("File %s exceeds maximum line limit of %d lines.", path, MaxMemoryLines)), nil
	}

	initLine := 1
	if len(viewRange) > 0 {
		if len(viewRange) != 2 {
			return dive.NewToolResultError("invalid view_range. It should be a list of two integers [start_line, end_line]"), nil
		}

		start, end := viewRange[0], viewRange[1]

		if start < 1 || start > nLines {
			return dive.NewToolResultError(fmt.Sprintf("invalid view_range: [%d, %d]. First element should be within range [1, %d]", start, end, nLines)), nil
		}
		if end > nLines {
			return dive.NewToolResultError(fmt.Sprintf("invalid view_range: [%d, %d]. Second element should be <= %d", start, end, nLines)), nil
		}
		if end != -1 && end < start {
			return dive.NewToolResultError(fmt.Sprintf("invalid view_range: [%d, %d]. Second element should be >= first element", start, end)), nil
		}

		initLine = start
		if end == -1 {
			content = strings.Join(lines[start-1:], "\n")
		} else {
			content = strings.Join(lines[start-1:end], "\n")
		}
	}

	output := t.makeOutput(content, path, initLine)
	return dive.NewToolResultText(output), nil
}

func (t *MemoryTool) handleCreate(path string, fileText *string) (*dive.ToolResult, error) {
	if err := t.validatePath(path); err != nil {
		return dive.NewToolResultError(err.Error()), nil
	}

	if fileText == nil {
		return dive.NewToolResultError("file_text is required for create command"), nil
	}

	if t.storage.Exists(path) {
		return dive.NewToolResultError(fmt.Sprintf("Error: File %s already exists", path)), nil
	}

	if err := t.storage.WriteFile(path, *fileText); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error creating file: %v", err)), nil
	}

	return dive.NewToolResultText(fmt.Sprintf("File created successfully at: %s", path)), nil
}

func (t *MemoryTool) handleStrReplace(path string, oldStr, newStr *string) (*dive.ToolResult, error) {
	if err := t.validatePath(path); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error: The path %s does not exist. Please provide a valid path.", path)), nil
	}

	if !t.storage.Exists(path) {
		return dive.NewToolResultError(fmt.Sprintf("Error: The path %s does not exist. Please provide a valid path.", path)), nil
	}

	if t.storage.IsDir(path) {
		return dive.NewToolResultError(fmt.Sprintf("Error: The path %s does not exist. Please provide a valid path.", path)), nil
	}

	if oldStr == nil {
		return dive.NewToolResultError("old_str is required for str_replace command"), nil
	}

	content, err := t.storage.ReadFile(path)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error reading file: %v", err)), nil
	}

	// Check if old_str exists and is unique
	occurrences := strings.Count(content, *oldStr)
	if occurrences == 0 {
		return dive.NewToolResultError(fmt.Sprintf("No replacement was performed, old_str `%s` did not appear verbatim in %s.", *oldStr, path)), nil
	}
	if occurrences > 1 {
		lines := strings.Split(content, "\n")
		lineNumbers := []int{}
		for i, line := range lines {
			if strings.Contains(line, *oldStr) {
				lineNumbers = append(lineNumbers, i+1)
			}
		}
		return dive.NewToolResultError(fmt.Sprintf("No replacement was performed. Multiple occurrences of old_str `%s` in lines: %v. Please ensure it is unique", *oldStr, lineNumbers)), nil
	}

	newStrValue := ""
	if newStr != nil {
		newStrValue = *newStr
	}

	// Perform replacement
	newContent := strings.Replace(content, *oldStr, newStrValue, 1)

	if err := t.storage.WriteFile(path, newContent); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error writing file: %v", err)), nil
	}

	// Generate snippet for feedback
	snippet := t.generateEditSnippet(content, newContent, *oldStr, newStrValue)
	successMsg := fmt.Sprintf("The memory file has been edited.%s", snippet)

	return dive.NewToolResultText(successMsg), nil
}

func (t *MemoryTool) handleInsert(path string, insertLine *int, insertText *string) (*dive.ToolResult, error) {
	if err := t.validatePath(path); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error: The path %s does not exist", path)), nil
	}

	if !t.storage.Exists(path) {
		return dive.NewToolResultError(fmt.Sprintf("Error: The path %s does not exist", path)), nil
	}

	if t.storage.IsDir(path) {
		return dive.NewToolResultError(fmt.Sprintf("Error: The path %s does not exist", path)), nil
	}

	if insertLine == nil {
		return dive.NewToolResultError("insert_line is required for insert command"), nil
	}
	if insertText == nil {
		return dive.NewToolResultError("insert_text is required for insert command"), nil
	}

	content, err := t.storage.ReadFile(path)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error reading file: %v", err)), nil
	}

	lines := strings.Split(content, "\n")
	nLines := len(lines)

	if *insertLine < 0 || *insertLine > nLines {
		return dive.NewToolResultError(fmt.Sprintf("Error: Invalid `insert_line` parameter: %d. It should be within the range of lines of the file: [0, %d]", *insertLine, nLines)), nil
	}

	// Insert new content
	newStrLines := strings.Split(*insertText, "\n")
	newLines := make([]string, 0, len(lines)+len(newStrLines))
	newLines = append(newLines, lines[:*insertLine]...)
	newLines = append(newLines, newStrLines...)
	newLines = append(newLines, lines[*insertLine:]...)

	newContent := strings.Join(newLines, "\n")

	if err := t.storage.WriteFile(path, newContent); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error writing file: %v", err)), nil
	}

	return dive.NewToolResultText(fmt.Sprintf("The file %s has been edited.", path)), nil
}

func (t *MemoryTool) handleDelete(path string) (*dive.ToolResult, error) {
	if err := t.validatePath(path); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error: The path %s does not exist", path)), nil
	}

	if !t.storage.Exists(path) {
		return dive.NewToolResultError(fmt.Sprintf("Error: The path %s does not exist", path)), nil
	}

	var err error
	if t.storage.IsDir(path) {
		err = t.storage.DeleteDir(path)
	} else {
		err = t.storage.DeleteFile(path)
	}

	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error deleting: %v", err)), nil
	}

	return dive.NewToolResultText(fmt.Sprintf("Successfully deleted %s", path)), nil
}

func (t *MemoryTool) handleRename(oldPath, newPath *string) (*dive.ToolResult, error) {
	if oldPath == nil {
		return dive.NewToolResultError("old_path is required for rename command"), nil
	}
	if newPath == nil {
		return dive.NewToolResultError("new_path is required for rename command"), nil
	}

	if err := t.validatePath(*oldPath); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error: The path %s does not exist", *oldPath)), nil
	}
	if err := t.validatePath(*newPath); err != nil {
		return dive.NewToolResultError(err.Error()), nil
	}

	if !t.storage.Exists(*oldPath) {
		return dive.NewToolResultError(fmt.Sprintf("Error: The path %s does not exist", *oldPath)), nil
	}

	if t.storage.Exists(*newPath) {
		return dive.NewToolResultError(fmt.Sprintf("Error: The destination %s already exists", *newPath)), nil
	}

	if err := t.storage.Rename(*oldPath, *newPath); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error renaming: %v", err)), nil
	}

	return dive.NewToolResultText(fmt.Sprintf("Successfully renamed %s to %s", *oldPath, *newPath)), nil
}

func (t *MemoryTool) generateEditSnippet(originalContent, newContent, oldStr, newStr string) string {
	// Find the line where replacement occurred
	beforeReplacement := strings.Split(originalContent, oldStr)[0]
	replacementLine := strings.Count(beforeReplacement, "\n")

	startLine := replacementLine - MemorySnippetLines
	if startLine < 0 {
		startLine = 0
	}
	endLine := replacementLine + MemorySnippetLines + strings.Count(newStr, "\n")

	lines := strings.Split(newContent, "\n")
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}

	snippet := strings.Join(lines[startLine:endLine+1], "\n")
	return t.makeOutput(snippet, "snippet", startLine+1)
}

func (t *MemoryTool) makeOutput(content, descriptor string, initLine int) string {
	lines := strings.Split(content, "\n")
	numberedLines := make([]string, len(lines))
	for i, line := range lines {
		numberedLines[i] = fmt.Sprintf("%6d\t%s", i+initLine, line)
	}

	numberedContent := strings.Join(numberedLines, "\n")
	return fmt.Sprintf("Here's the content of %s with line numbers:\n%s", descriptor, numberedContent)
}

func (t *MemoryTool) ToolConfiguration(providerName string) map[string]any {
	if providerName == "anthropic" {
		return map[string]any{"type": t.typeString, "name": t.name}
	}
	return nil
}

// InMemoryStorage implements MemoryStorage using an in-memory map.
// This is useful for testing and ephemeral memory that doesn't persist.
type InMemoryStorage struct {
	files       map[string]string
	directories map[string]bool
}

// NewInMemoryStorage creates a new InMemoryStorage with the /memories directory.
func NewInMemoryStorage() *InMemoryStorage {
	storage := &InMemoryStorage{
		files:       make(map[string]string),
		directories: make(map[string]bool),
	}
	// Create the root memories directory
	storage.directories["/memories"] = true
	return storage
}

func (s *InMemoryStorage) ReadFile(path string) (string, error) {
	content, exists := s.files[path]
	if !exists {
		return "", fmt.Errorf("file not found: %s", path)
	}
	return content, nil
}

func (s *InMemoryStorage) WriteFile(path string, content string) error {
	// Ensure parent directories exist
	dir := filepath.Dir(path)
	s.ensureDir(dir)

	s.files[path] = content
	return nil
}

func (s *InMemoryStorage) DeleteFile(path string) error {
	if _, exists := s.files[path]; !exists {
		return fmt.Errorf("file not found: %s", path)
	}
	delete(s.files, path)
	return nil
}

func (s *InMemoryStorage) DeleteDir(path string) error {
	if !s.directories[path] {
		return fmt.Errorf("directory not found: %s", path)
	}

	// Delete all files and subdirectories under this path
	for filePath := range s.files {
		if strings.HasPrefix(filePath, path+"/") || filePath == path {
			delete(s.files, filePath)
		}
	}
	for dirPath := range s.directories {
		if strings.HasPrefix(dirPath, path+"/") || dirPath == path {
			delete(s.directories, dirPath)
		}
	}

	return nil
}

func (s *InMemoryStorage) Rename(oldPath, newPath string) error {
	// Check if it's a file
	if content, exists := s.files[oldPath]; exists {
		// Ensure parent directory of new path exists
		s.ensureDir(filepath.Dir(newPath))
		s.files[newPath] = content
		delete(s.files, oldPath)
		return nil
	}

	// Check if it's a directory
	if s.directories[oldPath] {
		// Move all files and directories under oldPath to newPath
		for filePath, content := range s.files {
			if strings.HasPrefix(filePath, oldPath+"/") {
				newFilePath := newPath + filePath[len(oldPath):]
				s.files[newFilePath] = content
				delete(s.files, filePath)
			}
		}
		for dirPath := range s.directories {
			if strings.HasPrefix(dirPath, oldPath+"/") || dirPath == oldPath {
				newDirPath := newPath + dirPath[len(oldPath):]
				s.directories[newDirPath] = true
				delete(s.directories, dirPath)
			}
		}
		s.directories[newPath] = true
		return nil
	}

	return fmt.Errorf("path not found: %s", oldPath)
}

func (s *InMemoryStorage) Exists(path string) bool {
	if _, exists := s.files[path]; exists {
		return true
	}
	return s.directories[path]
}

func (s *InMemoryStorage) IsDir(path string) bool {
	return s.directories[path]
}

func (s *InMemoryStorage) ListDir(path string, maxDepth int) (string, error) {
	if !s.directories[path] {
		return "", fmt.Errorf("not a directory: %s", path)
	}

	var result strings.Builder

	// Collect all entries under this path
	entries := make(map[string]int64) // path -> size

	// Add the directory itself
	result.WriteString(fmt.Sprintf("4.0K\t%s\n", path))

	// Add files and directories
	for filePath, content := range s.files {
		if !strings.HasPrefix(filePath, path+"/") && filePath != path {
			continue
		}
		// Check depth
		relPath := filePath[len(path)+1:]
		depth := strings.Count(relPath, "/") + 1
		if depth > maxDepth {
			continue
		}
		// Skip hidden files
		parts := strings.Split(relPath, "/")
		hidden := false
		for _, part := range parts {
			if strings.HasPrefix(part, ".") {
				hidden = true
				break
			}
		}
		if hidden {
			continue
		}
		// Skip node_modules
		if strings.Contains(filePath, "node_modules") {
			continue
		}
		entries[filePath] = int64(len(content))
	}

	for dirPath := range s.directories {
		if !strings.HasPrefix(dirPath, path+"/") || dirPath == path {
			continue
		}
		// Check depth
		relPath := dirPath[len(path)+1:]
		depth := strings.Count(relPath, "/") + 1
		if depth > maxDepth {
			continue
		}
		// Skip hidden directories
		parts := strings.Split(relPath, "/")
		hidden := false
		for _, part := range parts {
			if strings.HasPrefix(part, ".") {
				hidden = true
				break
			}
		}
		if hidden {
			continue
		}
		// Skip node_modules
		if strings.Contains(dirPath, "node_modules") {
			continue
		}
		entries[dirPath] = 4096 // Standard directory size
	}

	// Format output
	for entryPath, size := range entries {
		result.WriteString(fmt.Sprintf("%s\t%s\n", formatSize(size), entryPath))
	}

	return result.String(), nil
}

func (s *InMemoryStorage) FileSize(path string) (int64, error) {
	content, exists := s.files[path]
	if !exists {
		return 0, fmt.Errorf("file not found: %s", path)
	}
	return int64(len(content)), nil
}

func (s *InMemoryStorage) ensureDir(path string) {
	parts := strings.Split(path, "/")
	current := ""
	for _, part := range parts {
		if part == "" {
			current = "/"
			continue
		}
		if current == "/" {
			current = "/" + part
		} else {
			current = current + "/" + part
		}
		s.directories[current] = true
	}
}

// formatSize formats a byte size as a human-readable string (e.g., "1.5K", "2.3M").
func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FileSystemStorage implements MemoryStorage using the actual filesystem.
type FileSystemStorage struct {
	basePath string
}

// NewFileSystemStorage creates a new FileSystemStorage rooted at the given base path.
// The base path should be an absolute path on the filesystem where /memories will be mapped.
func NewFileSystemStorage(basePath string) (*FileSystemStorage, error) {
	// Ensure base path exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}
	return &FileSystemStorage{basePath: basePath}, nil
}

// mapPath converts a virtual /memories path to an actual filesystem path.
func (s *FileSystemStorage) mapPath(virtualPath string) string {
	// Remove /memories prefix and append to base path
	if strings.HasPrefix(virtualPath, "/memories") {
		relativePath := virtualPath[len("/memories"):]
		return filepath.Join(s.basePath, relativePath)
	}
	return filepath.Join(s.basePath, virtualPath)
}

func (s *FileSystemStorage) ReadFile(path string) (string, error) {
	realPath := s.mapPath(path)
	data, err := os.ReadFile(realPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *FileSystemStorage) WriteFile(path string, content string) error {
	realPath := s.mapPath(path)
	// Ensure parent directory exists
	dir := filepath.Dir(realPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(realPath, []byte(content), 0644)
}

func (s *FileSystemStorage) DeleteFile(path string) error {
	realPath := s.mapPath(path)
	return os.Remove(realPath)
}

func (s *FileSystemStorage) DeleteDir(path string) error {
	realPath := s.mapPath(path)
	return os.RemoveAll(realPath)
}

func (s *FileSystemStorage) Rename(oldPath, newPath string) error {
	realOldPath := s.mapPath(oldPath)
	realNewPath := s.mapPath(newPath)

	// Ensure parent directory of new path exists
	dir := filepath.Dir(realNewPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.Rename(realOldPath, realNewPath)
}

func (s *FileSystemStorage) Exists(path string) bool {
	realPath := s.mapPath(path)
	_, err := os.Stat(realPath)
	return !os.IsNotExist(err)
}

func (s *FileSystemStorage) IsDir(path string) bool {
	realPath := s.mapPath(path)
	info, err := os.Stat(realPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func (s *FileSystemStorage) ListDir(path string, maxDepth int) (string, error) {
	realPath := s.mapPath(path)

	var result strings.Builder

	err := filepath.WalkDir(realPath, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip entries we can't access
		}

		// Get relative path for depth calculation
		relPath, err := filepath.Rel(realPath, p)
		if err != nil {
			return nil
		}

		// Skip hidden files and directories
		name := d.Name()
		if len(name) > 0 && name[0] == '.' && relPath != "." {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip node_modules
		if name == "node_modules" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Calculate depth
		depth := 0
		if relPath != "." {
			depth = strings.Count(relPath, string(filepath.Separator)) + 1
		}

		// Skip if deeper than maxDepth
		if depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Get size
		var size int64 = 4096 // Default for directories
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				size = info.Size()
			}
		}

		// Convert real path back to virtual path
		virtualPath := path
		if relPath != "." {
			virtualPath = path + "/" + relPath
		}

		result.WriteString(fmt.Sprintf("%s\t%s\n", formatSize(size), virtualPath))
		return nil
	})

	if err != nil {
		return "", err
	}

	return result.String(), nil
}

func (s *FileSystemStorage) FileSize(path string) (int64, error) {
	realPath := s.mapPath(path)
	info, err := os.Stat(realPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
