package toolkit

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/deepnoodle-ai/wonton/assert"
)

// testPathValidator creates a permissive PathValidator for unit tests
// that allows access to any path starting with the given root (typically "/")
func testPathValidator(root string) *PathValidator {
	return &PathValidator{WorkspaceDir: root}
}

func TestTextEditorTool_View_File(t *testing.T) {
	fs := newMockFileSystem()
	fs.AddFile("/test/file.txt", "line 1\nline 2\nline 3\nline 4\nline 5")

	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	input := &TextEditorToolInput{
		Command: CommandView,
		Path:    "/test/file.txt",
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	output := result.Content[0].Text
	assert.Contains(t, output, "line 1", "Expected output to contain file content")
	assert.Contains(t, output, "line 5", "Expected output to contain file content")

	// Check line numbers are present
	assert.Contains(t, output, "     1\t", "Expected output to contain line numbers")
	assert.Contains(t, output, "     5\t", "Expected output to contain line numbers")
}

func TestTextEditorTool_View_FileWithRange(t *testing.T) {
	fs := newMockFileSystem()
	fs.AddFile("/test/file.txt", "line 1\nline 2\nline 3\nline 4\nline 5")

	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	input := &TextEditorToolInput{
		Command:   CommandView,
		Path:      "/test/file.txt",
		ViewRange: []int{2, 4},
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	output := result.Content[0].Text
	assert.Contains(t, output, "line 2", "Expected output to contain lines 2-4")
	assert.Contains(t, output, "line 4", "Expected output to contain lines 2-4")

	// Should not contain line 1 or 5
	assert.NotContains(t, output, "line 1", "Expected output to not contain line 1")
	assert.NotContains(t, output, "line 5", "Expected output to not contain line 5")

	// Check line numbers start from 2
	assert.Contains(t, output, "     2\t", "Expected line numbers to start from 2")
}

func TestTextEditorTool_View_Directory(t *testing.T) {
	fs := newMockFileSystem()
	fs.AddDirectory("/test")
	fs.AddFile("/test/file1.txt", "content1")
	fs.AddFile("/test/file2.txt", "content2")

	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	input := &TextEditorToolInput{
		Command: CommandView,
		Path:    "/test",
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	output := result.Content[0].Text
	assert.Contains(t, output, "file1.txt", "Expected directory listing to contain files")
	assert.Contains(t, output, "file2.txt", "Expected directory listing to contain files")
}

func TestTextEditorTool_Create(t *testing.T) {
	fs := newMockFileSystem()

	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	content := "Hello, World!\nThis is a test file."
	input := &TextEditorToolInput{
		Command:  CommandCreate,
		Path:     "/test/new_file.txt",
		FileText: &content,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	// Verify file was created
	assert.True(t, fs.FileExists("/test/new_file.txt"), "Expected file to be created")

	// Verify content
	actualContent, _ := fs.ReadFile("/test/new_file.txt")
	assert.Equal(t, content, actualContent, "Expected content to match")

	// Verify no history for create operation
	assert.Len(t, tool.fileHistory["/test/new_file.txt"], 0, "Expected 0 history entries for create operation")
}

func TestTextEditorTool_StrReplace(t *testing.T) {
	fs := newMockFileSystem()
	originalContent := "Hello, World!\nThis is a test.\nHello, Universe!"
	fs.AddFile("/test/file.txt", originalContent)

	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	oldStr := "This is a test."
	newStr := "This is a successful test."
	input := &TextEditorToolInput{
		Command: CommandStrReplace,
		Path:    "/test/file.txt",
		OldStr:  &oldStr,
		NewStr:  &newStr,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	// Verify file was updated
	newContent, _ := fs.ReadFile("/test/file.txt")
	expectedContent := "Hello, World!\nThis is a successful test.\nHello, Universe!"
	assert.Equal(t, expectedContent, newContent, "Expected content to match")

	// Verify history contains original content
	assert.Len(t, tool.fileHistory["/test/file.txt"], 1, "Expected 1 history entry")
	assert.Equal(t, originalContent, tool.fileHistory["/test/file.txt"][0], "Expected history to contain original content")
}

func TestTextEditorTool_StrReplace_MultipleOccurrences(t *testing.T) {
	fs := newMockFileSystem()
	fs.AddFile("/test/file.txt", "Hello, World!\nHello, World!\nGoodbye!")

	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	oldStr := "Hello, World!"
	newStr := "Hi there!"
	input := &TextEditorToolInput{
		Command: CommandStrReplace,
		Path:    "/test/file.txt",
		OldStr:  &oldStr,
		NewStr:  &newStr,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.True(t, result.IsError, "Expected error for multiple occurrences")
	assert.Contains(t, result.Content[0].Text, "Multiple occurrences", "Expected error message about multiple occurrences")
}

func TestTextEditorTool_Insert(t *testing.T) {
	fs := newMockFileSystem()
	originalContent := "line 1\nline 2\nline 4"
	fs.AddFile("/test/file.txt", originalContent)

	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	insertLine := 2
	newStr := "line 3"
	input := &TextEditorToolInput{
		Command:    CommandInsert,
		Path:       "/test/file.txt",
		InsertLine: &insertLine,
		NewStr:     &newStr,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	// Verify file was updated
	newContent, _ := fs.ReadFile("/test/file.txt")
	expectedContent := "line 1\nline 2\nline 3\nline 4"
	assert.Equal(t, expectedContent, newContent, "Expected content to match")

	// Verify history
	assert.Len(t, tool.fileHistory["/test/file.txt"], 1, "Expected 1 history entry")
}

func TestTextEditorTool_ValidationErrors(t *testing.T) {
	fs := newMockFileSystem()
	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	tests := []struct {
		name        string
		input       *TextEditorToolInput
		expectError bool
		errorMsg    string
	}{
		{
			name: "non-absolute path",
			input: &TextEditorToolInput{
				Command: CommandView,
				Path:    "relative/path",
			},
			expectError: true,
			errorMsg:    "not an absolute path",
		},
		{
			name: "non-existent file for view",
			input: &TextEditorToolInput{
				Command: CommandView,
				Path:    "/non/existent/file",
			},
			expectError: true,
			errorMsg:    "does not exist",
		},
		{
			name: "create existing file",
			input: func() *TextEditorToolInput {
				fs.AddFile("/existing/file", "content")
				content := "new content"
				return &TextEditorToolInput{
					Command:  CommandCreate,
					Path:     "/existing/file",
					FileText: &content,
				}
			}(),
			expectError: true,
			errorMsg:    "already exists",
		},
		{
			name: "create without file_text",
			input: &TextEditorToolInput{
				Command: CommandCreate,
				Path:    "/new/file",
			},
			expectError: true,
			errorMsg:    "file_text` is required",
		},
		{
			name: "str_replace without old_str",
			input: &TextEditorToolInput{
				Command: CommandStrReplace,
				Path:    "/test/file",
			},
			expectError: true,
			errorMsg:    "old_str` is required",
		},
		{
			name: "insert without insert_line",
			input: func() *TextEditorToolInput {
				fs.AddFile("/test/file", "content")
				newStr := "new line"
				return &TextEditorToolInput{
					Command: CommandInsert,
					Path:    "/test/file",
					NewStr:  &newStr,
				}
			}(),
			expectError: true,
			errorMsg:    "insert_line` is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Call(context.Background(), tt.input)

			assert.NoError(t, err)

			if tt.expectError {
				assert.True(t, result.IsError, "Expected error, got success")
				assert.Contains(t, result.Content[0].Text, tt.errorMsg, "Expected error message to contain %q", tt.errorMsg)
			} else {
				assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)
			}
		})
	}
}

func TestTextEditorTool_Schema(t *testing.T) {
	tool := &TextEditorTool{}
	sch := tool.Schema()

	// Verify schema structure
	assert.Equal(t, schema.Object, sch.Type, "Expected schema type to be 'object'")

	// Verify required fields
	expectedRequired := []string{"command", "path"}
	assert.Len(t, sch.Required, len(expectedRequired), "Expected %d required fields", len(expectedRequired))

	for _, field := range expectedRequired {
		assert.Contains(t, sch.Required, field, "Expected required field %s to be present", field)
	}

	// Verify command enum values
	commandProp := sch.Properties["command"]
	assert.NotNil(t, commandProp, "Expected command property in schema")

	expectedCommands := []string{"view", "create", "str_replace", "insert"}
	assert.Len(t, commandProp.Enum, len(expectedCommands), "Expected %d command enum values", len(expectedCommands))
}

func TestTextEditorTool_Integration(t *testing.T) {
	// Test a complete workflow: create file, view it, edit it, view again
	fs := newMockFileSystem()
	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	// 1. Create a file
	content := "# README\n\nThis is a test file.\n\n## Features\n- Feature 1\n- Feature 2"
	createInput := &TextEditorToolInput{
		Command:  CommandCreate,
		Path:     "/project/README.md",
		FileText: &content,
	}

	result, err := tool.Call(context.Background(), createInput)
	assert.NoError(t, err)
	assert.False(t, result.IsError, "Create operation should succeed")

	// 2. View the file
	viewInput := &TextEditorToolInput{
		Command: CommandView,
		Path:    "/project/README.md",
	}

	result, err = tool.Call(context.Background(), viewInput)
	assert.NoError(t, err)
	assert.False(t, result.IsError, "View operation should succeed")

	// 3. Replace some content
	oldStr := "This is a test file."
	newStr := "This is a comprehensive documentation file."
	replaceInput := &TextEditorToolInput{
		Command: CommandStrReplace,
		Path:    "/project/README.md",
		OldStr:  &oldStr,
		NewStr:  &newStr,
	}

	result, err = tool.Call(context.Background(), replaceInput)
	assert.NoError(t, err)
	assert.False(t, result.IsError, "Replace operation should succeed")

	// 4. Insert a new feature
	insertLine := 6
	newFeature := "- Feature 3"
	insertInput := &TextEditorToolInput{
		Command:    CommandInsert,
		Path:       "/project/README.md",
		InsertLine: &insertLine,
		NewStr:     &newFeature,
	}

	result, err = tool.Call(context.Background(), insertInput)
	assert.NoError(t, err)
	assert.False(t, result.IsError, "Insert operation should succeed")

	// 5. Verify final content
	finalContent, _ := fs.ReadFile("/project/README.md")
	expectedFinal := "# README\n\nThis is a comprehensive documentation file.\n\n## Features\n- Feature 1\n- Feature 3\n- Feature 2"

	assert.Equal(t, expectedFinal, finalContent, "Final content should match expected content")

	// 6. Verify history tracking
	assert.Len(t, tool.fileHistory["/project/README.md"], 2, "Expected 2 history entries")
}

func TestNewTextEditorTool(t *testing.T) {
	// Test with default options
	adapter1 := NewTextEditorTool(TextEditorToolOptions{})
	assert.NotNil(t, adapter1, "Expected tool adapter to be created")

	// Test with custom options
	mockFS := newMockFileSystem()
	adapter2 := NewTextEditorTool(TextEditorToolOptions{
		Type:       "custom_editor",
		FileSystem: mockFS,
	})
	assert.NotNil(t, adapter2, "Expected tool adapter to be created with custom options")
}

func TestTextEditorTool_PathValidation(t *testing.T) {
	// Create a temporary workspace directory
	workspaceDir := t.TempDir()

	// Create a path validator for the workspace
	pathValidator, err := NewPathValidator(workspaceDir)
	assert.NoError(t, err)

	fs := newMockFileSystem()
	// Add a file inside workspace
	fs.AddFile(workspaceDir+"/allowed.txt", "allowed content")
	// Add a file outside workspace
	fs.AddFile("/etc/passwd", "root:x:0:0:root:/root:/bin/bash")

	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: pathValidator,
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	tests := []struct {
		name        string
		input       *TextEditorToolInput
		expectError bool
		errorMsg    string
	}{
		{
			name: "view file inside workspace - allowed",
			input: &TextEditorToolInput{
				Command: CommandView,
				Path:    workspaceDir + "/allowed.txt",
			},
			expectError: false,
		},
		{
			name: "view file outside workspace - denied",
			input: &TextEditorToolInput{
				Command: CommandView,
				Path:    "/etc/passwd",
			},
			expectError: true,
			errorMsg:    "outside workspace",
		},
		{
			name: "create file outside workspace - denied",
			input: func() *TextEditorToolInput {
				content := "malicious content"
				return &TextEditorToolInput{
					Command:  CommandCreate,
					Path:     "/tmp/evil.txt",
					FileText: &content,
				}
			}(),
			expectError: true,
			errorMsg:    "outside workspace",
		},
		{
			name: "str_replace outside workspace - denied",
			input: func() *TextEditorToolInput {
				oldStr := "root"
				newStr := "hacked"
				return &TextEditorToolInput{
					Command: CommandStrReplace,
					Path:    "/etc/passwd",
					OldStr:  &oldStr,
					NewStr:  &newStr,
				}
			}(),
			expectError: true,
			errorMsg:    "outside workspace",
		},
		{
			name: "insert outside workspace - denied",
			input: func() *TextEditorToolInput {
				line := 0
				newStr := "malicious line"
				return &TextEditorToolInput{
					Command:    CommandInsert,
					Path:       "/etc/passwd",
					InsertLine: &line,
					NewStr:     &newStr,
				}
			}(),
			expectError: true,
			errorMsg:    "outside workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Call(context.Background(), tt.input)
			assert.NoError(t, err)

			if tt.expectError {
				assert.True(t, result.IsError, "Expected error for path outside workspace")
				assert.Contains(t, result.Content[0].Text, tt.errorMsg)
			} else {
				assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)
			}
		})
	}
}

func TestTextEditorTool_TabPreservation(t *testing.T) {
	// Test that tabs are NOT converted to spaces (fixes Makefile corruption)
	fs := newMockFileSystem()

	// Makefile content with tabs (required for make)
	makefileContent := "all:\n\techo \"Building...\"\n\tgcc -o main main.c\n\nclean:\n\trm -f main"
	fs.AddFile("/project/Makefile", makefileContent)

	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	// Test str_replace preserves tabs
	oldStr := "echo \"Building...\""
	newStr := "echo \"Compiling...\""
	input := &TextEditorToolInput{
		Command: CommandStrReplace,
		Path:    "/project/Makefile",
		OldStr:  &oldStr,
		NewStr:  &newStr,
	}

	result, err := tool.Call(context.Background(), input)
	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	// Verify tabs are preserved
	newContent, _ := fs.ReadFile("/project/Makefile")
	assert.Contains(t, newContent, "\t", "Tabs should be preserved in Makefile")
	assert.Contains(t, newContent, "\techo \"Compiling...\"", "Tab before echo should be preserved")
	assert.Contains(t, newContent, "\tgcc", "Tab before gcc should be preserved")
	assert.Contains(t, newContent, "\trm", "Tab before rm should be preserved")
}

func TestTextEditorTool_InsertTabPreservation(t *testing.T) {
	// Test that insert command preserves tabs
	fs := newMockFileSystem()
	fs.AddFile("/project/Makefile", "all:\n\techo \"hello\"")

	tool := &TextEditorTool{
		fs:            fs,
		pathValidator: testPathValidator("/"),
		maxFileSize:   MaxFileSize,
		fileHistory:   make(map[string][]string),
	}

	// Insert a new line with a tab
	insertLine := 2
	newStr := "\techo \"world\""
	input := &TextEditorToolInput{
		Command:    CommandInsert,
		Path:       "/project/Makefile",
		InsertLine: &insertLine,
		NewStr:     &newStr,
	}

	result, err := tool.Call(context.Background(), input)
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	// Verify tabs are preserved
	newContent, _ := fs.ReadFile("/project/Makefile")
	assert.Contains(t, newContent, "\techo \"hello\"", "Original tab should be preserved")
	assert.Contains(t, newContent, "\techo \"world\"", "Inserted tab should be preserved")
}
