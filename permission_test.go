package dive

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// mockTool is a simple mock tool for testing
type mockTool struct {
	name        string
	annotations *ToolAnnotations
}

func (m *mockTool) Name() string                  { return m.name }
func (m *mockTool) Description() string           { return "mock tool" }
func (m *mockTool) Schema() *Schema               { return nil }
func (m *mockTool) Annotations() *ToolAnnotations { return m.annotations }
func (m *mockTool) Call(ctx context.Context, input any) (*ToolResult, error) {
	return NewToolResultText("success"), nil
}

func newMockTool(name string, annotations *ToolAnnotations) *mockTool {
	return &mockTool{name: name, annotations: annotations}
}

func newMockToolCall(name string, input map[string]any) *llm.ToolUseContent {
	inputBytes, _ := json.Marshal(input)
	return &llm.ToolUseContent{
		ID:    "test-id",
		Name:  name,
		Input: inputBytes,
	}
}

func TestPermissionModes(t *testing.T) {
	ctx := context.Background()

	t.Run("BypassPermissions allows all tools", func(t *testing.T) {
		config := &PermissionConfig{Mode: PermissionModeBypassPermissions}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("dangerous_tool", &ToolAnnotations{DestructiveHint: true})
		call := newMockToolCall("dangerous_tool", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("Plan mode allows read-only tools", func(t *testing.T) {
		config := &PermissionConfig{Mode: PermissionModePlan}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("read_file", &ToolAnnotations{ReadOnlyHint: true})
		call := newMockToolCall("read_file", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("Plan mode denies non-read-only tools", func(t *testing.T) {
		config := &PermissionConfig{Mode: PermissionModePlan}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("write_file", &ToolAnnotations{ReadOnlyHint: false})
		call := newMockToolCall("write_file", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookDeny, result.Action)
	})

	t.Run("AcceptEdits mode allows edit tools", func(t *testing.T) {
		config := &PermissionConfig{Mode: PermissionModeAcceptEdits}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("edit", &ToolAnnotations{EditHint: true})
		call := newMockToolCall("edit", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("AcceptEdits mode continues for non-edit tools", func(t *testing.T) {
		config := &PermissionConfig{Mode: PermissionModeAcceptEdits}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("fetch", &ToolAnnotations{ReadOnlyHint: true})
		call := newMockToolCall("fetch", nil)

		// Without a CanUseTool callback, it defaults to ask
		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAsk, result.Action)
	})

	t.Run("Default mode falls through to ask", func(t *testing.T) {
		config := &PermissionConfig{Mode: PermissionModeDefault}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("any_tool", nil)
		call := newMockToolCall("any_tool", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAsk, result.Action)
	})
}

func TestPermissionRules(t *testing.T) {
	ctx := context.Background()

	t.Run("Deny rule blocks tool", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			Rules: PermissionRules{
				DenyRule("dangerous_*", "This tool is not allowed"),
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("dangerous_command", nil)
		call := newMockToolCall("dangerous_command", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookDeny, result.Action)
		assert.Equal(t, "This tool is not allowed", result.Message)
	})

	t.Run("Allow rule permits tool", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			Rules: PermissionRules{
				AllowRule("safe_*"),
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("safe_tool", nil)
		call := newMockToolCall("safe_tool", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("Ask rule prompts for confirmation", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			Rules: PermissionRules{
				AskRule("bash", "Confirm bash command"),
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("bash", nil)
		call := newMockToolCall("bash", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAsk, result.Action)
		assert.Equal(t, "Confirm bash command", result.Message)
	})

	t.Run("Deny rules take precedence", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			Rules: PermissionRules{
				AllowRule("*"),
				DenyRule("dangerous", "blocked"),
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("dangerous", nil)
		call := newMockToolCall("dangerous", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookDeny, result.Action)
	})

	t.Run("Command pattern matching", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			Rules: PermissionRules{
				DenyCommandRule("bash", "rm -rf *", "Destructive command not allowed"),
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("bash", nil)
		call := newMockToolCall("bash", map[string]any{"command": "rm -rf /tmp"})

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookDeny, result.Action)
	})

	t.Run("Wildcard pattern matching", func(t *testing.T) {
		rules := PermissionRules{
			AllowRule("read_*"),
		}

		tool := newMockTool("read_file", nil)
		call := newMockToolCall("read_file", nil)

		result := rules.Evaluate(tool, call)
		assert.NotNil(t, result)
		assert.Equal(t, ToolHookAllow, result.Action)
	})
}

func TestPreToolUseHooks(t *testing.T) {
	ctx := context.Background()

	t.Run("Hook can allow tool", func(t *testing.T) {
		hookCalled := false
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
					hookCalled = true
					return AllowResult(), nil
				},
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.True(t, hookCalled)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("Hook can deny tool", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
					return DenyResult("Blocked by hook"), nil
				},
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookDeny, result.Action)
		assert.Equal(t, "Blocked by hook", result.Message)
	})

	t.Run("Hook continue passes to rules", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
					return ContinueResult(), nil
				},
			},
			Rules: PermissionRules{
				AllowRule("test"),
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("Hook can modify input", func(t *testing.T) {
		newInput := json.RawMessage(`{"modified": true}`)
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
					return AllowResultWithInput(newInput), nil
				},
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", map[string]any{"original": true})

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
		assert.Equal(t, newInput, result.UpdatedInput)
	})
}

func TestCanUseToolCallback(t *testing.T) {
	ctx := context.Background()

	t.Run("CanUseTool callback is invoked", func(t *testing.T) {
		callbackInvoked := false
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			CanUseTool: func(ctx context.Context, tool Tool, call *llm.ToolUseContent) (*ToolHookResult, error) {
				callbackInvoked = true
				return AllowResult(), nil
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.True(t, callbackInvoked)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("CanUseTool is skipped if rules match", func(t *testing.T) {
		callbackInvoked := false
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			Rules: PermissionRules{
				AllowRule("test"),
			},
			CanUseTool: func(ctx context.Context, tool Tool, call *llm.ToolUseContent) (*ToolHookResult, error) {
				callbackInvoked = true
				return DenyResult("Should not reach here"), nil
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.False(t, callbackInvoked)
		assert.Equal(t, ToolHookAllow, result.Action)
	})
}

func TestPostToolUseHooks(t *testing.T) {
	ctx := context.Background()

	t.Run("PostToolUse hooks are called", func(t *testing.T) {
		hookCalled := false
		config := &PermissionConfig{
			Mode: PermissionModeBypassPermissions,
			PostToolUse: []PostToolUseHook{
				func(ctx context.Context, hookCtx *PostToolUseContext) error {
					hookCalled = true
					assert.NotNil(t, hookCtx.Tool)
					assert.NotNil(t, hookCtx.Call)
					assert.NotNil(t, hookCtx.Result)
					return nil
				},
			},
		}
		pm := NewPermissionManager(config, nil)

		postCtx := &PostToolUseContext{
			Tool:   newMockTool("test", nil),
			Call:   newMockToolCall("test", nil),
			Result: &ToolCallResult{ID: "test"},
		}

		err := pm.RunPostToolUseHooks(ctx, postCtx)
		assert.NoError(t, err)
		assert.True(t, hookCalled)
	})
}

func TestPermissionHelpers(t *testing.T) {
	t.Run("AllowResult creates correct result", func(t *testing.T) {
		result := AllowResult()
		assert.Equal(t, ToolHookAllow, result.Action)
		assert.Empty(t, result.Message)
		assert.Nil(t, result.UpdatedInput)
	})

	t.Run("DenyResult creates correct result", func(t *testing.T) {
		result := DenyResult("blocked")
		assert.Equal(t, ToolHookDeny, result.Action)
		assert.Equal(t, "blocked", result.Message)
	})

	t.Run("AskResult creates correct result", func(t *testing.T) {
		result := AskResult("confirm?")
		assert.Equal(t, ToolHookAsk, result.Action)
		assert.Equal(t, "confirm?", result.Message)
	})

	t.Run("ContinueResult creates correct result", func(t *testing.T) {
		result := ContinueResult()
		assert.Equal(t, ToolHookContinue, result.Action)
	})
}

func TestRuleHelpers(t *testing.T) {
	t.Run("DenyRule creates correct rule", func(t *testing.T) {
		rule := DenyRule("bash", "no bash")
		assert.Equal(t, PermissionRuleDeny, rule.Type)
		assert.Equal(t, "bash", rule.Tool)
		assert.Equal(t, "no bash", rule.Message)
	})

	t.Run("AllowRule creates correct rule", func(t *testing.T) {
		rule := AllowRule("read_*")
		assert.Equal(t, PermissionRuleAllow, rule.Type)
		assert.Equal(t, "read_*", rule.Tool)
	})

	t.Run("AskRule creates correct rule", func(t *testing.T) {
		rule := AskRule("write_*", "confirm write")
		assert.Equal(t, PermissionRuleAsk, rule.Type)
		assert.Equal(t, "write_*", rule.Tool)
		assert.Equal(t, "confirm write", rule.Message)
	})

	t.Run("DenyCommandRule creates correct rule", func(t *testing.T) {
		rule := DenyCommandRule("bash", "rm *", "dangerous")
		assert.Equal(t, PermissionRuleDeny, rule.Type)
		assert.Equal(t, "bash", rule.Tool)
		assert.Equal(t, "rm *", rule.Command)
		assert.Equal(t, "dangerous", rule.Message)
	})
}

func TestPermissionConfigFromInteractionMode(t *testing.T) {
	t.Run("InteractNever maps to BypassPermissions", func(t *testing.T) {
		config := PermissionConfigFromInteractionMode(InteractNever)
		assert.Equal(t, PermissionModeBypassPermissions, config.Mode)
	})

	t.Run("InteractAlways maps to ask-all rule", func(t *testing.T) {
		config := PermissionConfigFromInteractionMode(InteractAlways)
		assert.Equal(t, PermissionModeDefault, config.Mode)
		assert.Len(t, config.Rules, 1)
		assert.Equal(t, PermissionRuleAsk, config.Rules[0].Type)
		assert.Equal(t, "*", config.Rules[0].Tool)
	})

	t.Run("InteractIfDestructive sets CanUseTool", func(t *testing.T) {
		config := PermissionConfigFromInteractionMode(InteractIfDestructive)
		assert.Equal(t, PermissionModeDefault, config.Mode)
		assert.NotNil(t, config.CanUseTool)

		ctx := context.Background()

		// Non-destructive tool should be allowed
		result, err := config.CanUseTool(ctx, newMockTool("safe", &ToolAnnotations{DestructiveHint: false}), nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)

		// Destructive tool should ask
		result, err = config.CanUseTool(ctx, newMockTool("danger", &ToolAnnotations{DestructiveHint: true}), nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAsk, result.Action)
	})

	t.Run("InteractIfNotReadOnly sets CanUseTool", func(t *testing.T) {
		config := PermissionConfigFromInteractionMode(InteractIfNotReadOnly)
		assert.Equal(t, PermissionModeDefault, config.Mode)
		assert.NotNil(t, config.CanUseTool)

		ctx := context.Background()

		// Read-only tool should be allowed
		result, err := config.CanUseTool(ctx, newMockTool("read", &ToolAnnotations{ReadOnlyHint: true}), nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)

		// Non-read-only tool should ask
		result, err = config.CanUseTool(ctx, newMockTool("write", &ToolAnnotations{ReadOnlyHint: false}), nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAsk, result.Action)
	})
}

func TestIsEditOperation(t *testing.T) {
	config := &PermissionConfig{Mode: PermissionModeAcceptEdits}
	pm := NewPermissionManager(config, nil)

	t.Run("EditHint annotation is detected", func(t *testing.T) {
		tool := newMockTool("custom_edit", &ToolAnnotations{EditHint: true})
		call := newMockToolCall("custom_edit", nil)

		result, err := pm.EvaluateToolUse(context.Background(), tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("Edit tool name is detected", func(t *testing.T) {
		tool := newMockTool("edit_file", nil)
		call := newMockToolCall("edit_file", nil)

		result, err := pm.EvaluateToolUse(context.Background(), tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("Write tool name is detected", func(t *testing.T) {
		tool := newMockTool("write_config", nil)
		call := newMockToolCall("write_config", nil)

		result, err := pm.EvaluateToolUse(context.Background(), tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("Bash mkdir command is detected as edit", func(t *testing.T) {
		tool := newMockTool("bash", nil)
		call := newMockToolCall("bash", map[string]any{"command": "mkdir -p /tmp/test"})

		result, err := pm.EvaluateToolUse(context.Background(), tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
	})
}

// Additional edge case tests

func TestHookErrorHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("PreToolUse hook error terminates flow", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
					return nil, context.Canceled
				},
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("CanUseTool error terminates flow", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			CanUseTool: func(ctx context.Context, tool Tool, call *llm.ToolUseContent) (*ToolHookResult, error) {
				return nil, context.DeadlineExceeded
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("PostToolUse hook error is returned", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			PostToolUse: []PostToolUseHook{
				func(ctx context.Context, hookCtx *PostToolUseContext) error {
					return context.Canceled
				},
			},
		}
		pm := NewPermissionManager(config, nil)

		postCtx := &PostToolUseContext{
			Tool:   newMockTool("test", nil),
			Call:   newMockToolCall("test", nil),
			Result: &ToolCallResult{ID: "test"},
		}

		err := pm.RunPostToolUseHooks(ctx, postCtx)
		assert.Error(t, err)
	})
}

func TestMultipleHooksChaining(t *testing.T) {
	ctx := context.Background()

	t.Run("Multiple PreToolUse hooks called in order", func(t *testing.T) {
		order := []int{}
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
					order = append(order, 1)
					return ContinueResult(), nil
				},
				func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
					order = append(order, 2)
					return ContinueResult(), nil
				},
				func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
					order = append(order, 3)
					return AllowResult(), nil
				},
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
		assert.Equal(t, []int{1, 2, 3}, order)
	})

	t.Run("Early hook termination stops chain", func(t *testing.T) {
		order := []int{}
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
					order = append(order, 1)
					return DenyResult("blocked"), nil
				},
				func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
					order = append(order, 2) // Should not be called
					return AllowResult(), nil
				},
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookDeny, result.Action)
		assert.Equal(t, []int{1}, order)
	})

	t.Run("Multiple PostToolUse hooks all called", func(t *testing.T) {
		count := 0
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			PostToolUse: []PostToolUseHook{
				func(ctx context.Context, hookCtx *PostToolUseContext) error {
					count++
					return nil
				},
				func(ctx context.Context, hookCtx *PostToolUseContext) error {
					count++
					return nil
				},
			},
		}
		pm := NewPermissionManager(config, nil)

		postCtx := &PostToolUseContext{
			Tool:   newMockTool("test", nil),
			Call:   newMockToolCall("test", nil),
			Result: &ToolCallResult{ID: "test"},
		}

		err := pm.RunPostToolUseHooks(ctx, postCtx)
		assert.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func TestInputMatchFunction(t *testing.T) {
	ctx := context.Background()

	t.Run("InputMatch function filters rules", func(t *testing.T) {
		config := &PermissionConfig{
			Mode: PermissionModeDefault,
			Rules: PermissionRules{
				{
					Type: PermissionRuleDeny,
					Tool: "write_file",
					InputMatch: func(input any) bool {
						m, ok := input.(map[string]any)
						if !ok {
							return false
						}
						path, _ := m["path"].(string)
						return path == "/etc/passwd"
					},
					Message: "Cannot write to /etc/passwd",
				},
			},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("write_file", nil)

		// Should be denied - matches /etc/passwd
		call := newMockToolCall("write_file", map[string]any{"path": "/etc/passwd"})
		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookDeny, result.Action)

		// Should fall through to ask - different path
		call = newMockToolCall("write_file", map[string]any{"path": "/tmp/test"})
		result, err = pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAsk, result.Action)
	})
}

func TestNilHandling(t *testing.T) {
	t.Run("Rules.Evaluate handles nil tool", func(t *testing.T) {
		rules := PermissionRules{AllowRule("*")}
		result := rules.Evaluate(nil, newMockToolCall("test", nil))
		assert.Nil(t, result)
	})

	t.Run("Rules.Evaluate handles nil call", func(t *testing.T) {
		rules := PermissionRules{AllowRule("*")}
		result := rules.Evaluate(newMockTool("test", nil), nil)
		assert.Nil(t, result)
	})

	t.Run("NewPermissionManager handles nil config", func(t *testing.T) {
		pm := NewPermissionManager(nil, nil)
		assert.Equal(t, PermissionModeDefault, pm.Mode())
	})
}

func TestConfirmerCallback(t *testing.T) {
	ctx := context.Background()

	t.Run("Confirm uses configured confirmer", func(t *testing.T) {
		confirmerCalled := false
		pm := NewPermissionManager(nil, func(ctx context.Context, tool Tool, call *llm.ToolUseContent, message string) (bool, error) {
			confirmerCalled = true
			assert.Equal(t, "test message", message)
			return true, nil
		})

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		confirmed, err := pm.Confirm(ctx, tool, call, "test message")
		assert.NoError(t, err)
		assert.True(t, confirmed)
		assert.True(t, confirmerCalled)
	})

	t.Run("Confirm returns false when denied", func(t *testing.T) {
		pm := NewPermissionManager(nil, func(ctx context.Context, tool Tool, call *llm.ToolUseContent, message string) (bool, error) {
			return false, nil
		})

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		confirmed, err := pm.Confirm(ctx, tool, call, "")
		assert.NoError(t, err)
		assert.False(t, confirmed)
	})

	t.Run("Confirm defaults to allow when no confirmer", func(t *testing.T) {
		pm := NewPermissionManager(nil, nil)

		tool := newMockTool("test", nil)
		call := newMockToolCall("test", nil)

		confirmed, err := pm.Confirm(ctx, tool, call, "")
		assert.NoError(t, err)
		assert.True(t, confirmed)
	})
}

func TestSetMode(t *testing.T) {
	pm := NewPermissionManager(nil, nil)

	assert.Equal(t, PermissionModeDefault, pm.Mode())

	pm.SetMode(PermissionModeBypassPermissions)
	assert.Equal(t, PermissionModeBypassPermissions, pm.Mode())

	pm.SetMode(PermissionModePlan)
	assert.Equal(t, PermissionModePlan, pm.Mode())
}

func TestCommandPatternMatching(t *testing.T) {
	t.Run("Exact command match", func(t *testing.T) {
		rules := PermissionRules{
			DenyCommandRule("bash", "rm -rf /", "exact match"),
		}
		tool := newMockTool("bash", nil)
		call := newMockToolCall("bash", map[string]any{"command": "rm -rf /"})

		result := rules.Evaluate(tool, call)
		assert.NotNil(t, result)
		assert.Equal(t, ToolHookDeny, result.Action)
	})

	t.Run("Wildcard at end", func(t *testing.T) {
		rules := PermissionRules{
			DenyCommandRule("bash", "git push *", "no push"),
		}
		tool := newMockTool("bash", nil)
		call := newMockToolCall("bash", map[string]any{"command": "git push origin main"})

		result := rules.Evaluate(tool, call)
		assert.NotNil(t, result)
		assert.Equal(t, ToolHookDeny, result.Action)
	})

	t.Run("Wildcard in middle", func(t *testing.T) {
		rules := PermissionRules{
			DenyCommandRule("bash", "curl * | bash", "no piped curl"),
		}
		tool := newMockTool("bash", nil)
		call := newMockToolCall("bash", map[string]any{"command": "curl http://example.com | bash"})

		result := rules.Evaluate(tool, call)
		assert.NotNil(t, result)
		assert.Equal(t, ToolHookDeny, result.Action)
	})

	t.Run("Non-matching command", func(t *testing.T) {
		rules := PermissionRules{
			DenyCommandRule("bash", "rm -rf *", "no rm"),
		}
		tool := newMockTool("bash", nil)
		call := newMockToolCall("bash", map[string]any{"command": "ls -la"})

		result := rules.Evaluate(tool, call)
		assert.Nil(t, result) // No match
	})

	t.Run("Command in different field", func(t *testing.T) {
		rules := PermissionRules{
			DenyCommandRule("shell", "rm *", "no rm"),
		}
		tool := newMockTool("shell", nil)
		// Uses "cmd" field instead of "command"
		call := newMockToolCall("shell", map[string]any{"cmd": "rm -rf /tmp"})

		result := rules.Evaluate(tool, call)
		assert.NotNil(t, result)
		assert.Equal(t, ToolHookDeny, result.Action)
	})
}

func TestGlobPatternMatching(t *testing.T) {
	t.Run("Question mark matches single char", func(t *testing.T) {
		rules := PermissionRules{
			AllowRule("read_?ile"),
		}
		tool := newMockTool("read_file", nil)
		call := newMockToolCall("read_file", nil)

		result := rules.Evaluate(tool, call)
		assert.NotNil(t, result)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("Multiple wildcards", func(t *testing.T) {
		rules := PermissionRules{
			AllowRule("*_file_*"),
		}
		tool := newMockTool("read_file_contents", nil)
		call := newMockToolCall("read_file_contents", nil)

		result := rules.Evaluate(tool, call)
		assert.NotNil(t, result)
		assert.Equal(t, ToolHookAllow, result.Action)
	})
}

func TestEmptyConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("Empty rules falls through to mode", func(t *testing.T) {
		config := &PermissionConfig{
			Mode:  PermissionModeBypassPermissions,
			Rules: PermissionRules{},
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("any", nil)
		call := newMockToolCall("any", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)
	})

	t.Run("Empty hooks are skipped", func(t *testing.T) {
		config := &PermissionConfig{
			Mode:        PermissionModeBypassPermissions,
			PreToolUse:  nil,
			PostToolUse: nil,
		}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("any", nil)
		call := newMockToolCall("any", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAllow, result.Action)

		postCtx := &PostToolUseContext{Tool: tool, Call: call, Result: &ToolCallResult{}}
		err = pm.RunPostToolUseHooks(ctx, postCtx)
		assert.NoError(t, err)
	})
}

func TestPlanModeEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("Plan mode with nil annotations", func(t *testing.T) {
		config := &PermissionConfig{Mode: PermissionModePlan}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("unknown", nil) // nil annotations
		call := newMockToolCall("unknown", nil)

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookDeny, result.Action)
	})
}

func TestAcceptEditsModeEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("AcceptEdits detects various edit commands", func(t *testing.T) {
		config := &PermissionConfig{Mode: PermissionModeAcceptEdits}
		pm := NewPermissionManager(config, nil)

		editCommands := []string{
			"touch newfile.txt",
			"rm old.txt",
			"cp src dst",
			"mv old new",
			"chmod 755 file",
		}

		for _, cmd := range editCommands {
			tool := newMockTool("bash", nil)
			call := newMockToolCall("bash", map[string]any{"command": cmd})

			result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
			assert.NoError(t, err, "Command: %s", cmd)
			assert.Equal(t, ToolHookAllow, result.Action, "Command: %s should be detected as edit", cmd)
		}
	})

	t.Run("AcceptEdits does not detect read commands", func(t *testing.T) {
		config := &PermissionConfig{Mode: PermissionModeAcceptEdits}
		pm := NewPermissionManager(config, nil)

		tool := newMockTool("bash", nil)
		call := newMockToolCall("bash", map[string]any{"command": "ls -la"})

		result, err := pm.EvaluateToolUse(ctx, tool, call, nil)
		assert.NoError(t, err)
		assert.Equal(t, ToolHookAsk, result.Action) // Falls through to ask
	})
}
