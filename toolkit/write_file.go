package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/schema"
)

var _ dive.TypedTool[*WriteFileInput] = &WriteFileTool{}
var _ dive.TypedToolPreviewer[*WriteFileInput] = &WriteFileTool{}

type WriteFileInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

type WriteFileToolOptions struct {
	// WorkspaceDir is the base directory for workspace validation (defaults to cwd)
	WorkspaceDir string
}

type WriteFileTool struct {
	pathValidator *PathValidator
}

// NewWriteFileTool creates a new tool for writing content to files
func NewWriteFileTool(options WriteFileToolOptions) *dive.TypedToolAdapter[*WriteFileInput] {
	pathValidator, err := NewPathValidator(options.WorkspaceDir)
	if err != nil {
		pathValidator = &PathValidator{}
	}
	return dive.ToolAdapter(&WriteFileTool{
		pathValidator: pathValidator,
	})
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Description() string {
	return "A tool that writes content to a file. Provide a 'file_path' parameter with the absolute path to the file you want to write to, and a 'content' parameter with the content to write."
}

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

func (t *WriteFileTool) PreviewCall(ctx context.Context, input *WriteFileInput) *dive.ToolCallPreview {
	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("Write to %s", input.FilePath),
		Details: fmt.Sprintf("Writing %d bytes to `%s`", len(input.Content), input.FilePath),
	}
}

func (t *WriteFileTool) Call(ctx context.Context, input *WriteFileInput) (*dive.ToolResult, error) {
	filePath := input.FilePath
	if filePath == "" {
		return dive.NewToolResultError("Error: No file path provided. Please provide a file path either in the constructor or as an argument."), nil
	}

	// Validate path is within workspace
	if t.pathValidator != nil && t.pathValidator.WorkspaceDir != "" {
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

func (t *WriteFileTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "write_file",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}
