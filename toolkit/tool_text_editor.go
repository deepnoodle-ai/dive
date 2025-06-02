package toolkit

import (
	"context"
	"fmt"
	"strings"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
)

var (
	_ dive.TypedTool[*TextEditorToolInput] = &TextEditorTool{}
	_ llm.ToolConfiguration                = &TextEditorTool{}
)

const (
	SnippetLines = 5           // Number of context lines to show around edits
	MaxFileSize  = 1024 * 1024 // 1MB file size limit for truncation
)

// Command represents the available commands for the text editor tool
type Command string

const (
	CommandView       Command = "view"
	CommandCreate     Command = "create"
	CommandStrReplace Command = "str_replace"
	CommandInsert     Command = "insert"
)

// text_editor_20241022 - Claude 3.5 Sonnet
// text_editor_20250124 - Claude 3.7 Sonnet
// text_editor_20250429 - Claude 4 Opus and Sonnet

/* A tool definition must be added in the request that looks like this:
{
  "type": "text_editor_20250429",
  "name": "text_editor"
}
*/

// TextEditorToolInput represents the input parameters for the text editor tool
type TextEditorToolInput struct {
	Command    Command `json:"command"`
	Path       string  `json:"path"`
	FileText   *string `json:"file_text,omitempty"`
	ViewRange  []int   `json:"view_range,omitempty"`
	OldStr     *string `json:"old_str,omitempty"`
	NewStr     *string `json:"new_str,omitempty"`
	InsertLine *int    `json:"insert_line,omitempty"`
}

// TextEditorToolOptions are the options used to configure a TextEditorTool.
type TextEditorToolOptions struct {
	Type       string
	Name       string
	FileSystem FileSystem // Optional: for testing with mock filesystem
}

// NewTextEditorTool creates a new TextEditorTool with the given options.
func NewTextEditorTool(opts TextEditorToolOptions) *dive.TypedToolAdapter[*TextEditorToolInput] {
	if opts.Type == "" {
		opts.Type = "text_editor_20250429"
	}
	if opts.Name == "" {
		opts.Name = "str_replace_based_edit_tool"
	}
	if opts.FileSystem == nil {
		opts.FileSystem = &RealFileSystem{}
	}
	return dive.ToolAdapter(&TextEditorTool{
		typeString:  opts.Type,
		name:        opts.Name,
		fs:          opts.FileSystem,
		fileHistory: make(map[string][]string),
	})
}

// TextEditorTool implements Anthropic's file editor tool
type TextEditorTool struct {
	typeString  string
	name        string
	fs          FileSystem
	fileHistory map[string][]string // Track file edit history
}

func (t *TextEditorTool) Name() string {
	return t.name
}

func (t *TextEditorTool) Description() string {
	return "A filesystem editor tool that allows the agent to view, create, and edit files."
}

func (t *TextEditorTool) Schema() *schema.Schema {
	return &schema.Schema{
		Type: "object",
		Properties: map[string]*schema.Property{
			"command": {
				Type:        "string",
				Description: "The command to execute: view (read file/directory), create (create new file), str_replace (replace text), insert (insert text at line)",
				Enum:        []string{"view", "create", "str_replace", "insert"},
			},
			"path": {
				Type:        "string",
				Description: "Absolute path to the file or directory",
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
				Description: "Replacement string (for str_replace and insert commands)",
			},
			"insert_line": {
				Type:        "integer",
				Description: "Line number to insert at (for insert command)",
			},
		},
		Required: []string{"command", "path"},
	}
}

func (t *TextEditorTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{
		Title:           "File Editor",
		ReadOnlyHint:    false,
		DestructiveHint: true,
		IdempotentHint:  false,
		OpenWorldHint:   false,
	}
}

func (t *TextEditorTool) Call(ctx context.Context, input *TextEditorToolInput) (*dive.ToolResult, error) {
	if err := t.validatePath(input.Command, input.Path); err != nil {
		return dive.NewToolResultError(err.Error()), nil
	}
	switch input.Command {
	case CommandView:
		return t.handleView(input.Path, input.ViewRange)
	case CommandCreate:
		return t.handleCreate(input.Path, input.FileText)
	case CommandStrReplace:
		return t.handleStrReplace(input.Path, input.OldStr, input.NewStr)
	case CommandInsert:
		return t.handleInsert(input.Path, input.InsertLine, input.NewStr)
	default:
		return dive.NewToolResultError(fmt.Sprintf("Unrecognized command %s. The allowed commands are: view, create, str_replace, insert", input.Command)), nil
	}
}

func (t *TextEditorTool) validatePath(command Command, path string) error {
	// Check if it's an absolute path
	if !t.fs.IsAbs(path) {
		return fmt.Errorf("the path %s is not an absolute path, it should start with `/`", path)
	}

	// Check if path exists (except for create command)
	exists := t.fs.FileExists(path)
	if !exists && command != CommandCreate {
		return fmt.Errorf("the path %s does not exist. Please provide a valid path", path)
	}
	if exists && command == CommandCreate {
		return fmt.Errorf("file already exists at: %s. Cannot overwrite files using command `create`", path)
	}

	// Check if the path points to a directory
	if exists && t.fs.IsDir(path) && command != CommandView {
		return fmt.Errorf("the path %s is a directory and only the `view` command can be used on directories", path)
	}

	return nil
}

func (t *TextEditorTool) handleView(path string, viewRange []int) (*dive.ToolResult, error) {
	if t.fs.IsDir(path) {
		if len(viewRange) > 0 {
			return dive.NewToolResultError("the `view_range` parameter is not allowed when `path` points to a directory"), nil
		}

		output, err := t.fs.ListDir(path)
		if err != nil {
			return dive.NewToolResultError(fmt.Sprintf("error listing directory: %v", err)), nil
		}

		result := fmt.Sprintf("Here's the files and directories up to 2 levels deep in %s, excluding hidden items:\n%s", path, output)
		return dive.NewToolResultText(result), nil
	}

	// Handle file viewing
	content, err := t.fs.ReadFile(path)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("error reading file %s: %v", path, err)), nil
	}

	initLine := 1
	if len(viewRange) > 0 {
		if len(viewRange) != 2 {
			return dive.NewToolResultError("invalid `view_range`. It should be a list of two integers"), nil
		}

		lines := strings.Split(content, "\n")
		nLines := len(lines)
		start, end := viewRange[0], viewRange[1]

		if start < 1 || start > nLines {
			return dive.NewToolResultError(fmt.Sprintf("invalid `view_range`: [%d, %d]. First element should be within range [1, %d]", start, end, nLines)), nil
		}
		if end > nLines {
			return dive.NewToolResultError(fmt.Sprintf("invalid `view_range`: [%d, %d]. Second element should be <= %d", start, end, nLines)), nil
		}
		if end != -1 && end < start {
			return dive.NewToolResultError(fmt.Sprintf("invalid `view_range`: [%d, %d]. Second element should be >= first element", start, end)), nil
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

func (t *TextEditorTool) handleCreate(path string, fileText *string) (*dive.ToolResult, error) {
	if fileText == nil {
		return dive.NewToolResultError("parameter `file_text` is required for command: create"), nil
	}

	if err := t.fs.WriteFile(path, *fileText); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("error writing file %s: %v", path, err)), nil
	}

	// Don't add to history for create - it's a new file, not an edit

	return dive.NewToolResultText(fmt.Sprintf("File created successfully at: %s", path)), nil
}

func (t *TextEditorTool) handleStrReplace(path string, oldStr, newStr *string) (*dive.ToolResult, error) {
	if oldStr == nil {
		return dive.NewToolResultError("parameter `old_str` is required for command: str_replace"), nil
	}

	content, err := t.fs.ReadFile(path)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("error reading file %s: %v", path, err)), nil
	}

	// Expand tabs for consistent handling
	content = strings.ReplaceAll(content, "\t", "    ")
	oldStrExpanded := strings.ReplaceAll(*oldStr, "\t", "    ")
	newStrExpanded := ""
	if newStr != nil {
		newStrExpanded = strings.ReplaceAll(*newStr, "\t", "    ")
	}

	// Check if old_str is unique
	occurrences := strings.Count(content, oldStrExpanded)
	if occurrences == 0 {
		return dive.NewToolResultError(fmt.Sprintf("No replacement was performed, old_str `%s` did not appear verbatim in %s", *oldStr, path)), nil
	}
	if occurrences > 1 {
		lines := strings.Split(content, "\n")
		lineNumbers := []int{}
		for i, line := range lines {
			if strings.Contains(line, oldStrExpanded) {
				lineNumbers = append(lineNumbers, i+1)
			}
		}
		return dive.NewToolResultError(fmt.Sprintf("No replacement was performed. Multiple occurrences of old_str `%s` in lines %v. Please ensure it is unique", *oldStr, lineNumbers)), nil
	}

	// Save original content to history
	t.fileHistory[path] = append(t.fileHistory[path], content)

	// Perform replacement
	newContent := strings.Replace(content, oldStrExpanded, newStrExpanded, 1)

	if err := t.fs.WriteFile(path, newContent); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("error writing file %s: %v", path, err)), nil
	}

	// Generate snippet for feedback
	snippet := t.generateEditSnippet(content, newContent, oldStrExpanded, newStrExpanded)
	successMsg := fmt.Sprintf("The file %s has been edited. %s\nReview the changes and make sure they are as expected. Edit the file again if necessary.", path, snippet)

	return dive.NewToolResultText(successMsg), nil
}

func (t *TextEditorTool) handleInsert(path string, insertLine *int, newStr *string) (*dive.ToolResult, error) {
	if insertLine == nil {
		return dive.NewToolResultError("parameter `insert_line` is required for command: insert"), nil
	}
	if newStr == nil {
		return dive.NewToolResultError("parameter `new_str` is required for command: insert"), nil
	}

	content, err := t.fs.ReadFile(path)
	if err != nil {
		return dive.NewToolResultError(fmt.Sprintf("error reading file %s: %v", path, err)), nil
	}

	// Expand tabs
	content = strings.ReplaceAll(content, "\t", "    ")
	newStrExpanded := strings.ReplaceAll(*newStr, "\t", "    ")

	lines := strings.Split(content, "\n")
	nLines := len(lines)

	if *insertLine < 0 || *insertLine > nLines {
		return dive.NewToolResultError(fmt.Sprintf("invalid `insert_line` parameter: %d. It should be within the range [0, %d]", *insertLine, nLines)), nil
	}

	// Save original content to history
	t.fileHistory[path] = append(t.fileHistory[path], content)

	// Insert new content
	newStrLines := strings.Split(newStrExpanded, "\n")
	newLines := make([]string, 0, len(lines)+len(newStrLines))
	newLines = append(newLines, lines[:*insertLine]...)
	newLines = append(newLines, newStrLines...)
	newLines = append(newLines, lines[*insertLine:]...)

	newContent := strings.Join(newLines, "\n")

	if err := t.fs.WriteFile(path, newContent); err != nil {
		return dive.NewToolResultError(fmt.Sprintf("error writing file %s: %v", path, err)), nil
	}

	// Generate snippet for feedback
	snippetStart := max(0, *insertLine-SnippetLines)
	snippetEnd := min(len(newLines), *insertLine+len(newStrLines)+SnippetLines)
	snippet := strings.Join(newLines[snippetStart:snippetEnd], "\n")
	snippetOutput := t.makeOutput(snippet, "a snippet of the edited file", snippetStart+1)

	successMsg := fmt.Sprintf("The file %s has been edited. %s\nReview the changes and make sure they are as expected (correct indentation, no duplicate lines, etc). Edit the file again if necessary.", path, snippetOutput)

	return dive.NewToolResultText(successMsg), nil
}

func (t *TextEditorTool) generateEditSnippet(originalContent, newContent, oldStr, newStr string) string {
	// Find the line where replacement occurred
	beforeReplacement := strings.Split(originalContent, oldStr)[0]
	replacementLine := strings.Count(beforeReplacement, "\n")

	startLine := max(0, replacementLine-SnippetLines)
	endLine := replacementLine + SnippetLines + strings.Count(newStr, "\n")

	lines := strings.Split(newContent, "\n")
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}

	snippet := strings.Join(lines[startLine:endLine+1], "\n")
	return t.makeOutput(snippet, fmt.Sprintf("a snippet of %s", "the file"), startLine+1)
}

func (t *TextEditorTool) makeOutput(content, descriptor string, initLine int) string {
	content = t.maybeTruncate(content)
	content = strings.ReplaceAll(content, "\t", "    ") // Expand tabs

	lines := strings.Split(content, "\n")
	numberedLines := make([]string, len(lines))
	for i, line := range lines {
		numberedLines[i] = fmt.Sprintf("%6d\t%s", i+initLine, line)
	}

	numberedContent := strings.Join(numberedLines, "\n")
	return fmt.Sprintf("Here's the result of running `cat -n` on %s:\n%s\n", descriptor, numberedContent)
}

func (t *TextEditorTool) maybeTruncate(content string) string {
	if len(content) > MaxFileSize {
		return content[:MaxFileSize] + "\n... (content truncated)"
	}
	return content
}

func (t *TextEditorTool) ToolConfiguration(providerName string) map[string]any {
	if providerName == "anthropic" {
		return map[string]any{"type": t.typeString, "name": t.name}
	}
	return nil
}

// Helper functions
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
