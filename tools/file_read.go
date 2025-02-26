package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/getstingrai/dive/llm"
)

type FileReadInput struct {
	Path string `json:"path"`
}

type FileReadTool struct {
	defaultFilePath string
	maxSize         int
}

// NewFileReadTool creates a new tool for reading file contents
func NewFileReadTool(defaultFilePath string, maxSize int) *FileReadTool {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &FileReadTool{
		defaultFilePath: defaultFilePath,
		maxSize:         maxSize,
	}
}

func (t *FileReadTool) Definition() *llm.ToolDefinition {
	description := "A tool that reads the content of a file. To use this tool, provide a 'path' parameter with the path to the file you want to read."

	if t.defaultFilePath != "" {
		description = fmt.Sprintf("A tool that reads file content. The default file is %s, but you can provide a different 'path' parameter to read another file.", t.defaultFilePath)
	}

	return &llm.ToolDefinition{
		Name:        "ReadFile",
		Description: description,
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]*llm.SchemaProperty{
				"path": {
					Type:        "string",
					Description: "Path to the file to be read",
				},
			},
		},
	}
}

func (t *FileReadTool) Call(ctx context.Context, input string) (string, error) {
	var params FileReadInput
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", err
	}

	filePath := params.Path
	if filePath == "" {
		filePath = t.defaultFilePath
	}

	if filePath == "" {
		return "Error: No file path provided. Please provide a file path either in the constructor or as an argument.", nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: File not found at path: %s", filePath), nil
		} else if os.IsPermission(err) {
			return fmt.Sprintf("Error: Permission denied when trying to read file: %s", filePath), nil
		}
		return fmt.Sprintf("Error: Failed to read file %s. %s", filePath, err.Error()), nil
	}

	// Truncate content if it exceeds the maximum size
	result := string(content)
	if len(result) > t.maxSize {
		result = result[:t.maxSize] + " ..."
	}

	return result, nil
}

func (t *FileReadTool) ShouldReturnResult() bool {
	return true
}
