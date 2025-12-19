package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/schema"
)

var (
	_ dive.TypedTool[*EditInput]          = &EditTool{}
	_ dive.TypedToolPreviewer[*EditInput] = &EditTool{}
)

// EditInput represents the input parameters for the edit tool
type EditInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// EditToolOptions configures the EditTool
type EditToolOptions struct {
	// MaxFileSize is the maximum file size to edit (default 10MB)
	MaxFileSize int64
}

// EditTool performs exact string replacements in files
type EditTool struct {
	maxFileSize int64
}

// NewEditTool creates a new EditTool
func NewEditTool(opts ...EditToolOptions) *dive.TypedToolAdapter[*EditInput] {
	var resolvedOpts EditToolOptions
	if len(opts) > 0 {
		resolvedOpts = opts[0]
	}
	if resolvedOpts.MaxFileSize == 0 {
		resolvedOpts.MaxFileSize = 10 * 1024 * 1024 // 10MB
	}
	return dive.ToolAdapter(&EditTool{
		maxFileSize: resolvedOpts.MaxFileSize,
	})
}

func (t *EditTool) Name() string {
	return "edit"
}

func (t *EditTool) Description() string {
	return `Perform exact string replacements in files.

This tool replaces exact text matches in a file. Use it when you need to make
precise edits to existing files.

Requirements:
- old_string must be unique in the file (unless using replace_all: true)
- new_string must be different from old_string
- The file must exist

Use replace_all: true when you want to replace all occurrences, such as
renaming a variable throughout a file.

Examples:
- Fix a typo: {"file_path": "/path/to/file.go", "old_string": "teh", "new_string": "the"}
- Rename variable: {"file_path": "/path/to/file.go", "old_string": "oldName", "new_string": "newName", "replace_all": true}
- Update import: {"file_path": "/path/to/file.go", "old_string": "\"old/package\"", "new_string": "\"new/package\""}`
}

func (t *EditTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type:     "object",
		Required: []string{"file_path", "old_string", "new_string"},
		Properties: map[string]*schema.Property{
			"file_path": {
				Type:        "string",
				Description: "The absolute path to the file to modify",
			},
			"old_string": {
				Type:        "string",
				Description: "The exact text to replace (must be unique in the file unless using replace_all)",
			},
			"new_string": {
				Type:        "string",
				Description: "The text to replace it with (must be different from old_string)",
			},
			"replace_all": {
				Type:        "boolean",
				Description: "Replace all occurrences of old_string (default: false). Use for renaming variables.",
			},
		},
	}
}

func (t *EditTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "Edit File",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
}

func (t *EditTool) PreviewCall(ctx context.Context, input *EditInput) *dive.ToolCallPreview {
	filename := filepath.Base(input.FilePath)
	action := "Replace"
	if input.ReplaceAll {
		action = "Replace all"
	}

	// Truncate strings for preview
	oldStr := input.OldString
	if len(oldStr) > 30 {
		oldStr = oldStr[:30] + "..."
	}
	oldStr = strings.ReplaceAll(oldStr, "\n", "\\n")

	return &dive.ToolCallPreview{
		Summary: fmt.Sprintf("%s in %s: %q", action, filename, oldStr),
	}
}

func (t *EditTool) Call(ctx context.Context, input *EditInput) (*dive.ToolResult, error) {
	// Validate inputs
	if input.OldString == input.NewString {
		return dive.NewToolResultError("old_string and new_string must be different"), nil
	}

	// Validate path
	if !filepath.IsAbs(input.FilePath) {
		return dive.NewToolResultError(fmt.Sprintf("file_path must be absolute, got: %s", input.FilePath)), nil
	}

	// Check file exists
	info, err := os.Stat(input.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return dive.NewToolResultError(fmt.Sprintf("File does not exist: %s", input.FilePath)), nil
		}
		return dive.NewToolResultError(fmt.Sprintf("Error accessing file: %v", err)), nil
	}

	if info.IsDir() {
		return dive.NewToolResultError(fmt.Sprintf("Path is a directory, not a file: %s", input.FilePath)), nil
	}

	if info.Size() > t.maxFileSize {
		return dive.NewToolResultError(fmt.Sprintf("File too large: %d bytes (max %d bytes)", info.Size(), t.maxFileSize)), nil
	}

	// Read file
	content, err := os.ReadFile(input.FilePath)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error reading file: %v", err)), nil
	}

	contentStr := string(content)

	// Count occurrences
	count := strings.Count(contentStr, input.OldString)

	if count == 0 {
		return dive.NewToolResultError(fmt.Sprintf("old_string not found in file: %q", truncateForError(input.OldString, 50))), nil
	}

	if count > 1 && !input.ReplaceAll {
		// Find line numbers of occurrences
		lines := strings.Split(contentStr, "\n")
		var lineNumbers []int
		for i, line := range lines {
			if strings.Contains(line, input.OldString) {
				lineNumbers = append(lineNumbers, i+1)
			}
		}
		return dive.NewToolResultError(fmt.Sprintf(
			"old_string appears %d times (lines %v). Use replace_all: true to replace all, or provide a more specific string.",
			count, lineNumbers,
		)), nil
	}

	// Perform replacement
	var newContent string
	if input.ReplaceAll {
		newContent = strings.ReplaceAll(contentStr, input.OldString, input.NewString)
	} else {
		newContent = strings.Replace(contentStr, input.OldString, input.NewString, 1)
	}

	// Write file back
	if err := os.WriteFile(input.FilePath, []byte(newContent), info.Mode()); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error writing file: %v", err)), nil
	}

	// Generate success message with snippet
	snippet := t.generateSnippet(newContent, input.NewString)

	var resultMsg string
	if input.ReplaceAll {
		resultMsg = fmt.Sprintf("Replaced %d occurrence(s) in %s", count, input.FilePath)
	} else {
		resultMsg = fmt.Sprintf("Replaced 1 occurrence in %s", input.FilePath)
	}

	display := resultMsg
	if snippet != "" {
		resultMsg += "\n\n" + snippet
	}

	return dive.NewToolResultText(resultMsg).WithDisplay(display), nil
}

// generateSnippet creates a code snippet showing the edited area
func (t *EditTool) generateSnippet(content, newString string) string {
	lines := strings.Split(content, "\n")

	// Find the line(s) containing the new string
	var matchLine int = -1
	for i, line := range lines {
		if strings.Contains(line, newString) || (len(newString) > 0 && strings.Contains(line, strings.Split(newString, "\n")[0])) {
			matchLine = i
			break
		}
	}

	if matchLine == -1 {
		return ""
	}

	// Show context around the edit
	const contextLines = 3
	start := matchLine - contextLines
	if start < 0 {
		start = 0
	}
	end := matchLine + contextLines + 1
	if end > len(lines) {
		end = len(lines)
	}

	var snippet strings.Builder
	snippet.WriteString("```\n")
	for i := start; i < end; i++ {
		snippet.WriteString(fmt.Sprintf("%4d │ %s\n", i+1, lines[i]))
	}
	snippet.WriteString("```")

	return snippet.String()
}

func truncateForError(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
