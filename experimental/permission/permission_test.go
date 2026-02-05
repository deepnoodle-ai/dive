package permission

import (
	"context"
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

func TestManager(t *testing.T) {
	t.Run("bypass mode allows all", func(t *testing.T) {
		config := &Config{Mode: ModeBypassPermissions}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{"command": "rm -rf /"}`)}

		result, err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAllow, result.Action)
	})

	t.Run("plan mode allows read-only tools", func(t *testing.T) {
		config := &Config{Mode: ModePlan}
		manager := NewManager(config, nil)

		readTool := &mockTool{
			name:        "Read",
			annotations: &dive.ToolAnnotations{ReadOnlyHint: true},
		}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{"file": "test.txt"}`)}

		result, err := manager.EvaluateToolUse(context.Background(), readTool, call)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAllow, result.Action)
	})

	t.Run("plan mode denies non-read-only tools", func(t *testing.T) {
		config := &Config{Mode: ModePlan}
		manager := NewManager(config, nil)

		writeTool := &mockTool{
			name:        "Write",
			annotations: &dive.ToolAnnotations{ReadOnlyHint: false},
		}
		call := &llm.ToolUseContent{Name: "Write", Input: []byte(`{}`)}

		result, err := manager.EvaluateToolUse(context.Background(), writeTool, call)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookDeny, result.Action)
		assert.Contains(t, result.Message, "plan mode")
	})

	t.Run("accept edits mode allows edit operations", func(t *testing.T) {
		config := &Config{Mode: ModeAcceptEdits}
		manager := NewManager(config, nil)

		editTool := &mockTool{
			name:        "Edit",
			annotations: &dive.ToolAnnotations{EditHint: true},
		}
		call := &llm.ToolUseContent{Name: "Edit", Input: []byte(`{}`)}

		result, err := manager.EvaluateToolUse(context.Background(), editTool, call)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAllow, result.Action)
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

		result, err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookDeny, result.Action)
		assert.Equal(t, "Bash is not allowed", result.Message)
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

		result, err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAllow, result.Action)
	})

	t.Run("ask rules match tool name", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute this command?"),
			},
		}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		result, err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAsk, result.Action)
		assert.Equal(t, "Execute this command?", result.Message)
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

		result, err := manager.EvaluateToolUse(context.Background(), tool, matchCall)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAllow, result.Action)

		result, err = manager.EvaluateToolUse(context.Background(), tool, noMatchCall)
		assert.NoError(t, err)
		// Should fall through to default (ask)
		assert.Equal(t, dive.ToolHookAsk, result.Action)
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

		result, err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAllow, result.Action)
	})

	t.Run("session allowlist", func(t *testing.T) {
		config := &Config{Mode: ModeDefault}
		manager := NewManager(config, nil)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}

		// Before adding to allowlist - should ask
		result, err := manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAsk, result.Action)

		// Add bash category to session allowlist
		manager.AllowForSession("bash")

		// After adding to allowlist - should allow
		result, err = manager.EvaluateToolUse(context.Background(), tool, call)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAllow, result.Action)

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

	t.Run("confirm with nil confirmer returns true", func(t *testing.T) {
		manager := NewManager(nil, nil)

		result, err := manager.Confirm(context.Background(), nil, nil, "message")
		assert.NoError(t, err)
		assert.True(t, result)
	})

	t.Run("confirm calls confirmer", func(t *testing.T) {
		var calledWith string
		confirmer := func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, msg string) (bool, error) {
			calledWith = msg
			return true, nil
		}

		manager := NewManager(nil, confirmer)

		result, err := manager.Confirm(context.Background(), nil, nil, "test message")
		assert.NoError(t, err)
		assert.True(t, result)
		assert.Equal(t, "test message", calledWith)
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

		result, err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAllow, result.Action)
	})

	t.Run("ask result triggers confirmation", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute?"),
			},
		}

		confirmed := false
		confirmer := func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, msg string) (bool, error) {
			confirmed = true
			return true, nil
		}

		hook := Hook(config, confirmer)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		hookCtx := &dive.PreToolUseContext{Tool: tool, Call: call}

		result, err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.True(t, confirmed)
		assert.Equal(t, dive.ToolHookAllow, result.Action)
	})

	t.Run("denied confirmation returns deny", func(t *testing.T) {
		config := &Config{
			Mode: ModeDefault,
			Rules: Rules{
				AskRule("Bash", "Execute?"),
			},
		}

		confirmer := func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, msg string) (bool, error) {
			return false, nil
		}

		hook := Hook(config, confirmer)

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		hookCtx := &dive.PreToolUseContext{Tool: tool, Call: call}

		result, err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookDeny, result.Action)
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

		result, err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookContinue, result.Action)
		assert.Equal(t, "Read", loggedName)
		assert.Equal(t, `{"file": "test.txt"}`, string(loggedInput))
	})

	t.Run("handles nil tool", func(t *testing.T) {
		var loggedName string

		hook := AuditHook(func(name string, input []byte) {
			loggedName = name
		})

		hookCtx := &dive.PreToolUseContext{Tool: nil, Call: nil}

		result, err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookContinue, result.Action)
		assert.Equal(t, "unknown", loggedName)
	})
}

func TestHookWithOptions(t *testing.T) {
	t.Run("calls OnAllow callback", func(t *testing.T) {
		var calledTool dive.Tool

		hook := HookWithOptions{
			Config: &Config{
				Mode:  ModeDefault,
				Rules: Rules{AllowRule("Read")},
			},
			OnAllow: func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent) {
				calledTool = tool
			},
		}.Build()

		tool := &mockTool{name: "Read"}
		call := &llm.ToolUseContent{Name: "Read", Input: []byte(`{}`)}
		hookCtx := &dive.PreToolUseContext{Tool: tool, Call: call}

		result, err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookAllow, result.Action)
		assert.Equal(t, tool, calledTool)
	})

	t.Run("calls OnDeny callback", func(t *testing.T) {
		var calledReason string

		hook := HookWithOptions{
			Config: &Config{
				Mode:  ModeDefault,
				Rules: Rules{DenyRule("Bash", "Not allowed")},
			},
			OnDeny: func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, reason string) {
				calledReason = reason
			},
		}.Build()

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		hookCtx := &dive.PreToolUseContext{Tool: tool, Call: call}

		result, err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, dive.ToolHookDeny, result.Action)
		assert.Equal(t, "Not allowed", calledReason)
	})

	t.Run("calls OnAsk callback", func(t *testing.T) {
		var calledMessage string

		hook := HookWithOptions{
			Config: &Config{
				Mode:  ModeDefault,
				Rules: Rules{AskRule("Bash", "Execute?")},
			},
			Confirmer: func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, msg string) (bool, error) {
				return true, nil
			},
			OnAsk: func(ctx context.Context, tool dive.Tool, call *llm.ToolUseContent, message string) {
				calledMessage = message
			},
		}.Build()

		tool := &mockTool{name: "Bash"}
		call := &llm.ToolUseContent{Name: "Bash", Input: []byte(`{}`)}
		hookCtx := &dive.PreToolUseContext{Tool: tool, Call: call}

		_, err := hook(context.Background(), hookCtx)
		assert.NoError(t, err)
		assert.Equal(t, "Execute?", calledMessage)
	})
}
