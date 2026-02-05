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

// DefaultReadFileMaxSize is the default maximum file size in bytes (100KB).
const DefaultReadFileMaxSize = 1024 * 100

// ReadFileInput represents the input parameters for the Read tool.
type ReadFileInput struct {
	// FilePath is the absolute path to the file to read. Required.
	FilePath string `json:"file_path"`

	// Offset is the 1-based line number to start reading from.
	// When combined with Limit, enables reading specific portions of large files.
	// Defaults to 1 (start of file).
	Offset int `json:"offset,omitempty"`

	// Limit is the maximum number of lines to read.
	// Defaults to 2000 when Offset is specified, otherwise reads the entire file.
	Limit int `json:"limit,omitempty"`
}

// ReadFileToolOptions configures the behavior of [ReadFileTool].
type ReadFileToolOptions struct {
	// MaxSize is the maximum file size in bytes that can be read entirely.
	// Files larger than this require using Offset and Limit parameters.
	// Defaults to [DefaultReadFileMaxSize] (100KB).
	MaxSize int `json:"max_size,omitempty"`

	// WorkspaceDir restricts file reads to paths within this directory.
	// Defaults to the current working directory if empty.
	WorkspaceDir string
}

// ReadFileTool reads file contents from the filesystem.
//
// This tool provides flexible file reading with support for both full-file
// reads and partial reads using line offsets. It detects binary files and
// warns appropriately.
//
// Features:
//   - Full file reading with size limits
//   - Partial reading via offset and limit for large files
//   - Line numbers in output (cat -n style) when using offset/limit
//   - Binary file detection to avoid garbled output
type ReadFileTool struct {
	maxSize       int
	pathValidator *PathValidator
}

// NewReadFileTool creates a new ReadFileTool with the given options.
func NewReadFileTool(options ReadFileToolOptions) *dive.TypedToolAdapter[*ReadFileInput] {
	if options.MaxSize == 0 {
		options.MaxSize = DefaultReadFileMaxSize
	}
	var pathValidator *PathValidator
	if options.WorkspaceDir != "" {
		pathValidator, _ = NewPathValidator(options.WorkspaceDir)
	}
	return dive.ToolAdapter(&ReadFileTool{
		maxSize:       options.MaxSize,
		pathValidator: pathValidator,
	})
}

// Name returns "Read" as the tool identifier.
func (t *ReadFileTool) Name() string {
	return "Read"
}

// Description returns detailed usage instructions for the LLM.
func (t *ReadFileTool) Description() string {
	return `Read a file from the filesystem.

By default, reads up to 2000 lines starting from the beginning. Use offset and limit
for reading specific portions of large files.

Supports text files, and will warn if content appears to be binary.`
}

// Schema returns the JSON schema describing the tool's input parameters.
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

// PreviewCall returns a summary of the read operation for permission prompts.
func (t *ReadFileTool) PreviewCall(ctx context.Context, input *ReadFileInput) *dive.ToolCallPreview {
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Read %s", input.FilePath),
	}
}

// Call reads the file contents and returns them.
//
// When Offset and Limit are not specified, reads the entire file (subject to
// MaxSize). When specified, reads the requested line range and includes
// line numbers in the output.
//
// Binary files are detected by checking for null bytes and control characters.
// If detected, an error is returned instead of garbled content.
func (t *ReadFileTool) Call(ctx context.Context, input *ReadFileInput) (*dive.ToolResult, error) {
	filePath := input.FilePath
	if filePath == "" {
		return NewToolResultError("Error: No file path provided."), nil
	}

	// Validate path is within workspace (skip validation if no validator configured)
	if t.pathValidator != nil {
		if err := t.pathValidator.ValidateRead(filePath); err != nil {
			return NewToolResultError(fmt.Sprintf("Error: %s", err.Error())), nil
		}
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

// Annotations returns metadata hints about the tool's behavior.
// Read is marked as read-only and idempotent.
func (t *ReadFileTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Read",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}
