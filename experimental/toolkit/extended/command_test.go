package extended

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func TestCommandTool_Name(t *testing.T) {
	tool := NewCommandTool()
	assert.Equal(t, "Command", tool.Name())
}

func TestCommandTool_Description(t *testing.T) {
	tool := NewCommandTool()
	desc := tool.Description()
	assert.Contains(t, desc, "Execute")
	assert.Contains(t, desc, "timeout")
	assert.Contains(t, desc, "run_in_background")
	assert.Contains(t, desc, runtime.GOOS)
}

func TestCommandTool_Schema(t *testing.T) {
	tool := NewCommandTool()
	s := tool.Schema()

	assert.Equal(t, schema.Object, s.Type)
	assert.Contains(t, s.Required, "name")
	assert.Contains(t, s.Properties, "name")
	assert.Contains(t, s.Properties, "args")
	assert.Contains(t, s.Properties, "description")
	assert.Contains(t, s.Properties, "timeout")
	assert.Contains(t, s.Properties, "run_in_background")
}

func TestCommandTool_Call(t *testing.T) {
	ctx := context.Background()

	t.Run("SimpleCommand", func(t *testing.T) {
		tool := NewCommandTool()

		var input *CommandInput
		if runtime.GOOS == "windows" {
			input = &CommandInput{Name: "cmd", Args: []any{"/c", "echo", "hello"}}
		} else {
			input = &CommandInput{Name: "echo", Args: []any{"hello"}}
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		var response map[string]string
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["stdout"], "hello")
		assert.Equal(t, "0", response["return_code"])
	})

	t.Run("CommandWithDescription", func(t *testing.T) {
		tool := NewCommandTool()

		var input *CommandInput
		if runtime.GOOS == "windows" {
			input = &CommandInput{
				Name:        "cmd",
				Args:        []any{"/c", "echo", "test"},
				Description: "Print test message",
			}
		} else {
			input = &CommandInput{
				Name:        "echo",
				Args:        []any{"test"},
				Description: "Print test message",
			}
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Display, "Print test message")
	})

	t.Run("CommandWithTimeout", func(t *testing.T) {
		tool := NewCommandTool()

		var input *CommandInput
		if runtime.GOOS == "windows" {
			input = &CommandInput{
				Name:    "cmd",
				Args:    []any{"/c", "echo", "quick"},
				Timeout: 5000,
			}
		} else {
			input = &CommandInput{
				Name:    "echo",
				Args:    []any{"quick"},
				Timeout: 5000,
			}
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})

	t.Run("CommandTimeout", func(t *testing.T) {
		tool := NewCommandTool()

		var input *CommandInput
		if runtime.GOOS == "windows" {
			input = &CommandInput{
				Name:    "cmd",
				Args:    []any{"/c", "ping", "-n", "10", "127.0.0.1"},
				Timeout: 100, // Very short timeout
			}
		} else {
			input = &CommandInput{
				Name:    "sleep",
				Args:    []any{"10"},
				Timeout: 100, // Very short timeout
			}
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "timed out")
	})

	t.Run("MissingCommandName", func(t *testing.T) {
		tool := NewCommandTool()

		input := &CommandInput{Name: ""}
		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "no command name provided")
	})

	t.Run("InvalidArgs", func(t *testing.T) {
		tool := NewCommandTool()

		input := &CommandInput{
			Name: "echo",
			Args: []any{map[string]string{"invalid": "type"}},
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "args must be strings or numbers")
	})

	t.Run("NumericArgs", func(t *testing.T) {
		tool := NewCommandTool()

		var input *CommandInput
		if runtime.GOOS == "windows" {
			input = &CommandInput{Name: "cmd", Args: []any{"/c", "echo", 42, 3.14}}
		} else {
			input = &CommandInput{Name: "echo", Args: []any{42, 3.14}}
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		var response map[string]string
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		assert.NoError(t, err)
		assert.Contains(t, response["stdout"], "42")
	})

	t.Run("FailingCommand", func(t *testing.T) {
		tool := NewCommandTool()

		var input *CommandInput
		if runtime.GOOS == "windows" {
			input = &CommandInput{Name: "cmd", Args: []any{"/c", "exit", "1"}}
		} else {
			input = &CommandInput{Name: "false"}
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)

		var response map[string]string
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		assert.NoError(t, err)
		assert.NotEqual(t, "0", response["return_code"])
	})
}

func TestCommandTool_RunInBackground(t *testing.T) {
	ctx := context.Background()

	t.Run("BackgroundExecution", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewCommandTool(CommandToolOptions{ShellManager: sm})

		var input *CommandInput
		if runtime.GOOS == "windows" {
			input = &CommandInput{
				Name:            "cmd",
				Args:            []any{"/c", "echo", "background"},
				RunInBackground: true,
				Description:     "Background echo",
			}
		} else {
			input = &CommandInput{
				Name:            "echo",
				Args:            []any{"background"},
				RunInBackground: true,
				Description:     "Background echo",
			}
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		var response map[string]string
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		assert.NoError(t, err)

		assert.NotEmpty(t, response["shell_id"])
		assert.Equal(t, "running", response["status"])
		assert.Contains(t, result.Display, "background")

		// Verify it's tracked
		shellID := response["shell_id"]
		info, exists := sm.Get(shellID)
		assert.True(t, exists)
		assert.Equal(t, "Background echo", info.Description)
	})

	t.Run("BackgroundWithoutShellManager", func(t *testing.T) {
		tool := NewCommandTool() // No shell manager

		input := &CommandInput{
			Name:            "echo",
			Args:            []any{"test"},
			RunInBackground: true,
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "background execution not supported")
	})

	t.Run("BackgroundLongRunning", func(t *testing.T) {
		sm := NewShellManager()
		tool := NewCommandTool(CommandToolOptions{ShellManager: sm})

		var input *CommandInput
		if runtime.GOOS == "windows" {
			input = &CommandInput{
				Name:            "cmd",
				Args:            []any{"/c", "ping", "-n", "5", "127.0.0.1"},
				RunInBackground: true,
			}
		} else {
			input = &CommandInput{
				Name:            "sleep",
				Args:            []any{"5"},
				RunInBackground: true,
			}
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		var response map[string]string
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		assert.NoError(t, err)

		shellID := response["shell_id"]
		assert.True(t, sm.IsRunning(shellID))

		// Clean up
		sm.Kill(shellID)
	})
}

func TestCommandTool_PreviewCall(t *testing.T) {
	ctx := context.Background()
	tool := NewCommandTool()

	t.Run("SimplePreview", func(t *testing.T) {
		input := &CommandInput{Name: "ls", Args: []any{"-la"}}
		preview := tool.PreviewCall(ctx, input)
		assert.Contains(t, preview.Summary, "ls -la")
	})

	t.Run("BackgroundPreview", func(t *testing.T) {
		input := &CommandInput{Name: "sleep", Args: []any{"10"}, RunInBackground: true}
		preview := tool.PreviewCall(ctx, input)
		assert.Contains(t, preview.Summary, "background")
	})

	t.Run("DescriptionPreview", func(t *testing.T) {
		input := &CommandInput{Name: "ls", Description: "List files"}
		preview := tool.PreviewCall(ctx, input)
		assert.Equal(t, "List files", preview.Summary)
	})
}

func TestCommandTool_Annotations(t *testing.T) {
	tool := NewCommandTool()
	annotations := tool.Annotations()

	assert.NotNil(t, annotations)
	assert.Equal(t, "Command", annotations.Title)
	assert.False(t, annotations.ReadOnlyHint)
	assert.True(t, annotations.DestructiveHint)
	assert.True(t, annotations.OpenWorldHint)
}

func TestCommandTool_TimeoutLimits(t *testing.T) {
	ctx := context.Background()
	tool := NewCommandTool()

	t.Run("MaxTimeoutCap", func(t *testing.T) {
		// Request a timeout longer than max (10 min)
		var input *CommandInput
		if runtime.GOOS == "windows" {
			input = &CommandInput{
				Name:    "cmd",
				Args:    []any{"/c", "echo", "test"},
				Timeout: 700000, // 11+ minutes
			}
		} else {
			input = &CommandInput{
				Name:    "echo",
				Args:    []any{"test"},
				Timeout: 700000, // 11+ minutes
			}
		}

		// Should succeed (timeout capped to max)
		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)
	})
}

func TestCommandTool_WorkingDirectory(t *testing.T) {
	ctx := context.Background()

	t.Run("ValidWorkingDirectory", func(t *testing.T) {
		// Use "/" as workspace to allow access to /tmp
		tool := NewCommandTool(CommandToolOptions{WorkspaceDir: "/"})

		var input *CommandInput
		if runtime.GOOS == "windows" {
			input = &CommandInput{
				Name:             "cmd",
				Args:             []any{"/c", "cd"},
				WorkingDirectory: "/",
			}
		} else {
			input = &CommandInput{
				Name:             "pwd",
				WorkingDirectory: "/tmp",
			}
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError, "Result error: %s", result.Content[0].Text)

		var response map[string]string
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		assert.NoError(t, err)

		if runtime.GOOS != "windows" {
			// On macOS, /tmp is a symlink to /private/tmp
			assert.True(t, response["stdout"] == "/tmp\n" || response["stdout"] == "/private/tmp\n",
				"Expected /tmp or /private/tmp, got: %s", response["stdout"])
		}
	})
}

func TestCommandTool_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("FullWorkflow", func(t *testing.T) {
		sm := NewShellManager()
		cmdTool := NewCommandTool(CommandToolOptions{ShellManager: sm})
		getOutputTool := NewGetShellOutputTool(GetShellOutputToolOptions{ShellManager: sm})

		// Start a background command
		var cmdInput *CommandInput
		if runtime.GOOS == "windows" {
			cmdInput = &CommandInput{
				Name:            "cmd",
				Args:            []any{"/c", "echo", "workflow test"},
				RunInBackground: true,
			}
		} else {
			cmdInput = &CommandInput{
				Name:            "echo",
				Args:            []any{"workflow test"},
				RunInBackground: true,
			}
		}

		result, err := cmdTool.Call(ctx, cmdInput)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		var cmdResponse map[string]string
		err = json.Unmarshal([]byte(result.Content[0].Text), &cmdResponse)
		assert.NoError(t, err)
		shellID := cmdResponse["shell_id"]

		// Wait for output
		time.Sleep(200 * time.Millisecond)

		outputInput := &GetShellOutputInput{
			ShellID: shellID,
			Timeout: 5000,
		}

		outputResult, err := getOutputTool.Call(ctx, outputInput)
		assert.NoError(t, err)
		assert.False(t, outputResult.IsError)

		var outputResponse map[string]interface{}
		err = json.Unmarshal([]byte(outputResult.Content[0].Text), &outputResponse)
		assert.NoError(t, err)

		assert.Equal(t, "completed", outputResponse["status"])
		assert.Contains(t, outputResponse["stdout"], "workflow test")
	})
}
