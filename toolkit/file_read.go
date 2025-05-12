package toolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diveagents/dive/llm"
)

var _ llm.ToolWithMetadata = &FileReadTool{}

const DefaultFileReadMaxSize = 1024 * 200 // 200KB

type FileReadInput struct {
	Path string `json:"path"`
}

type FileReadToolOptions struct {
	MaxSize       int    `json:"max_size,omitempty"`
	RootDirectory string `json:"root_directory,omitempty"`
}

type FileReadTool struct {
	maxSize       int
	rootDirectory string
}

// NewFileReadTool creates a new tool for reading file contents
func NewFileReadTool(options FileReadToolOptions) *FileReadTool {
	if options.MaxSize <= 0 {
		options.MaxSize = DefaultFileReadMaxSize
	}
	return &FileReadTool{
		maxSize:       options.MaxSize,
		rootDirectory: options.RootDirectory,
	}
}

func (t *FileReadTool) Name() string {
	return "file_read"
}

func (t *FileReadTool) Description() string {
	return "A tool that reads the content of a file. To use this tool, provide a 'path' parameter with the path to the file you want to read."
}

func (t *FileReadTool) Schema() llm.Schema {
	return llm.Schema{
		Type:     "object",
		Required: []string{"path"},
		Properties: map[string]*llm.SchemaProperty{
			"path": {
				Type:        "string",
				Description: "Path to the file to be read",
			},
		},
	}
}

// resolvePath resolves the provided path, applying rootDirectory if configured
// and preventing directory traversal attacks
func (t *FileReadTool) resolvePath(path string) (string, error) {
	if t.rootDirectory == "" {
		return path, nil
	}
	resolvedPath := filepath.Join(t.rootDirectory, path)

	absRoot, err := filepath.Abs(t.rootDirectory)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for root directory: %w", err)
	}
	absPath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	// Check if the resolved path is within the root directory
	cleanRoot := filepath.Clean(absRoot)
	cleanPath := filepath.Clean(absPath)

	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", fmt.Errorf("path attempts to access location outside of root directory")
	}
	return resolvedPath, nil
}

func (t *FileReadTool) Call(ctx context.Context, input *llm.ToolCallInput) (*llm.ToolCallOutput, error) {
	var params FileReadInput
	if err := json.Unmarshal([]byte(input.Input), &params); err != nil {
		return nil, err
	}
	filePath := params.Path
	if filePath == "" {
		return llm.NewToolCallOutput("Error: No file path provided."), nil
	}
	resolvedPath, err := t.resolvePath(filePath)
	if err != nil {
		return llm.NewToolCallOutput(fmt.Sprintf("Error: %s", err.Error())), nil
	}
	fileInfo, err := os.Stat(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return llm.NewToolCallOutput(fmt.Sprintf("Error: File not found at path: %s", filePath)), nil
		} else if os.IsPermission(err) {
			return llm.NewToolCallOutput(fmt.Sprintf("Error: Permission denied when trying to access file: %s", filePath)), nil
		}
		return llm.NewToolCallOutput(fmt.Sprintf("Error: Failed to access file %s. %s", filePath, err.Error())), nil
	}
	if fileInfo.Size() > int64(t.maxSize) {
		return llm.NewToolCallOutput(fmt.Sprintf("Error: File %s is too large (%d bytes). Maximum allowed size is %d bytes.",
			filePath, fileInfo.Size(), t.maxSize)), nil
	}
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return llm.NewToolCallOutput(fmt.Sprintf("Error: Failed to read file %s. %s", filePath, err.Error())), nil
	}
	if isBinaryContent(content) {
		return llm.NewToolCallOutput(fmt.Sprintf("Warning: File %s appears to be a binary file. The content may not display correctly:\n\n%s",
			filePath, string(content))), nil
	}
	return llm.NewToolCallOutput(string(content)), nil
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

func (t *FileReadTool) Metadata() llm.ToolMetadata {
	return llm.ToolMetadata{
		Version:    "0.0.1",
		Capability: llm.ToolCapabilityReadOnly,
	}
}
