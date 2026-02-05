package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
)

var _ dive.TypedTool[*WriteFileInput] = &WriteFileTool{}
var _ dive.TypedToolPreviewer[*WriteFileInput] = &WriteFileTool{}

// WriteFileInput represents the input parameters for the Write tool.
type WriteFileInput struct {
	// FilePath is the absolute path to the file to write. Required.
	FilePath string `json:"file_path"`

	// Content is the text content to write to the file. Required.
	Content string `json:"content"`
}

// WriteFileToolOptions configures the behavior of [WriteFileTool].
type WriteFileToolOptions struct {
	// WorkspaceDir restricts file writes to paths within this directory.
	// Defaults to the current working directory if empty.
	WorkspaceDir string
}

// WriteFileTool writes content to files on the filesystem.
//
// This tool creates new files or overwrites existing ones with the provided
// content. Parent directories are created automatically if they don't exist.
//
// Features:
//   - Automatic parent directory creation
//   - Workspace path validation when configured
//   - Clear success message with byte count
//
// Security: This tool can overwrite any file within the workspace. The agent
// permission system should be used to control which files can be written.
type WriteFileTool struct {
	pathValidator *PathValidator
}

// NewWriteFileTool creates a new WriteFileTool with the given options.
func NewWriteFileTool(opts ...WriteFileToolOptions) *dive.TypedToolAdapter[*WriteFileInput] {
	var options WriteFileToolOptions
	if len(opts) > 0 {
		options = opts[0]
	}
	var pathValidator *PathValidator
	if options.WorkspaceDir != "" {
		pathValidator, _ = NewPathValidator(options.WorkspaceDir)
	}
	return dive.ToolAdapter(&WriteFileTool{
		pathValidator: pathValidator,
	})
}

// Name returns "Write" as the tool identifier.
func (t *WriteFileTool) Name() string {
	return "Write"
}

// Description returns usage instructions for the LLM.
func (t *WriteFileTool) Description() string {
	return "A tool that writes content to a file. Provide a 'file_path' parameter with the absolute path to the file you want to write to, and a 'content' parameter with the content to write."
}

// Schema returns the JSON schema describing the tool's input parameters.
func (t *WriteFileTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"file_path", "content"},
		Properties: map[string]*schema.Property{
			"file_path": {
				Type:        "string",
				Description: "The absolute path to the file to write (must be absolute, not relative)",
			},
			"content": {
				Type:        "string",
				Description: "The content to write to the file",
			},
		},
	}
}

// PreviewCall returns a summary of the write operation for permission prompts.
func (t *WriteFileTool) PreviewCall(ctx context.Context, input *WriteFileInput) *dive.ToolCallPreview {
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Write to %s", input.FilePath),
		Details: fmt.Sprintf("Writing %d bytes to `%s`", len(input.Content), input.FilePath),
	}
}

// Call writes the content to the specified file.
//
// Creates parent directories as needed. Overwrites existing files.
// Returns the number of bytes written on success.
func (t *WriteFileTool) Call(ctx context.Context, input *WriteFileInput) (*dive.ToolResult, error) {
	filePath := input.FilePath
	if filePath == "" {
		return dive.NewToolResultError("Error: No file path provided. Please provide a file path either in the constructor or as an argument."), nil
	}

	// Validate path is within workspace (skip validation if no validator configured)
	if t.pathValidator != nil {
		if err := t.pathValidator.ValidateWrite(filePath); err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error: %s", err.Error())), nil
		}
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
	bytesWritten := len(input.Content)
	return dive.NewToolResultText(fmt.Sprintf("Successfully wrote %d bytes to %s", bytesWritten, filePath)).
		WithDisplay(fmt.Sprintf("Wrote %d bytes to %s", bytesWritten, filePath)), nil
}

// Annotations returns metadata hints about the tool's behavior.
// Write is marked as destructive (can overwrite files), idempotent
// (same content produces same result), and has EditHint for special UI treatment.
func (t *WriteFileTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Write",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  true,
		OpenWorldHint:   false,
		EditHint:        true,
	}
}
