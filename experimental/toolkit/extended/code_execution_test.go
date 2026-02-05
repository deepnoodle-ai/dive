package extended

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestCodeExecutionTool_Name(t *testing.T) {
	tool := NewCodeExecutionTool()
	assert.Equal(t, "code_execution", tool.Name())
}

func TestCodeExecutionTool_Description(t *testing.T) {
	tool := NewCodeExecutionTool()
	assert.Contains(t, tool.Description(), "Bash commands")
	assert.Contains(t, tool.Description(), "sandboxed environment")
}

func TestCodeExecutionTool_Schema(t *testing.T) {
	tool := NewCodeExecutionTool()
	// Server-side tools have nil schema
	assert.Nil(t, tool.Schema())
}

func TestCodeExecutionTool_Annotations(t *testing.T) {
	tool := NewCodeExecutionTool()
	annotations := tool.Annotations()
	assert.NotNil(t, annotations)
	assert.Equal(t, "Code Execution", annotations.Title)
	assert.False(t, annotations.ReadOnlyHint)    // Can create/modify files
	assert.False(t, annotations.DestructiveHint) // Sandboxed
	assert.False(t, annotations.IdempotentHint)  // Not idempotent
	assert.False(t, annotations.OpenWorldHint)   // No internet access
}

func TestCodeExecutionTool_ToolConfiguration(t *testing.T) {
	tool := NewCodeExecutionTool()
	config := tool.ToolConfiguration("anthropic")
	assert.NotNil(t, config)
	assert.Equal(t, "code_execution_20250825", config["type"])
	assert.Equal(t, "code_execution", config["name"])
}

func TestCodeExecutionTool_ToolConfiguration_Legacy(t *testing.T) {
	tool := NewCodeExecutionTool(CodeExecutionToolOptions{
		Type: CodeExecutionToolTypeLegacy,
	})
	config := tool.ToolConfiguration("anthropic")
	assert.NotNil(t, config)
	assert.Equal(t, "code_execution_20250522", config["type"])
	assert.Equal(t, "code_execution", config["name"])
}

func TestCodeExecutionTool_Call_ReturnsError(t *testing.T) {
	tool := NewCodeExecutionTool()
	_, err := tool.Call(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server-side tool does not implement local calls")
}

func TestCodeExecutionTool_Type(t *testing.T) {
	tool := NewCodeExecutionTool()
	assert.Equal(t, anthropic.CodeExecutionToolType, tool.Type())

	legacyTool := NewCodeExecutionTool(CodeExecutionToolOptions{
		Type: CodeExecutionToolTypeLegacy,
	})
	assert.Equal(t, anthropic.CodeExecutionToolTypeLegacy, legacyTool.Type())
}

func TestCodeExecutionToolType_Constants(t *testing.T) {
	assert.Equal(t, "code_execution_20250825", CodeExecutionToolType)
	assert.Equal(t, "code_execution_20250522", CodeExecutionToolTypeLegacy)
}

func TestBashCodeExecutionToolResultContent_Unmarshal(t *testing.T) {
	jsonData := `{
		"type": "bash_code_execution_tool_result",
		"tool_use_id": "srvtoolu_01B3C4D5E6F7G8H9I0J1K2L3",
		"content": {
			"type": "bash_code_execution_result",
			"stdout": "total 24\ndrwxr-xr-x 2 user user 4096 Jan 1 12:00 .",
			"stderr": "",
			"return_code": 0
		}
	}`

	content, err := llm.UnmarshalContent([]byte(jsonData))
	assert.NoError(t, err)
	assert.NotNil(t, content)
	assert.Equal(t, llm.ContentTypeBashCodeExecutionToolResult, content.Type())

	bashResult, ok := content.(*llm.BashCodeExecutionToolResultContent)
	assert.True(t, ok)
	assert.Equal(t, "srvtoolu_01B3C4D5E6F7G8H9I0J1K2L3", bashResult.ToolUseID)
	assert.Equal(t, "bash_code_execution_result", bashResult.Content.Type)
	assert.Contains(t, bashResult.Content.Stdout, "total 24")
	assert.Empty(t, bashResult.Content.Stderr)
	assert.Equal(t, 0, bashResult.Content.ReturnCode)
	assert.False(t, bashResult.IsError())
}

func TestBashCodeExecutionToolResultContent_UnmarshalError(t *testing.T) {
	jsonData := `{
		"type": "bash_code_execution_tool_result",
		"tool_use_id": "srvtoolu_01VfmxgZ46TiHbmXgy928hQR",
		"content": {
			"type": "bash_code_execution_tool_result_error",
			"error_code": "unavailable"
		}
	}`

	content, err := llm.UnmarshalContent([]byte(jsonData))
	assert.NoError(t, err)
	assert.NotNil(t, content)

	bashResult, ok := content.(*llm.BashCodeExecutionToolResultContent)
	assert.True(t, ok)
	assert.True(t, bashResult.IsError())
	assert.Equal(t, "unavailable", bashResult.Content.ErrorCode)
}

func TestBashCodeExecutionToolResultContent_Marshal(t *testing.T) {
	content := &llm.BashCodeExecutionToolResultContent{
		ToolUseID: "test_id",
		Content: llm.BashCodeExecutionResult{
			Type:       "bash_code_execution_result",
			Stdout:     "hello world",
			Stderr:     "",
			ReturnCode: 0,
		},
	}

	data, err := json.Marshal(content)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"type":"bash_code_execution_tool_result"`)
	assert.Contains(t, string(data), `"tool_use_id":"test_id"`)
}

func TestTextEditorCodeExecutionToolResultContent_UnmarshalView(t *testing.T) {
	jsonData := `{
		"type": "text_editor_code_execution_tool_result",
		"tool_use_id": "srvtoolu_01C4D5E6F7G8H9I0J1K2L3M4",
		"content": {
			"type": "text_editor_code_execution_result",
			"file_type": "text",
			"content": "{\n  \"setting\": \"value\"\n}",
			"numLines": 3,
			"startLine": 1,
			"totalLines": 3
		}
	}`

	content, err := llm.UnmarshalContent([]byte(jsonData))
	assert.NoError(t, err)
	assert.NotNil(t, content)
	assert.Equal(t, llm.ContentTypeTextEditorCodeExecutionToolResult, content.Type())

	editorResult, ok := content.(*llm.TextEditorCodeExecutionToolResultContent)
	assert.True(t, ok)
	assert.Equal(t, "srvtoolu_01C4D5E6F7G8H9I0J1K2L3M4", editorResult.ToolUseID)
	assert.Equal(t, "text_editor_code_execution_result", editorResult.Content.Type)
	assert.Equal(t, "text", editorResult.Content.FileType)
	assert.Contains(t, editorResult.Content.Content, "setting")
	assert.Equal(t, 3, editorResult.Content.NumLines)
	assert.Equal(t, 1, editorResult.Content.StartLine)
	assert.Equal(t, 3, editorResult.Content.TotalLines)
	assert.False(t, editorResult.IsError())
}

func TestTextEditorCodeExecutionToolResultContent_UnmarshalCreate(t *testing.T) {
	jsonData := `{
		"type": "text_editor_code_execution_tool_result",
		"tool_use_id": "srvtoolu_01D5E6F7G8H9I0J1K2L3M4N5",
		"content": {
			"type": "text_editor_code_execution_result",
			"is_file_update": false
		}
	}`

	content, err := llm.UnmarshalContent([]byte(jsonData))
	assert.NoError(t, err)
	assert.NotNil(t, content)

	editorResult, ok := content.(*llm.TextEditorCodeExecutionToolResultContent)
	assert.True(t, ok)
	assert.NotNil(t, editorResult.Content.IsFileUpdate)
	assert.False(t, *editorResult.Content.IsFileUpdate)
}

func TestTextEditorCodeExecutionToolResultContent_UnmarshalEdit(t *testing.T) {
	jsonData := `{
		"type": "text_editor_code_execution_tool_result",
		"tool_use_id": "srvtoolu_01E6F7G8H9I0J1K2L3M4N5O6",
		"content": {
			"type": "text_editor_code_execution_result",
			"oldStart": 3,
			"oldLines": 1,
			"newStart": 3,
			"newLines": 1,
			"lines": ["-  \"debug\": true", "+  \"debug\": false"]
		}
	}`

	content, err := llm.UnmarshalContent([]byte(jsonData))
	assert.NoError(t, err)
	assert.NotNil(t, content)

	editorResult, ok := content.(*llm.TextEditorCodeExecutionToolResultContent)
	assert.True(t, ok)
	assert.Equal(t, 3, editorResult.Content.OldStart)
	assert.Equal(t, 1, editorResult.Content.OldLines)
	assert.Equal(t, 3, editorResult.Content.NewStart)
	assert.Equal(t, 1, editorResult.Content.NewLines)
	assert.Len(t, editorResult.Content.Lines, 2)
	assert.Equal(t, "-  \"debug\": true", editorResult.Content.Lines[0])
	assert.Equal(t, "+  \"debug\": false", editorResult.Content.Lines[1])
}

func TestTextEditorCodeExecutionToolResultContent_UnmarshalError(t *testing.T) {
	jsonData := `{
		"type": "text_editor_code_execution_tool_result",
		"tool_use_id": "srvtoolu_error",
		"content": {
			"type": "text_editor_code_execution_tool_result_error",
			"error_code": "file_not_found"
		}
	}`

	content, err := llm.UnmarshalContent([]byte(jsonData))
	assert.NoError(t, err)
	assert.NotNil(t, content)

	editorResult, ok := content.(*llm.TextEditorCodeExecutionToolResultContent)
	assert.True(t, ok)
	assert.True(t, editorResult.IsError())
	assert.Equal(t, "file_not_found", editorResult.Content.ErrorCode)
}

func TestTextEditorCodeExecutionToolResultContent_Marshal(t *testing.T) {
	isFileUpdate := false
	content := &llm.TextEditorCodeExecutionToolResultContent{
		ToolUseID: "test_id",
		Content: llm.TextEditorCodeExecutionResult{
			Type:         "text_editor_code_execution_result",
			IsFileUpdate: &isFileUpdate,
		},
	}

	data, err := json.Marshal(content)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"type":"text_editor_code_execution_tool_result"`)
	assert.Contains(t, string(data), `"tool_use_id":"test_id"`)
}

func TestBashCodeExecutionToolResultContent_WithFiles(t *testing.T) {
	jsonData := `{
		"type": "bash_code_execution_tool_result",
		"tool_use_id": "srvtoolu_files",
		"content": {
			"type": "bash_code_execution_result",
			"stdout": "File created",
			"stderr": "",
			"return_code": 0,
			"content": [
				{"file_id": "file_abc123"},
				{"file_id": "file_def456"}
			]
		}
	}`

	content, err := llm.UnmarshalContent([]byte(jsonData))
	assert.NoError(t, err)
	assert.NotNil(t, content)

	bashResult, ok := content.(*llm.BashCodeExecutionToolResultContent)
	assert.True(t, ok)
	assert.Len(t, bashResult.Content.Content, 2)
	assert.Equal(t, "file_abc123", bashResult.Content.Content[0].FileID)
	assert.Equal(t, "file_def456", bashResult.Content.Content[1].FileID)
}
