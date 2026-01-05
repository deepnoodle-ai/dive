package toolkit

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

var _ dive.TypedTool[*ReadFileInput] = &ReadFileTool{}
var _ dive.TypedToolPreviewer[*ReadFileInput] = &ReadFileTool{}

const DefaultReadFileMaxSize = 1024 * 100 // 100KB

type ReadFileInput struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"` // Line number to start reading from (1-based)
	Limit    int    `json:"limit,omitempty"`  // Number of lines to read
}

type ReadFileToolOptions struct {
	MaxSize      int    `json:"max_size,omitempty"`
	WorkspaceDir string // Base directory for workspace validation (defaults to cwd)
}

type ReadFileTool struct {
	maxSize       int
	pathValidator *PathValidator
}

// NewReadFileTool creates a new tool for reading file contents
func NewReadFileTool(options ReadFileToolOptions) *dive.TypedToolAdapter[*ReadFileInput] {
	if options.MaxSize == 0 {
		options.MaxSize = DefaultReadFileMaxSize
	}
	pathValidator, err := NewPathValidator(options.WorkspaceDir)
	if err != nil {
		// Store nil to indicate validation is unavailable - will fail closed at call time
		pathValidator = nil
	}
	return dive.ToolAdapter(&ReadFileTool{
		maxSize:       options.MaxSize,
		pathValidator: pathValidator,
	})
}

func (t *ReadFileTool) Name() string {
	return "Read"
}

func (t *ReadFileTool) Description() string {
	return `Read a file from the filesystem.

By default, reads up to 2000 lines starting from the beginning. Use offset and limit
for reading specific portions of large files.

Supports text files, and will warn if content appears to be binary.`
}

func (t *ReadFileTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"file_path"},
		Properties: map[string]*schema.Property{
			"file_path": {
				Type:        "string",
				Description: "The absolute path to the file to read",
			},
			"offset": {
				Type:        "integer",
				Description: "The line number to start reading from (1-based). Only provide if the file is too large to read at once.",
			},
			"limit": {
				Type:        "integer",
				Description: "The number of lines to read. Only provide if the file is too large to read at once.",
			},
		},
	}
}

func (t *ReadFileTool) PreviewCall(ctx context.Context, input *ReadFileInput) *dive.ToolCallPreview {
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Read %s", input.FilePath),
	}
}

func (t *ReadFileTool) Call(ctx context.Context, input *ReadFileInput) (*dive.ToolResult, error) {
	filePath := input.FilePath
	if filePath == "" {
		return NewToolResultError("Error: No file path provided."), nil
	}

	// Validate path is within workspace (fail closed if validator unavailable)
	if t.pathValidator == nil {
		return NewToolResultError("Error: path validation unavailable - cannot safely perform file operations"), nil
	}
	if err := t.pathValidator.ValidateRead(filePath); err != nil {
		return NewToolResultError(fmt.Sprintf("Error: %s", err.Error())), nil
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("failed to resolve absolute path: %s", err.Error())), nil
	}

	// Open file first to avoid TOCTOU race conditions
	file, err := os.Open(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewToolResultError(fmt.Sprintf("Error: File not found at path: %s", filePath)), nil
		} else if os.IsPermission(err) {
			return NewToolResultError(fmt.Sprintf("Error: Permission denied when trying to access file: %s", filePath)), nil
		}
		return NewToolResultError(fmt.Sprintf("Error: Failed to access file %s. %s", filePath, err.Error())), nil
	}
	defer file.Close()

	// Stat the open file handle to avoid TOCTOU issues
	fileInfo, err := file.Stat()
	if err != nil {
		return NewToolResultError(fmt.Sprintf("Error: Failed to get file info for %s. %s", filePath, err.Error())), nil
	}

	if fileInfo.IsDir() {
		return NewToolResultError(fmt.Sprintf("Error: Path is a directory, not a file: %s", filePath)), nil
	}

	// If no offset/limit, read the whole file (with size check)
	if input.Offset == 0 && input.Limit == 0 {
		if fileInfo.Size() > int64(t.maxSize) {
			return NewToolResultError(fmt.Sprintf("Error: File %s is too large (%d bytes). Maximum allowed size is %d bytes. Use offset and limit parameters to read portions.",
				filePath, fileInfo.Size(), t.maxSize)), nil
		}

		content, err := io.ReadAll(file)
		if err != nil {
			return NewToolResultError(fmt.Sprintf("Error: Failed to read file %s. %s", filePath, err.Error())), nil
		}

		if isBinaryContent(content) {
			return NewToolResultError(fmt.Sprintf("Warning: File %s appears to be a binary file.", filePath)), nil
		}

		return NewToolResultText(string(content)).
			WithDisplay(fmt.Sprintf("Read %s (%d bytes)", filePath, len(content))), nil
	}

	// Read with offset/limit (line-based)
	scanner := bufio.NewScanner(file)
	var lines []string
	lineNum := 0
	startLine := input.Offset
	if startLine < 1 {
		startLine = 1
	}
	maxLines := input.Limit
	if maxLines <= 0 {
		maxLines = 2000 // Default limit
	}

	for scanner.Scan() {
		lineNum++
		if lineNum < startLine {
			continue
		}
		if len(lines) >= maxLines {
			break
		}
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return NewToolResultError(fmt.Sprintf("Error reading file: %s", err.Error())), nil
	}

	// Format with line numbers like cat -n
	var result strings.Builder
	for i, line := range lines {
		result.WriteString(fmt.Sprintf("%6d\t%s\n", startLine+i, line))
	}

	display := fmt.Sprintf("Read %s (lines %d-%d)", filePath, startLine, startLine+len(lines)-1)
	return NewToolResultText(result.String()).WithDisplay(display), nil
}

// isBinaryContent attempts to determine if the content is binary by checking for null bytes
// and examining the ratio of control characters to printable characters
func isBinaryContent(content []byte) bool {
	// Quick check: if there are null bytes, it's likely binary
	if bytes.Contains(content, []byte{0}) {
		return true
	}

	// Check a sample of the file (up to first 512 bytes)
	sampleSize := 512
	if len(content) < sampleSize {
		sampleSize = len(content)
	}

	sample := content[:sampleSize]
	controlCount := 0

	for _, b := range sample {
		// Count control characters (except common whitespace)
		if (b < 32 && b != 9 && b != 10 && b != 13) || b > 126 {
			controlCount++
		}
	}

	// If more than 10% are control characters, likely binary
	return controlCount > sampleSize/10
}

func (t *ReadFileTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Read",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}
