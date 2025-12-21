package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func TestMemoryTool_View_File(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/test.txt", "line 1\nline 2\nline 3\nline 4\nline 5")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	input := &MemoryToolInput{
		Command: MemoryCommandView,
		Path:    "/memories/test.txt",
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	output := result.Content[0].Text
	assert.Contains(t, output, "line 1")
	assert.Contains(t, output, "line 5")
	assert.Contains(t, output, "     1\t") // Line numbers present
	assert.Contains(t, output, "     5\t")
}

func TestMemoryTool_View_FileWithRange(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/test.txt", "line 1\nline 2\nline 3\nline 4\nline 5")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	input := &MemoryToolInput{
		Command:   MemoryCommandView,
		Path:      "/memories/test.txt",
		ViewRange: []int{2, 4},
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	output := result.Content[0].Text
	assert.Contains(t, output, "line 2")
	assert.Contains(t, output, "line 4")
	assert.NotContains(t, output, "line 1")
	assert.NotContains(t, output, "line 5")
	assert.Contains(t, output, "     2\t") // Line numbers start from 2
}

func TestMemoryTool_View_Directory(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/notes/file1.txt", "content1")
	storage.WriteFile("/memories/notes/file2.txt", "content2")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	input := &MemoryToolInput{
		Command: MemoryCommandView,
		Path:    "/memories",
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	output := result.Content[0].Text
	assert.Contains(t, output, "up to 2 levels deep")
}

func TestMemoryTool_View_NonExistentPath(t *testing.T) {
	storage := NewInMemoryStorage()

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	input := &MemoryToolInput{
		Command: MemoryCommandView,
		Path:    "/memories/nonexistent.txt",
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "does not exist")
}

func TestMemoryTool_Create(t *testing.T) {
	storage := NewInMemoryStorage()

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	content := "Hello, World!\nThis is a test file."
	input := &MemoryToolInput{
		Command:  MemoryCommandCreate,
		Path:     "/memories/new_file.txt",
		FileText: &content,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)
	assert.Contains(t, result.Content[0].Text, "File created successfully")

	// Verify file was created
	assert.True(t, storage.Exists("/memories/new_file.txt"))

	actualContent, _ := storage.ReadFile("/memories/new_file.txt")
	assert.Equal(t, content, actualContent)
}

func TestMemoryTool_Create_AlreadyExists(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/existing.txt", "existing content")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	content := "new content"
	input := &MemoryToolInput{
		Command:  MemoryCommandCreate,
		Path:     "/memories/existing.txt",
		FileText: &content,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "already exists")
}

func TestMemoryTool_Create_MissingFileText(t *testing.T) {
	storage := NewInMemoryStorage()

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	input := &MemoryToolInput{
		Command: MemoryCommandCreate,
		Path:    "/memories/new_file.txt",
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "file_text is required")
}

func TestMemoryTool_StrReplace(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/test.txt", "Hello, World!\nThis is a test.\nGoodbye!")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	oldStr := "This is a test."
	newStr := "This is a successful test."
	input := &MemoryToolInput{
		Command: MemoryCommandStrReplace,
		Path:    "/memories/test.txt",
		OldStr:  &oldStr,
		NewStr:  &newStr,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)
	assert.Contains(t, result.Content[0].Text, "memory file has been edited")

	newContent, _ := storage.ReadFile("/memories/test.txt")
	assert.Contains(t, newContent, "This is a successful test.")
	assert.NotContains(t, newContent, "This is a test.")
}

func TestMemoryTool_StrReplace_MultipleOccurrences(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/test.txt", "Hello, World!\nHello, World!\nGoodbye!")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	oldStr := "Hello, World!"
	newStr := "Hi there!"
	input := &MemoryToolInput{
		Command: MemoryCommandStrReplace,
		Path:    "/memories/test.txt",
		OldStr:  &oldStr,
		NewStr:  &newStr,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "Multiple occurrences")
}

func TestMemoryTool_StrReplace_NotFound(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/test.txt", "Hello, World!")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	oldStr := "nonexistent text"
	newStr := "replacement"
	input := &MemoryToolInput{
		Command: MemoryCommandStrReplace,
		Path:    "/memories/test.txt",
		OldStr:  &oldStr,
		NewStr:  &newStr,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "did not appear verbatim")
}

func TestMemoryTool_Insert(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/test.txt", "line 1\nline 2\nline 4")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	insertLine := 2
	insertText := "line 3"
	input := &MemoryToolInput{
		Command:    MemoryCommandInsert,
		Path:       "/memories/test.txt",
		InsertLine: &insertLine,
		InsertText: &insertText,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)
	assert.Contains(t, result.Content[0].Text, "has been edited")

	newContent, _ := storage.ReadFile("/memories/test.txt")
	assert.Equal(t, "line 1\nline 2\nline 3\nline 4", newContent)
}

func TestMemoryTool_Insert_AtBeginning(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/test.txt", "line 2\nline 3")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	insertLine := 0
	insertText := "line 1"
	input := &MemoryToolInput{
		Command:    MemoryCommandInsert,
		Path:       "/memories/test.txt",
		InsertLine: &insertLine,
		InsertText: &insertText,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError)

	newContent, _ := storage.ReadFile("/memories/test.txt")
	assert.Equal(t, "line 1\nline 2\nline 3", newContent)
}

func TestMemoryTool_Insert_InvalidLine(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/test.txt", "line 1\nline 2")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	insertLine := 10 // Beyond file length
	insertText := "new line"
	input := &MemoryToolInput{
		Command:    MemoryCommandInsert,
		Path:       "/memories/test.txt",
		InsertLine: &insertLine,
		InsertText: &insertText,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "Invalid `insert_line`")
}

func TestMemoryTool_Delete_File(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/test.txt", "content")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	input := &MemoryToolInput{
		Command: MemoryCommandDelete,
		Path:    "/memories/test.txt",
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)
	assert.Contains(t, result.Content[0].Text, "Successfully deleted")

	assert.False(t, storage.Exists("/memories/test.txt"))
}

func TestMemoryTool_Delete_Directory(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/subdir/file1.txt", "content1")
	storage.WriteFile("/memories/subdir/file2.txt", "content2")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	input := &MemoryToolInput{
		Command: MemoryCommandDelete,
		Path:    "/memories/subdir",
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)
	assert.Contains(t, result.Content[0].Text, "Successfully deleted")

	assert.False(t, storage.Exists("/memories/subdir"))
	assert.False(t, storage.Exists("/memories/subdir/file1.txt"))
	assert.False(t, storage.Exists("/memories/subdir/file2.txt"))
}

func TestMemoryTool_Delete_NonExistent(t *testing.T) {
	storage := NewInMemoryStorage()

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	input := &MemoryToolInput{
		Command: MemoryCommandDelete,
		Path:    "/memories/nonexistent.txt",
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "does not exist")
}

func TestMemoryTool_Rename_File(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/old.txt", "content")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	oldPath := "/memories/old.txt"
	newPath := "/memories/new.txt"
	input := &MemoryToolInput{
		Command: MemoryCommandRename,
		OldPath: &oldPath,
		NewPath: &newPath,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)
	assert.Contains(t, result.Content[0].Text, "Successfully renamed")

	assert.False(t, storage.Exists("/memories/old.txt"))
	assert.True(t, storage.Exists("/memories/new.txt"))

	content, _ := storage.ReadFile("/memories/new.txt")
	assert.Equal(t, "content", content)
}

func TestMemoryTool_Rename_Directory(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/olddir/file.txt", "content")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	oldPath := "/memories/olddir"
	newPath := "/memories/newdir"
	input := &MemoryToolInput{
		Command: MemoryCommandRename,
		OldPath: &oldPath,
		NewPath: &newPath,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.False(t, result.IsError, "Expected success, got error: %s", result.Content[0].Text)

	assert.False(t, storage.Exists("/memories/olddir"))
	assert.True(t, storage.Exists("/memories/newdir"))
	assert.True(t, storage.Exists("/memories/newdir/file.txt"))
}

func TestMemoryTool_Rename_DestinationExists(t *testing.T) {
	storage := NewInMemoryStorage()
	storage.WriteFile("/memories/old.txt", "old content")
	storage.WriteFile("/memories/new.txt", "new content")

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	oldPath := "/memories/old.txt"
	newPath := "/memories/new.txt"
	input := &MemoryToolInput{
		Command: MemoryCommandRename,
		OldPath: &oldPath,
		NewPath: &newPath,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "already exists")
}

func TestMemoryTool_Rename_SourceNotFound(t *testing.T) {
	storage := NewInMemoryStorage()

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	oldPath := "/memories/nonexistent.txt"
	newPath := "/memories/new.txt"
	input := &MemoryToolInput{
		Command: MemoryCommandRename,
		OldPath: &oldPath,
		NewPath: &newPath,
	}

	result, err := tool.Call(context.Background(), input)

	assert.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "does not exist")
}

func TestMemoryTool_PathTraversal_Prevention(t *testing.T) {
	storage := NewInMemoryStorage()

	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid path", "/memories/test.txt", false},
		{"valid nested path", "/memories/subdir/test.txt", false},
		{"path traversal with ..", "/memories/../etc/passwd", true},
		{"path traversal encoded", "/memories/%2e%2e/etc/passwd", true},
		{"outside memories dir", "/etc/passwd", true},
		{"relative path", "memories/test.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.validatePath(tt.path)
			if tt.wantErr {
				assert.Error(t, err, "Expected path validation to fail for %s", tt.path)
			} else {
				assert.NoError(t, err, "Expected path validation to succeed for %s", tt.path)
			}
		})
	}
}

func TestMemoryTool_Schema(t *testing.T) {
	tool := &MemoryTool{}
	sch := tool.Schema()

	assert.Equal(t, schema.Object, sch.Type)

	// Verify required fields
	assert.Contains(t, sch.Required, "command")

	// Verify command enum values
	commandProp := sch.Properties["command"]
	assert.NotNil(t, commandProp)

	expectedCommands := []string{"view", "create", "str_replace", "insert", "delete", "rename"}
	assert.Len(t, commandProp.Enum, len(expectedCommands))
}

func TestMemoryTool_ToolConfiguration(t *testing.T) {
	tool := &MemoryTool{
		typeString: "memory_20250818",
		name:       "memory",
	}

	// Test Anthropic configuration
	config := tool.ToolConfiguration("anthropic")
	assert.NotNil(t, config)
	assert.Equal(t, "memory_20250818", config["type"])
	assert.Equal(t, "memory", config["name"])

	// Test other provider returns nil
	config = tool.ToolConfiguration("openai")
	assert.Nil(t, config)
}

func TestNewMemoryTool(t *testing.T) {
	// Test with default options
	adapter := NewMemoryTool()
	assert.NotNil(t, adapter)
	assert.Equal(t, "memory", adapter.Name())

	// Test with custom options
	customStorage := NewInMemoryStorage()
	adapter = NewMemoryTool(MemoryToolOptions{
		Type:      "custom_memory",
		Name:      "custom",
		Storage:   customStorage,
		MemoryDir: "/custom_memories",
	})
	assert.NotNil(t, adapter)
	assert.Equal(t, "custom", adapter.Name())
}

func TestInMemoryStorage(t *testing.T) {
	storage := NewInMemoryStorage()

	// Test that /memories directory exists by default
	assert.True(t, storage.Exists("/memories"))
	assert.True(t, storage.IsDir("/memories"))

	// Test WriteFile creates parent directories
	err := storage.WriteFile("/memories/a/b/c/file.txt", "content")
	assert.NoError(t, err)
	assert.True(t, storage.Exists("/memories/a"))
	assert.True(t, storage.Exists("/memories/a/b"))
	assert.True(t, storage.Exists("/memories/a/b/c"))
	assert.True(t, storage.Exists("/memories/a/b/c/file.txt"))

	// Test ReadFile
	content, err := storage.ReadFile("/memories/a/b/c/file.txt")
	assert.NoError(t, err)
	assert.Equal(t, "content", content)

	// Test FileSize
	size, err := storage.FileSize("/memories/a/b/c/file.txt")
	assert.NoError(t, err)
	assert.Equal(t, int64(7), size) // "content" is 7 bytes

	// Test file not found
	_, err = storage.ReadFile("/memories/nonexistent.txt")
	assert.Error(t, err)
}

func TestFileSystemStorage(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	storage, err := NewFileSystemStorage(tmpDir)
	assert.NoError(t, err)

	// Test WriteFile
	err = storage.WriteFile("/memories/test.txt", "hello world")
	assert.NoError(t, err)

	// Verify file exists on disk
	_, err = os.Stat(filepath.Join(tmpDir, "test.txt"))
	assert.NoError(t, err)

	// Test ReadFile
	content, err := storage.ReadFile("/memories/test.txt")
	assert.NoError(t, err)
	assert.Equal(t, "hello world", content)

	// Test Exists
	assert.True(t, storage.Exists("/memories/test.txt"))
	assert.False(t, storage.Exists("/memories/nonexistent.txt"))

	// Test IsDir
	assert.False(t, storage.IsDir("/memories/test.txt"))

	// Create a subdirectory
	err = storage.WriteFile("/memories/subdir/nested.txt", "nested content")
	assert.NoError(t, err)
	assert.True(t, storage.IsDir("/memories/subdir"))

	// Test FileSize
	size, err := storage.FileSize("/memories/test.txt")
	assert.NoError(t, err)
	assert.Equal(t, int64(11), size) // "hello world" is 11 bytes

	// Test Rename
	err = storage.Rename("/memories/test.txt", "/memories/renamed.txt")
	assert.NoError(t, err)
	assert.False(t, storage.Exists("/memories/test.txt"))
	assert.True(t, storage.Exists("/memories/renamed.txt"))

	// Test DeleteFile
	err = storage.DeleteFile("/memories/renamed.txt")
	assert.NoError(t, err)
	assert.False(t, storage.Exists("/memories/renamed.txt"))

	// Test DeleteDir
	err = storage.DeleteDir("/memories/subdir")
	assert.NoError(t, err)
	assert.False(t, storage.Exists("/memories/subdir"))
	assert.False(t, storage.Exists("/memories/subdir/nested.txt"))
}

func TestFileSystemStorage_ListDir(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFileSystemStorage(tmpDir)
	assert.NoError(t, err)

	// Create some test files
	storage.WriteFile("/memories/file1.txt", "content1")
	storage.WriteFile("/memories/file2.txt", "content2")
	storage.WriteFile("/memories/subdir/file3.txt", "content3")

	// Create hidden file (should be excluded)
	os.WriteFile(filepath.Join(tmpDir, ".hidden"), []byte("hidden"), 0644)

	listing, err := storage.ListDir("/memories", 2)
	assert.NoError(t, err)

	assert.Contains(t, listing, "file1.txt")
	assert.Contains(t, listing, "file2.txt")
	assert.Contains(t, listing, "subdir")
	assert.NotContains(t, listing, ".hidden")
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0B"},
		{100, "100B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1048576, "1.0M"},
		{1572864, "1.5M"},
		{1073741824, "1.0G"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatSize(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMemoryTool_Integration(t *testing.T) {
	// Test a complete workflow: create, view, edit, rename, delete
	storage := NewInMemoryStorage()
	tool := &MemoryTool{
		storage:   storage,
		memoryDir: "/memories",
	}

	// 1. Create a file
	content := "# Notes\n\nThis is my notes file.\n\n## Todo\n- Item 1\n- Item 2"
	createInput := &MemoryToolInput{
		Command:  MemoryCommandCreate,
		Path:     "/memories/notes.xml",
		FileText: &content,
	}

	result, err := tool.Call(context.Background(), createInput)
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	// 2. View the file
	viewInput := &MemoryToolInput{
		Command: MemoryCommandView,
		Path:    "/memories/notes.xml",
	}

	result, err = tool.Call(context.Background(), viewInput)
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	// 3. Replace some content
	oldStr := "This is my notes file."
	newStr := "This is my comprehensive notes file."
	replaceInput := &MemoryToolInput{
		Command: MemoryCommandStrReplace,
		Path:    "/memories/notes.xml",
		OldStr:  &oldStr,
		NewStr:  &newStr,
	}

	result, err = tool.Call(context.Background(), replaceInput)
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	// 4. Insert a new item
	insertLine := 6
	insertText := "- Item 3"
	insertInput := &MemoryToolInput{
		Command:    MemoryCommandInsert,
		Path:       "/memories/notes.xml",
		InsertLine: &insertLine,
		InsertText: &insertText,
	}

	result, err = tool.Call(context.Background(), insertInput)
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	// 5. Rename the file
	oldPath := "/memories/notes.xml"
	newPath := "/memories/archive/notes.xml"
	renameInput := &MemoryToolInput{
		Command: MemoryCommandRename,
		OldPath: &oldPath,
		NewPath: &newPath,
	}

	result, err = tool.Call(context.Background(), renameInput)
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	// 6. Verify final state
	assert.False(t, storage.Exists("/memories/notes.xml"))
	assert.True(t, storage.Exists("/memories/archive/notes.xml"))

	finalContent, _ := storage.ReadFile("/memories/archive/notes.xml")
	assert.Contains(t, finalContent, "comprehensive notes file")
	assert.Contains(t, finalContent, "Item 3")

	// 7. Delete the file
	deleteInput := &MemoryToolInput{
		Command: MemoryCommandDelete,
		Path:    "/memories/archive",
	}

	result, err = tool.Call(context.Background(), deleteInput)
	assert.NoError(t, err)
	assert.False(t, result.IsError)

	assert.False(t, storage.Exists("/memories/archive"))
}
