package toolkit

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/schema"
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
	// WorkspaceDir is the base directory for workspace validation (defaults to cwd)
	WorkspaceDir string
}

// EditTool performs exact string replacements in files
type EditTool struct {
	maxFileSize   int64
	pathValidator *PathValidator
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
	pathValidator, err := NewPathValidator(resolvedOpts.WorkspaceDir)
	if err != nil {
		pathValidator = &PathValidator{}
	}
	return dive.ToolAdapter(&EditTool{
		maxFileSize:   resolvedOpts.MaxFileSize,
		pathValidator: pathValidator,
	})
}

func (t *EditTool) Name() string {
	return "Edit"
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
		Title:           "Edit",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  true,
		OpenWorldHint:   false,
		EditHint:        true,
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

	// Validate path is within workspace
	if t.pathValidator != nil && t.pathValidator.WorkspaceDir != "" {
		if err := t.pathValidator.ValidateWrite(input.FilePath); err != nil {
			return dive.NewToolResultError(fmt.Sprintf("Error: %s", err.Error())), nil
		}
	}

	// Open file first to avoid TOCTOU race conditions
	file, err := os.Open(input.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return dive.NewToolResultError(fmt.Sprintf("File does not exist: %s", input.FilePath)), nil
		}
		return dive.NewToolResultError(fmt.Sprintf("Error accessing file: %v", err)), nil
	}
	defer file.Close()

	// Stat the open file handle to avoid TOCTOU issues
	info, err := file.Stat()
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("Error getting file info: %v", err)), nil
	}

	if info.IsDir() {
		return dive.NewToolResultError(fmt.Sprintf("Path is a directory, not a file: %s", input.FilePath)), nil
	}

	if info.Size() > t.maxFileSize {
		return dive.NewToolResultError(fmt.Sprintf("File too large: %d bytes (max %d bytes)", info.Size(), t.maxFileSize)), nil
	}

	// Read file from the already-open handle
	content, err := io.ReadAll(file)
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

	// Find line number where old_string starts (for diff context)
	lineNum := 1
	idx := strings.Index(contentStr, input.OldString)
	if idx >= 0 {
		lineNum = strings.Count(contentStr[:idx], "\n") + 1
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

	// Generate diff for display
	diff := t.generateDiff(contentStr, newContent, input.OldString, input.NewString, lineNum)

	var resultMsg string
	if input.ReplaceAll {
		resultMsg = fmt.Sprintf("Replaced %d occurrence(s) in %s", count, input.FilePath)
	} else {
		resultMsg = fmt.Sprintf("Replaced 1 occurrence in %s", input.FilePath)
	}

	// Result sent to LLM includes the snippet for context
	resultMsg += "\n\n" + diff

	return dive.NewToolResultText(resultMsg).WithDisplay(diff), nil
}

// Diff generation limits
const (
	maxDiffLines     = 50  // Maximum lines to show in diff output
	maxDiffLineWidth = 200 // Maximum characters per line before truncation
	contextBefore    = 2   // Lines of context before the change
	contextAfter     = 2   // Lines of context after the change
)

// generateDiff creates a Claude Code-style diff showing old vs new content
func (t *EditTool) generateDiff(oldContent, newContent, oldString, newString string, lineNum int) string {
	oldLines := strings.Split(oldString, "\n")
	newLines := strings.Split(newString, "\n")

	// Count additions and removals
	added := len(newLines)
	removed := len(oldLines)

	// Build summary
	var summary string
	if added > 0 && removed > 0 {
		if added == 1 && removed == 1 {
			summary = "Changed 1 line"
		} else {
			summary = fmt.Sprintf("Added %d line%s, removed %d line%s",
				added, pluralize(added), removed, pluralize(removed))
		}
	} else if added > 0 {
		summary = fmt.Sprintf("Added %d line%s", added, pluralize(added))
	} else if removed > 0 {
		summary = fmt.Sprintf("Removed %d line%s", removed, pluralize(removed))
	}

	// Check if diff would be too large - if so, return summary only
	totalDiffLines := added + removed + contextBefore + contextAfter
	if totalDiffLines > maxDiffLines {
		return fmt.Sprintf("%s (diff too large to display, %d lines changed)", summary, added+removed)
	}

	// Build the diff view with context
	contentLines := strings.Split(newContent, "\n")

	// Find where the new string starts in the new content
	newStartLine := -1
	firstNewLine := ""
	if len(newLines) > 0 {
		firstNewLine = newLines[0]
	}
	for i, line := range contentLines {
		if firstNewLine != "" && strings.Contains(line, firstNewLine) {
			newStartLine = i
			break
		}
	}
	if newStartLine == -1 {
		newStartLine = lineNum - 1
		if newStartLine < 0 {
			newStartLine = 0
		}
	}

	// Calculate context bounds
	start := newStartLine - contextBefore
	if start < 0 {
		start = 0
	}
	end := newStartLine + len(newLines) + contextAfter
	if end > len(contentLines) {
		end = len(contentLines)
	}

	var diff strings.Builder
	diff.WriteString(summary)
	diff.WriteString("\n")

	lineCount := 0

	// Write context before
	for i := start; i < newStartLine && lineCount < maxDiffLines; i++ {
		diff.WriteString(fmt.Sprintf("    %4d  %s\n", i+1, truncateLine(contentLines[i])))
		lineCount++
	}

	// Write removed lines (from old string)
	for i, line := range oldLines {
		if lineCount >= maxDiffLines {
			diff.WriteString(fmt.Sprintf("    ... %d more lines omitted\n", len(oldLines)-i))
			break
		}
		lineNo := newStartLine + i + 1
		diff.WriteString(fmt.Sprintf("  - %4d  %s\n", lineNo, truncateLine(line)))
		lineCount++
	}

	// Write added lines (from new string)
	for i, line := range newLines {
		if lineCount >= maxDiffLines {
			diff.WriteString(fmt.Sprintf("    ... %d more lines omitted\n", len(newLines)-i))
			break
		}
		lineNo := newStartLine + i + 1
		diff.WriteString(fmt.Sprintf("  + %4d  %s\n", lineNo, truncateLine(line)))
		lineCount++
	}

	// Write context after
	for i := newStartLine + len(newLines); i < end && lineCount < maxDiffLines; i++ {
		diff.WriteString(fmt.Sprintf("    %4d  %s\n", i+1, truncateLine(contentLines[i])))
		lineCount++
	}

	return strings.TrimRight(diff.String(), "\n")
}

// truncateLine limits line length for display
func truncateLine(line string) string {
	// Replace tabs with spaces for consistent display
	line = strings.ReplaceAll(line, "\t", "    ")

	// Check for binary/non-printable content
	for _, r := range line {
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return "[binary content]"
		}
	}

	if len(line) > maxDiffLineWidth {
		return line[:maxDiffLineWidth-3] + "..."
	}
	return line
}

func pluralize(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func truncateForError(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
