package toolkit

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/schema"
	"github.com/stretchr/testify/require"
)

func TestTextEditorTool_View_File(t *testing.T) {
	fs := newMockFileSystem()
	fs.AddFile("/test/file.txt", "line 1\nline 2\nline 3\nline 4\nline 5")

	tool := &TextEditorTool{
		fs:          fs,
		fileHistory: make(map[string][]string),
	}

	input := &TextEditorToolInput{
		Command: CommandView,
		Path:    "/test/file.txt",
	}

	result, err := tool.Call(context.Background(), input)

	require.NoError(t, err)
	require.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	output := result.Content[0].Text
	require.Contains(t, output, "line 1", "Expected output to contain file content")
	require.Contains(t, output, "line 5", "Expected output to contain file content")

	// Check line numbers are present
	require.Contains(t, output, "     1\t", "Expected output to contain line numbers")
	require.Contains(t, output, "     5\t", "Expected output to contain line numbers")
}

func TestTextEditorTool_View_FileWithRange(t *testing.T) {
	fs := newMockFileSystem()
	fs.AddFile("/test/file.txt", "line 1\nline 2\nline 3\nline 4\nline 5")

	tool := &TextEditorTool{
		fs:          fs,
		fileHistory: make(map[string][]string),
	}

	input := &TextEditorToolInput{
		Command:   CommandView,
		Path:      "/test/file.txt",
		ViewRange: []int{2, 4},
	}

	result, err := tool.Call(context.Background(), input)

	require.NoError(t, err)
	require.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	output := result.Content[0].Text
	require.Contains(t, output, "line 2", "Expected output to contain lines 2-4")
	require.Contains(t, output, "line 4", "Expected output to contain lines 2-4")

	// Should not contain line 1 or 5
	require.NotContains(t, output, "line 1", "Expected output to not contain line 1")
	require.NotContains(t, output, "line 5", "Expected output to not contain line 5")

	// Check line numbers start from 2
	require.Contains(t, output, "     2\t", "Expected line numbers to start from 2")
}

func TestTextEditorTool_View_Directory(t *testing.T) {
	fs := newMockFileSystem()
	fs.AddDirectory("/test")
	fs.AddFile("/test/file1.txt", "content1")
	fs.AddFile("/test/file2.txt", "content2")

	tool := &TextEditorTool{
		fs:          fs,
		fileHistory: make(map[string][]string),
	}

	input := &TextEditorToolInput{
		Command: CommandView,
		Path:    "/test",
	}

	result, err := tool.Call(context.Background(), input)

	require.NoError(t, err)
	require.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	output := result.Content[0].Text
	require.Contains(t, output, "file1.txt", "Expected directory listing to contain files")
	require.Contains(t, output, "file2.txt", "Expected directory listing to contain files")
}

func TestTextEditorTool_Create(t *testing.T) {
	fs := newMockFileSystem()

	tool := &TextEditorTool{
		fs:          fs,
		fileHistory: make(map[string][]string),
	}

	content := "Hello, World!\nThis is a test file."
	input := &TextEditorToolInput{
		Command:  CommandCreate,
		Path:     "/test/new_file.txt",
		FileText: &content,
	}

	result, err := tool.Call(context.Background(), input)

	require.NoError(t, err)
	require.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	// Verify file was created
	require.True(t, fs.FileExists("/test/new_file.txt"), "Expected file to be created")

	// Verify content
	actualContent, _ := fs.ReadFile("/test/new_file.txt")
	require.Equal(t, content, actualContent, "Expected content to match")

	// Verify no history for create operation
	require.Len(t, tool.fileHistory["/test/new_file.txt"], 0, "Expected 0 history entries for create operation")
}

func TestTextEditorTool_StrReplace(t *testing.T) {
	fs := newMockFileSystem()
	originalContent := "Hello, World!\nThis is a test.\nHello, Universe!"
	fs.AddFile("/test/file.txt", originalContent)

	tool := &TextEditorTool{
		fs:          fs,
		fileHistory: make(map[string][]string),
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

	require.NoError(t, err)
	require.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	// Verify file was updated
	newContent, _ := fs.ReadFile("/test/file.txt")
	expectedContent := "Hello, World!\nThis is a successful test.\nHello, Universe!"
	require.Equal(t, expectedContent, newContent, "Expected content to match")

	// Verify history contains original content
	require.Len(t, tool.fileHistory["/test/file.txt"], 1, "Expected 1 history entry")
	require.Equal(t, originalContent, tool.fileHistory["/test/file.txt"][0], "Expected history to contain original content")
}

func TestTextEditorTool_StrReplace_MultipleOccurrences(t *testing.T) {
	fs := newMockFileSystem()
	fs.AddFile("/test/file.txt", "Hello, World!\nHello, World!\nGoodbye!")

	tool := &TextEditorTool{
		fs:          fs,
		fileHistory: make(map[string][]string),
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

	require.NoError(t, err)
	require.True(t, result.IsError, "Expected error for multiple occurrences")
	require.Contains(t, result.Content[0].Text, "Multiple occurrences", "Expected error message about multiple occurrences")
}

func TestTextEditorTool_Insert(t *testing.T) {
	fs := newMockFileSystem()
	originalContent := "line 1\nline 2\nline 4"
	fs.AddFile("/test/file.txt", originalContent)

	tool := &TextEditorTool{
		fs:          fs,
		fileHistory: make(map[string][]string),
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

	require.NoError(t, err)
	require.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	// Verify file was updated
	newContent, _ := fs.ReadFile("/test/file.txt")
	expectedContent := "line 1\nline 2\nline 3\nline 4"
	require.Equal(t, expectedContent, newContent, "Expected content to match")

	// Verify history
	require.Len(t, tool.fileHistory["/test/file.txt"], 1, "Expected 1 history entry")
}

func TestTextEditorTool_ValidationErrors(t *testing.T) {
	fs := newMockFileSystem()
	tool := &TextEditorTool{
		fs:          fs,
		fileHistory: make(map[string][]string),
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

			require.NoError(t, err)

			if tt.expectError {
				require.True(t, result.IsError, "Expected error, got success")
				require.Contains(t, result.Content[0].Text, tt.errorMsg, "Expected error message to contain %q", tt.errorMsg)
			} else {
				require.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)
			}
		})
	}
}

func TestTextEditorTool_Schema(t *testing.T) {
	tool := &TextEditorTool{}
	sch := tool.Schema()

	// Verify schema structure
	require.Equal(t, schema.Object, sch.Type, "Expected schema type to be 'object'")

	// Verify required fields
	expectedRequired := []string{"command", "path"}
	require.Len(t, sch.Required, len(expectedRequired), "Expected %d required fields", len(expectedRequired))

	for _, field := range expectedRequired {
		require.Contains(t, sch.Required, field, "Expected required field %s to be present", field)
	}

	// Verify command enum values
	commandProp := sch.Properties["command"]
	require.NotNil(t, commandProp, "Expected command property in schema")

	expectedCommands := []string{"view", "create", "str_replace", "insert"}
	require.Len(t, commandProp.Enum, len(expectedCommands), "Expected %d command enum values", len(expectedCommands))
}

func TestTextEditorTool_Integration(t *testing.T) {
	// Test a complete workflow: create file, view it, edit it, view again
	fs := newMockFileSystem()
	tool := &TextEditorTool{
		fs:          fs,
		fileHistory: make(map[string][]string),
	}

	// 1. Create a file
	content := "# README\n\nThis is a test file.\n\n## Features\n- Feature 1\n- Feature 2"
	createInput := &TextEditorToolInput{
		Command:  CommandCreate,
		Path:     "/project/README.md",
		FileText: &content,
	}

	result, err := tool.Call(context.Background(), createInput)
	require.NoError(t, err)
	require.False(t, result.IsError, "Create operation should succeed")

	// 2. View the file
	viewInput := &TextEditorToolInput{
		Command: CommandView,
		Path:    "/project/README.md",
	}

	result, err = tool.Call(context.Background(), viewInput)
	require.NoError(t, err)
	require.False(t, result.IsError, "View operation should succeed")

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
	require.NoError(t, err)
	require.False(t, result.IsError, "Replace operation should succeed")

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
	require.NoError(t, err)
	require.False(t, result.IsError, "Insert operation should succeed")

	// 5. Verify final content
	finalContent, _ := fs.ReadFile("/project/README.md")
	expectedFinal := "# README\n\nThis is a comprehensive documentation file.\n\n## Features\n- Feature 1\n- Feature 3\n- Feature 2"

	require.Equal(t, expectedFinal, finalContent, "Final content should match expected content")

	// 6. Verify history tracking
	require.Len(t, tool.fileHistory["/project/README.md"], 2, "Expected 2 history entries")
}

func TestNewTextEditorTool(t *testing.T) {
	// Test with default options
	adapter1 := NewTextEditorTool(TextEditorToolOptions{})
	require.NotNil(t, adapter1, "Expected tool adapter to be created")

	// Test with custom options
	mockFS := newMockFileSystem()
	adapter2 := NewTextEditorTool(TextEditorToolOptions{
		Type:       "custom_editor",
		FileSystem: mockFS,
	})
	require.NotNil(t, adapter2, "Expected tool adapter to be created with custom options")
}
