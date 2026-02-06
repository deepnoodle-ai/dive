package permission

import (
	"context"
	"fmt"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// mockTool implements dive.Tool for testing
type mockTool struct {
	name        string
	annotations *dive.ToolAnnotations
}

func (m *mockTool) Name() string                       { return m.name }
func (m *mockTool) Description() string                { return "Test tool" }
func (m *mockTool) Schema() *dive.Schema               { return nil }
func (m *mockTool) Annotations() *dive.ToolAnnotations { return m.annotations }
func (m *mockTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return nil, nil
}

// testDialog implements dive.Dialog for testing
type testDialog struct {
	showFunc func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error)
}

func (d *testDialog) Show(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
	return d.showFunc(ctx, in)
}

func TestManager(t *testing.T) {
	t.Run("bypass mode allows all", func(t *testing.T) {
		config := &Config{Mode: ModeBypassPermissions}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf /"}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("plan mode allows read-only tools", func(t *testing.T) {
		config := &Config{Mode: ModePlan}
		manager := NewManager(config, nil)

		readTool := &mockTool{
			name:        "Read",
			annotations: &dive.ToolAnnotations{ReadOnlyHint: true},
		}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file": "test.txt"}`)}

		err := manager.EvaluateToolUse(context.Background(), readTool, call)
		assert.NoError(t, err)
	})

	t.Run("plan mode denies non-read-only tools", func(t *testing.T) {
		config := &Config{Mode: ModePlan}
		manager := NewManager(config, nil)

		writeTool := &mockTool{
			name:        "Write",
			annotations: &dive.ToolAnnotations{ReadOnlyHint: false},
		}
		call := &llm.ToolUseContent{Name: "Write", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), writeTool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "plan mode")
	})

	t.Run("accept edits mode allows edit operations", func(t *testing.T) {
		config := &Config{Mode: ModeAcceptEdits}
		manager := NewManager(config, nil)

		editTool := &mockTool{
			name:        "Edit",
			annotations: &dive.ToolAnnotations{EditHint: true},
		}
		call := &llm.ToolUseContent{Name: "Edit", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), editTool, call)
		assert.NoError(t, err)
	})

	t.Run("deny rules take precedence", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("Bash"),
				DenyRule("Bash", "Bash is not allowed"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Equal(t, "Bash is not allowed", err.Error())
	})

	t.Run("allow rules match tool name", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("Read"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Read"}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("ask rules call dialog", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute this command?"),
			},
		}

		var confirmedMsg string
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			confirmedMsg = in.Message
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.Equal(t, "Execute this command?", confirmedMsg)
	})

	t.Run("ask rules deny when dialog denies", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute?"),
			},
		}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Confirmed: false}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("command pattern matching", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowCommandRule("Bash", "go build"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		matchCall := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "go build ./..."}`)}
		noMatchCall := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf"}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, matchCall)
		assert.NoError(t, err)

		// Should fall through to default (confirm). No dialog = auto-allow.
		err = manager.EvaluateToolUse(context.Background(), tool, noMatchCall)
		assert.NoError(t, err)
	})

	t.Run("wildcard tool pattern", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("*"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "AnyTool"}
		call := &llm.ToolUseContent{Name: "AnyTool", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("session allowlist", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		dialog := &dive.DenyAllDialog{}
		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		// Before adding to allowlist - should deny (dialog says no)
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)

		// Add bash category to session allowlist
		manager.AllowForSession("bash")

		// After adding to allowlist - should allow
		err = manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)

		// Check IsSessionAllowed
		assert.True(t, manager.IsSessionAllowed("bash"))
		assert.False(t, manager.IsSessionAllowed("edit"))

		// Clear allowlist
		manager.ClearSessionAllowlist()
		assert.False(t, manager.IsSessionAllowed("bash"))
	})

	t.Run("mode can be changed dynamically", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		manager := NewManager(config, nil)

		assert.Equal(t, ModeDefault, manager.Mode())

		manager.SetMode(ModeBypassPermissions)
		assert.Equal(t, ModeBypassPermissions, manager.Mode())
	})

	t.Run("nil dialog auto-allows", func(t *testing.T) {
		manager := NewManager(nil, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		// Default with nil dialog should auto-allow
		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})

	t.Run("nil config defaults to ModeDefault", func(t *testing.T) {
		manager := NewManager(nil, nil)
		assert.Equal(t, ModeDefault, manager.Mode())
	})
}

func TestRuleHelpers(t *testing.T) {
	t.Run("DenyRule", func(t *testing.T) {
		rule := DenyRule("Bash", "Not allowed")
		assert.Equal(t, RuleDeny, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "Not allowed", rule.Message)
	})

	t.Run("AllowRule", func(t *testing.T) {
		rule := AllowRule("Read")
		assert.Equal(t, RuleAllow, rule.Type)
		assert.Equal(t, "Read", rule.Tool)
	})

	t.Run("AskRule", func(t *testing.T) {
		rule := AskRule("Bash", "Execute?")
		assert.Equal(t, RuleAsk, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "Execute?", rule.Message)
	})

	t.Run("DenyCommandRule", func(t *testing.T) {
		rule := DenyCommandRule("Bash", "rm -rf", "Dangerous command")
		assert.Equal(t, RuleDeny, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "rm -rf", rule.Command)
		assert.Equal(t, "Dangerous command", rule.Message)
	})

	t.Run("AllowCommandRule", func(t *testing.T) {
		rule := AllowCommandRule("Bash", "go build")
		assert.Equal(t, RuleAllow, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "go build", rule.Command)
	})

	t.Run("AskCommandRule", func(t *testing.T) {
		rule := AskCommandRule("Bash", "git push", "Push changes?")
		assert.Equal(t, RuleAsk, rule.Type)
		assert.Equal(t, "Bash", rule.Tool)
		assert.Equal(t, "git push", rule.Command)
		assert.Equal(t, "Push changes?", rule.Message)
	})
}

func TestGetToolCategory(t *testing.T) {
	t.Run("bash patterns", func(t *testing.T) {
		for _, name := range []string{"Bash", "Command", "Shell", "Exec", "RunCommand"} {
			cat := GetToolCategory(name)
			assert.Equal(t, "bash", cat.Key)
		}
	})

	t.Run("edit patterns", func(t *testing.T) {
		for _, name := range []string{"Edit", "Write", "Create", "Mkdir", "Touch"} {
			cat := GetToolCategory(name)
			assert.Equal(t, "edit", cat.Key)
		}
	})

	t.Run("read pattern", func(t *testing.T) {
		cat := GetToolCategory("Read")
		assert.Equal(t, "read", cat.Key)
	})

	t.Run("search patterns", func(t *testing.T) {
		for _, name := range []string{"Glob", "Grep", "Search"} {
			cat := GetToolCategory(name)
			assert.Equal(t, "search", cat.Key)
		}
	})

	t.Run("unknown tool", func(t *testing.T) {
		cat := GetToolCategory("CustomTool")
		assert.Equal(t, "CustomTool", cat.Key)
		assert.Contains(t, cat.Label, "CustomTool")
	})
}

func TestHook(t *testing.T) {
	t.Run("creates hook from config", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AllowRule("Read"),
			},
		}

		hook := Hook(config, nil)
		assert.NotNil(t, hook)

		tool := &mockTool{name: "Read"}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{}`)}
		hookCtx := &dive.PreToolUseContext{Tool: tool, Call: call}

		err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
	})

	t.Run("ask rule triggers confirmation", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute?"),
			},
		}

		confirmed := false
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			confirmed = true
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		hook := Hook(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		hookCtx := &dive.PreToolUseContext{Tool: tool, Call: call}

		err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.True(t, confirmed)
	})

	t.Run("denied confirmation returns error", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute?"),
			},
		}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Confirmed: false}, nil
		}}

		hook := Hook(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		hookCtx := &dive.PreToolUseContext{Tool: tool, Call: call}

		err := hook(context.Background(), hookCtx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})
}

func TestConfirmDialogInput(t *testing.T) {
	t.Run("dialog receives correct fields", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		var received *dive.DialogInput
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			received = in
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "ls"}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.NotNil(t, received)
		assert.True(t, received.Confirm)
		assert.Equal(t, "Bash", received.Title)
		assert.Equal(t, tool, received.Tool)
		assert.Equal(t, call, received.Call)
	})

	t.Run("dialog error propagates", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return nil, fmt.Errorf("dialog failed")
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dialog failed")
	})

	t.Run("dialog canceled treated as denial", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			return &dive.DialogOutput{Canceled: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("ask rule message forwarded to dialog", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Are you sure about this?"),
			},
		}

		var received *dive.DialogInput
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			received = in
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.True(t, received.Confirm)
		assert.Equal(t, "Are you sure about this?", received.Message)
		assert.Equal(t, "Bash", received.Title)
	})

	t.Run("default fallthrough confirms via dialog", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}

		called := false
		dialog := &testDialog{showFunc: func(ctx context.Context, in *dive.DialogInput) (*dive.DialogOutput, error) {
			called = true
			return &dive.DialogOutput{Confirmed: true}, nil
		}}

		manager := NewManager(config, dialog)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("default fallthrough denied via dialog", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		manager := NewManager(config, &dive.DenyAllDialog{})

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})

	t.Run("AutoApproveDialog allows via manager", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		manager := NewManager(config, &dive.AutoApproveDialog{})

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
	})
}

func TestAuditHook(t *testing.T) {
	t.Run("logs tool calls", func(t *testing.T) {
		var loggedName string
		var loggedInput []byte

		hook := AuditHook(func(name string, input []byte) {
			loggedName = name
			loggedInput = input
		})

		tool := &mockTool{name: "Read"}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file": "test.txt"}`)}
		hookCtx := &dive.PreToolUseContext{Tool: tool, Call: call}

		err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, "Read", loggedName)
		assert.Equal(t, `{"file": "test.txt"}`, string(loggedInput))
	})

	t.Run("handles nil tool", func(t *testing.T) {
		var loggedName string

		hook := AuditHook(func(name string, input []byte) {
			loggedName = name
		})

		hookCtx := &dive.PreToolUseContext{Tool: nil, Call: nil}

		err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, "unknown", loggedName)
	})
}

