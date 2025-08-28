package toolkit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/schema"
)

var _ dive.TypedTool[*ReadFileInput] = &ReadFileTool{}

const DefaultReadFileMaxSize = 1024 * 100 // 100KB

type ReadFileInput struct {
	Path string `json:"path"`
}

type ReadFileToolOptions struct {
	MaxSize int `json:"max_size,omitempty"`
}

type ReadFileTool struct {
	maxSize int
}

// NewReadFileTool creates a new tool for reading file contents
func NewReadFileTool(options ReadFileToolOptions) *dive.TypedToolAdapter[*ReadFileInput] {
	if options.MaxSize == 0 {
		options.MaxSize = DefaultReadFileMaxSize
	}
	return dive.ToolAdapter(&ReadFileTool{
		maxSize: options.MaxSize,
	})
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) Description() string {
	return "A tool that reads the content of a file. To use this tool, provide a 'path' parameter with the path to the file you want to read."
}

func (t *ReadFileTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"path"},
		Properties: map[string]*schema.Property{
			"path": {
				Type:        "string",
				Description: "Path to the file to be read",
			},
		},
	}
}

func (t *ReadFileTool) Call(ctx context.Context, input *ReadFileInput) (*dive.ToolResult, error) {
	filePath := input.Path
	if filePath == "" {
		return NewToolResultError("Error: No file path provided."), nil
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("failed to resolve absolute path: %s", err.Error())), nil
	}

	fileInfo, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewToolResultError(fmt.Sprintf("Error: File not found at path: %s", filePath)), nil
		} else if os.IsPermission(err) {
			return NewToolResultError(fmt.Sprintf("Error: Permission denied when trying to access file: %s", filePath)), nil
		}
		return NewToolResultError(fmt.Sprintf("Error: Failed to access file %s. %s", filePath, err.Error())), nil
	}

	if fileInfo.Size() > int64(t.maxSize) {
		return NewToolResultError(fmt.Sprintf("Error: File %s is too large (%d bytes). Maximum allowed size is %d bytes.",
			filePath, fileInfo.Size(), t.maxSize)), nil
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return NewToolResultError(fmt.Sprintf("Error: Failed to read file %s. %s", filePath, err.Error())), nil
	}

	if isBinaryContent(content) {
		return NewToolResultError(fmt.Sprintf("Warning: File %s appears to be a binary file. The content may not display correctly:\n\n%s",
			filePath, string(content))), nil
	}
	return NewToolResultText(string(content)), nil
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
		Title:           "read_file",
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}
